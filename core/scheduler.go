package core

import (
	"fmt"
	"sort"
	"sync"
)

// maxDirtyPasses bounds re-render cascades (a re-render that dirties another
// scope) so a runaway reactive cycle can't spin the flush forever. When a
// flush reaches it with scopes still dirty, it logs signal.cycle once (with
// the scopes' component paths in dev mode) and drops them, so the next frame
// starts clean instead of burning another 100 passes on the same cycle. Like
// maxNotifyPasses for a signal's subscribers, it stops a loop, not a bug: the
// UI shows the last state rendered.
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

	// DescribeDirty names a dirty scope for the log when a flush gives up
	// (its component path). The DOM renderer sets it in dev mode; nil
	// otherwise.
	DescribeDirty func(key any) string

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

	// reads holds the callbacks of in-flight DOM reads (Ref.Get), keyed by
	// request id. The bridge answers each MutRead in a batch through Resolve.
	reads    map[int]func(any)
	nextRead int

	// effects are effect first-runs (OnMount, UseEffect) waiting for the DOM
	// their component rendered: the bridge runs them via RunEffects right
	// after it applies the frame's mutations.
	effects []func()

	// frames are callbacks registered with NextFrame: they run once, at the
	// start of the next frame. They are not "work" (HasWork ignores them), so
	// a test harness flushing until quiescent doesn't spin on an animation,
	// and the bridge can stop animating while the page is hidden.
	frames []func()

	// ManualFrames stops Flush from running frame callbacks: the caller runs
	// them with RunFrames. gooweetest sets it, so tests step animations with
	// Screen.Frame instead of every flush advancing them.
	ManualFrames bool
}

// NextFrame runs fn once at the start of the next frame, on the render loop.
// Call it again from fn to keep animating. It requests a frame (OnWork) even
// during a flush: a callback registered from a frame callback runs on the
// following frame, not the current one.
func (s *Scheduler) NextFrame(fn func()) {
	s.frames = append(s.frames, fn)
	if s.OnWork != nil {
		s.OnWork()
	}
}

// RunFrames runs the frame callbacks registered so far, as one batch, each
// panic-contained. Callbacks they register wait for the next RunFrames.
func (s *Scheduler) RunFrames() {
	if len(s.frames) == 0 {
		return
	}
	fs := s.frames
	s.frames = nil
	Batch(func() {
		for _, fn := range fs {
			Recover(LogRecoverFrame, "panic in frame callback", nil, fn)
		}
	})
}

// HasFrames reports whether frame callbacks wait for the next frame.
func (s *Scheduler) HasFrames() bool { return len(s.frames) > 0 }

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
	// One batch for the whole inbox: several goroutines' results landing in
	// the same frame notify each written signal once. Each callback is
	// contained, so one panicking callback can't abort the flush (and crash
	// the render loop) or starve the others.
	Batch(func() {
		for _, fn := range posted {
			Recover(LogRecoverScheduled, "panic in core.Schedule callback", nil, fn)
		}
	})
}

func NewScheduler() *Scheduler {
	return &Scheduler{dirty: make(map[any]*dirtyScope), reads: make(map[int]func(any))}
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

// Read enqueues a MutRead for prop on nodeID and parks fn until the bridge
// delivers the value via Resolve. The mutation's Value carries the request id.
func (s *Scheduler) Read(nodeID int, prop string, fn func(any)) {
	s.nextRead++
	id := s.nextRead
	s.reads[id] = fn
	s.Enqueue(Mutation{Type: MutRead, NodeID: nodeID, Key: prop, Value: id})
}

// Resolve delivers the reply to a pending read on the render loop. Unknown ids
// (already resolved, or never issued) are ignored. A panicking callback is
// contained and logged, like an event handler.
func (s *Scheduler) Resolve(id int, v any) {
	fn, ok := s.reads[id]
	if !ok {
		return
	}
	delete(s.reads, id)
	defer func() {
		if rec := recover(); rec != nil {
			Log(LogRecoverRead, "panic in ref read callback", map[string]any{
				"request": id,
				"panic":   rec,
			})
		}
	}()
	Batch(func() { fn(v) })
}

// QueueEffect defers fn until the mutations rendered so far are in the DOM.
// Renderers queue effect first-runs here, so OnMount/UseEffect see the
// component's elements (refs are set, the nodes are in the document).
func (s *Scheduler) QueueEffect(fn func()) {
	s.effects = append(s.effects, fn)
	s.signalWork()
}

// RunEffects runs the queued effects, in queue order (parents before
// children), as one batch; each is panic-contained. The bridge calls it after
// applying a frame's mutations; headless tests call it after Flush.
func (s *Scheduler) RunEffects() {
	if len(s.effects) == 0 {
		return
	}
	fx := s.effects
	s.effects = nil
	Batch(func() {
		for _, fn := range fx {
			Recover(LogRecoverEffect, "panic in effect", nil, fn)
		}
	})
}

// HasWork reports whether a Flush or RunEffects would do anything: queued
// mutations, dirty scopes, callbacks posted by goroutines, or effects waiting
// to run. Frame callbacks (NextFrame) don't count; see HasFrames. Test harnesses use it to flush until quiescent.
func (s *Scheduler) HasWork() bool {
	s.postMu.Lock()
	posted := len(s.posted)
	s.postMu.Unlock()
	return len(s.queue) > 0 || len(s.dirty) > 0 || posted > 0 || len(s.effects) > 0
}

// PendingEffects reports how many effects wait for RunEffects.
func (s *Scheduler) PendingEffects() int { return len(s.effects) }

// PendingReads reports how many reads await a reply.
func (s *Scheduler) PendingReads() int { return len(s.reads) }

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
	// Animation callbacks first: what they draw and set goes out with this
	// frame.
	if !s.ManualFrames {
		s.RunFrames()
	}
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
	if len(s.dirty) > 0 {
		s.dropCycle()
	}
	s.inFlush = false

	out := coalesce(s.queue)
	s.queue = nil
	return out
}

// dropCycle logs and drops the scopes still dirty after maxDirtyPasses: a
// render keeps dirtying scopes (its own or another's), which only a code
// change ends.
func (s *Scheduler) dropCycle() {
	fields := map[string]any{"passes": maxDirtyPasses, "scopes": len(s.dirty)}
	if s.DescribeDirty != nil {
		var paths []string
		for key := range s.dirty {
			if p := s.DescribeDirty(key); p != "" {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		fields["components"] = paths
	}
	Log(LogSignalCycle, "re-render cycle: scopes kept dirtying each other; dropping them for this frame", fields)
	for _, e := range s.dirty {
		e.cancelled = true
	}
	s.dirty = make(map[any]*dirtyScope)
}

// coalesce collapses redundant property/attribute writes to the same
// (node, key) to their last value, and draws to the same canvas to the last
// one, preserving order and all other mutations.
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
		if lastWins(m.Type) {
			lastIdx[ck{m.Type, m.NodeID, m.Key}] = i
		}
	}
	out := make([]Mutation, 0, len(muts))
	for i, m := range muts {
		if lastWins(m.Type) {
			if lastIdx[ck{m.Type, m.NodeID, m.Key}] != i {
				continue // superseded by a later write
			}
		}
		out = append(out, m)
	}
	return out
}

// lastWins reports whether a later mutation of this type to the same (node,
// key) makes earlier ones redundant.
func lastWins(t MutationType) bool {
	return t == MutSetProperty || t == MutSetAttribute || t == MutDraw
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
	MutRead         // read a property (Key) of a node; Value = request id, answered via Scheduler.Resolve
	MutCall         // call a method (Key) with arguments (Value, a []any), after the batch's writes
	MutDraw         // replay a goowee/canvas command buffer (Value, a []uint32) on a <canvas>, after the batch
	MutListen       // the node has (Value true) or no longer has (false) a Go handler for event Key
)

// DrawVersion is the goowee/canvas buffer format MutDraw carries. The runtime
// (goowee.canvasVersion in goowee.js) must report the same version.
const DrawVersion = 1

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
	case MutRead:
		return "Read"
	case MutCall:
		return "Call"
	case MutDraw:
		return "Draw"
	case MutListen:
		return "Listen"
	default:
		return "Unknown"
	}
}
