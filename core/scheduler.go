package core

import (
	"fmt"
	"sort"
	"sync"
)

// maxDirtyPasses bounds re-render cascades (a re-render that dirties another
// scope) so a runaway reactive cycle can't spin the flush forever.
const maxDirtyPasses = 100

type dirtyScope struct {
	seq       int
	render    func()
	cancelled bool
}

type Scheduler struct {
	queue    []Mutation
	dirty    map[any]*dirtyScope // scopes to re-render on next flush, keyed by token
	flushing map[any]*dirtyScope // entries being processed in the current pass
	inFlush  bool

	// OnWork is called when work first appears (a mutation is enqueued or a
	// scope is marked dirty). The bridge uses it to schedule exactly one
	// animation frame instead of polling every frame. nil in tests, which
	// drive Flush directly.
	OnWork func()

	// posted holds callbacks handed in by off-loop goroutines via Post; they
	// run on the render loop at the next flush. Guarded because Post is the one
	// thing called from other goroutines.
	postMu sync.Mutex
	posted []func()
}

// Post queues fn to run on the render loop at the next flush. It is safe to
// call from any goroutine — this is how off-loop code (timers, network) feeds
// state changes in without racing the renderer. See core.Schedule.
func (s *Scheduler) Post(fn func()) {
	s.postMu.Lock()
	s.posted = append(s.posted, fn)
	s.postMu.Unlock()
	if s.OnWork != nil {
		s.OnWork() // request a frame (single-threaded WASM: safe cross-goroutine)
	}
}

func (s *Scheduler) drainPosted() {
	s.postMu.Lock()
	posted := s.posted
	s.posted = nil
	s.postMu.Unlock()
	for _, fn := range posted {
		fn() // runs on the flush goroutine; may Set signals → dirty/enqueue
	}
}

func NewScheduler() *Scheduler {
	return &Scheduler{dirty: make(map[any]*dirtyScope)}
}

func (s *Scheduler) signalWork() {
	// During a flush, enqueues are drained by that same flush, so there is no
	// new frame to schedule.
	if s.inFlush {
		return
	}
	if s.OnWork != nil {
		s.OnWork()
	}
}

func (s *Scheduler) Enqueue(muts ...Mutation) {
	s.queue = append(s.queue, muts...)
	s.signalWork()
}

// MarkDirty schedules a scope (identified by a unique key, e.g. the ScopeNode
// pointer) to re-render on the next flush. Repeated marks before a flush
// coalesce into a single re-render — this is what turns N signal writes in one
// frame into one re-render + one diff. seq orders re-renders parent-before-
// child (a parent mounts before its children, so its seq is smaller).
func (s *Scheduler) MarkDirty(key any, seq int, render func()) {
	if e, ok := s.dirty[key]; ok {
		e.render = render
		e.cancelled = false
		return
	}
	s.dirty[key] = &dirtyScope{seq: seq, render: render}
	s.signalWork()
}

// CancelDirty drops a scope's pending re-render — used when a parent re-render
// unmounts a still-dirty child, so we don't re-render a removed subtree.
func (s *Scheduler) CancelDirty(key any) {
	if e, ok := s.dirty[key]; ok {
		e.cancelled = true
		delete(s.dirty, key)
	}
	if e, ok := s.flushing[key]; ok {
		e.cancelled = true
	}
}

// Flush re-renders dirty scopes (parent-before-child, skipping any cancelled
// mid-flush), then returns the coalesced mutation batch. It is the single
// place batched render work happens.
func (s *Scheduler) Flush() []Mutation {
	s.inFlush = true
	// Run callbacks posted by off-loop goroutines first, on this goroutine;
	// their signal writes then feed the dirty/enqueue passes below.
	s.drainPosted()
	for pass := 0; pass < maxDirtyPasses && len(s.dirty) > 0; pass++ {
		current := s.dirty
		s.dirty = make(map[any]*dirtyScope)
		s.flushing = current

		entries := make([]*dirtyScope, 0, len(current))
		for _, e := range current {
			entries = append(entries, e)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].seq < entries[j].seq })
		for _, e := range entries {
			if !e.cancelled {
				e.render()
			}
		}
		s.flushing = nil
	}
	s.inFlush = false

	out := coalesce(s.queue)
	s.queue = nil
	return out
}

// coalesce collapses redundant property/attribute writes to the same
// (node, key) to their last value, preserving order and all other mutations.
func coalesce(muts []Mutation) []Mutation {
	if len(muts) < 2 {
		return muts
	}
	type ck struct {
		t   MutationType
		id  int
		key string
	}
	lastIdx := make(map[ck]int, len(muts))
	for i, m := range muts {
		if m.Type == MutSetProperty || m.Type == MutSetAttribute {
			lastIdx[ck{m.Type, m.NodeID, m.Key}] = i
		}
	}
	out := make([]Mutation, 0, len(muts))
	for i, m := range muts {
		if m.Type == MutSetProperty || m.Type == MutSetAttribute {
			if lastIdx[ck{m.Type, m.NodeID, m.Key}] != i {
				continue // superseded by a later write
			}
		}
		out = append(out, m)
	}
	return out
}

func (s *Scheduler) Len() int {
	return len(s.queue)
}

func (s *Scheduler) String() string {
	return fmt.Sprintf("Scheduler(queue=%d dirty=%d)", len(s.queue), len(s.dirty))
}

type Mutation struct {
	Type    MutationType `json:"type"`
	NodeID  int          `json:"nodeId"`
	Key     string       `json:"key,omitempty"`
	Value   any          `json:"value,omitempty"`
	ChildID int          `json:"childId,omitempty"`
	RefID   int          `json:"refId,omitempty"`
	NS      string       `json:"ns,omitempty"` // XML namespace for CreateElement (SVG)
}

type MutationType int

const (
	MutCreateElement MutationType = iota
	MutRemoveNode
	MutSetAttribute
	MutSetProperty
	MutAppendChild
	MutInsertBefore
	MutRemoveAttribute
	MutHydrate      // claim a server-rendered node by id (Value = tag or "#text")
	MutInvoke       // call a method on a node (Key = method name), e.g. focus
	MutPortalAppend // append a child to a container matched by selector (Value)
)

func (mt MutationType) String() string {
	switch mt {
	case MutCreateElement:
		return "CreateElement"
	case MutRemoveNode:
		return "RemoveNode"
	case MutSetAttribute:
		return "SetAttribute"
	case MutSetProperty:
		return "SetProperty"
	case MutAppendChild:
		return "AppendChild"
	case MutInsertBefore:
		return "InsertBefore"
	case MutRemoveAttribute:
		return "RemoveAttribute"
	case MutHydrate:
		return "Hydrate"
	case MutInvoke:
		return "Invoke"
	case MutPortalAppend:
		return "PortalAppend"
	default:
		return "Unknown"
	}
}
