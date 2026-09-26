package core

import "reflect"

// subscriber is one Subscribe registration. Unsubscribing marks it dead (O(1))
// rather than splicing the slice; dead entries are skipped by notify and
// compacted away lazily, never during a notification pass.
type subscriber struct {
	fn   func()
	dead bool
}

type Signal[T any] struct {
	value     T
	version   uint64 // bumped whenever value changes; lets Computeds detect stale deps
	subs      []*subscriber
	deadSubs  int // dead entries still in subs, awaiting compaction
	eq        func(a, b T) bool
	notifying bool
	dirty     bool
	queued    bool // written inside a Batch; notifies when the batch ends

	// Set only for a Computed (see computed.go).
	derived *derivation[T]
}

func NewSignal[T any](v T) *Signal[T] {
	return &Signal[T]{value: v}
}

// WithEquals sets a custom equality function used by Set to decide
// whether a change is real. Intended for construction-time chaining:
//
//	s := core.NewSignal(user).WithEquals(func(a, b User) bool { return a.ID == b.ID })
//
// Without this, Set compares via interface equality (any(a) == any(b)).
// For pointer-typed signals (Signal[*Foo]), interface equality compares
// the pointer, not the pointee — mutating a struct through the same
// pointer and calling Set again is silently treated as equal and will
// NOT notify. Use WithEquals (or avoid mutating through pointers) to
// opt into value-level equality.
func (s *Signal[T]) WithEquals(eq func(a, b T) bool) *Signal[T] {
	s.eq = eq
	return s
}

// Get returns the current value. For a Computed it is always up to date: if a
// dependency changed since the last evaluation (an unobserved Computed, or a
// read inside a Batch before notifications ran), it recomputes first.
func (s *Signal[T]) Get() T {
	if readTracker != nil {
		recordRead(s) // dev mode only (see devcheck.go)
	}
	if s.derived != nil {
		s.derived.refresh(s)
	}
	return s.value
}

// Peek returns the current value like Get, but is never counted as a
// dependency read by dev-mode checking: use it for an intentional snapshot
// inside a reactive region.
func (s *Signal[T]) Peek() T {
	if s.derived != nil {
		s.derived.refresh(s)
	}
	return s.value
}

func (s *Signal[T]) Value() any { return s.Get() }

func (s *Signal[T]) currentVersion() uint64 {
	// Called by Computeds checking their deps: not a user read.
	if s.derived != nil {
		s.derived.refresh(s)
	}
	return s.version
}

func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

// Subscribe registers fn to run after each change and returns a function that
// removes it. Unsubscribing is O(1) and idempotent.
func (s *Signal[T]) Subscribe(fn func()) func() {
	if s.derived != nil && len(s.subs)-s.deadSubs == 0 {
		s.derived.activate(s) // first observer: start listening to deps
	}
	sub := &subscriber{fn: fn}
	s.subs = append(s.subs, sub)
	return func() {
		if sub.dead {
			return
		}
		sub.dead = true
		sub.fn = nil // release the closure now; the slot is compacted later
		s.deadSubs++
		s.compact()
		if s.derived != nil && len(s.subs)-s.deadSubs == 0 {
			s.derived.deactivate() // last observer gone: stop listening
		}
	}
}

// compact drops dead subscribers once they make up at least half the slice,
// so unsubscribe stays amortized O(1). It never runs mid-notification: the
// notify loop indexes subs positionally.
func (s *Signal[T]) compact() {
	if s.notifying || s.deadSubs == 0 || s.deadSubs*2 < len(s.subs) {
		return
	}
	live := s.subs[:0]
	for _, sub := range s.subs {
		if !sub.dead {
			live = append(live, sub)
		}
	}
	for i := len(live); i < len(s.subs); i++ {
		s.subs[i] = nil // let dropped subscribers be collected
	}
	s.subs = live
	s.deadSubs = 0
}

const maxNotifyPasses = 1000

// Set stores v and notifies subscribers, unless v equals the current
// value (see equal). Notification semantics:
//
//   - Subscribers run in subscription order.
//   - Each pass covers the subscribers present when it starts: subscribers
//     added during a pass do not run in that pass; subscribers removed during
//     a pass are skipped.
//   - If a subscriber calls Set with a different value, the current
//     pass finishes, then all subscribers run again observing the
//     final value (bounded by maxNotifyPasses; a cycle logs and stops).
//   - A nested Set on the same signal never recurses
//     (it marks dirty and returns; the outer loop re-notifies).
//   - Inside Batch, the value is stored immediately but notification is
//     deferred to the end of the outermost batch (once per signal).
//   - Equality is interface equality (any(a) == any(b)). For pointer-
//     typed signals (Signal[*Foo]), the pointer is compared, not the
//     pointee — mutating through the same pointer and calling Set again
//     is silently skipped. Use WithEquals for value-level equality.
func (s *Signal[T]) Set(v T) {
	if s.equal(s.value, v) {
		return
	}
	s.value = v
	s.version++
	s.publish()
}

// publish notifies subscribers of a change now, or at the end of the current
// Batch.
func (s *Signal[T]) publish() {
	if batchDepth > 0 {
		if !s.queued {
			s.queued = true
			batchQueue = append(batchQueue, s)
		}
		return
	}
	s.notify()
}

// flushBatched delivers a notification deferred by Batch.
func (s *Signal[T]) flushBatched() {
	s.queued = false
	s.notify()
}

func (s *Signal[T]) notify() {
	if s.notifying {
		s.dirty = true
		return
	}
	s.notifying = true
	defer func() {
		s.notifying = false
		s.compact()
	}()

	for pass := 0; ; pass++ {
		if pass >= maxNotifyPasses {
			Log(LogSignalCycle, "signal update cycle detected; stopping notification", map[string]any{
				"passes": maxNotifyPasses,
			})
			return
		}
		s.dirty = false
		// No snapshot copy: subscribers appended during the pass land beyond n,
		// removed ones are flagged dead, and compaction waits for the pass to
		// end — so indexing the live slice is safe and allocation-free.
		n := len(s.subs)
		saved := readTracker // subscribers' reads aren't the tracked caller's
		if saved != nil {
			readTracker = nil
		}
		for i := 0; i < n; i++ {
			if sub := s.subs[i]; !sub.dead {
				sub.fn()
			}
		}
		if saved != nil {
			readTracker = saved
		}
		if !s.dirty {
			return
		}
	}
}

func (s *Signal[T]) equal(a, b T) bool {
	if s.eq != nil {
		return s.eq(a, b)
	}
	// Comparing uncomparable dynamic types (slices/maps/funcs) with == panics.
	// Go can recover that runtime panic; TinyGo cannot, so we must not trigger
	// it. Decide comparability by reflection instead: an uncomparable value
	// (e.g. a signal holding a []T) is treated as always-changed, matching the
	// previous recover-based fallback but without the crash on TinyGo.
	// Value.Comparable inspects the dynamic value, so it also catches a
	// comparable-looking struct whose interface field holds a slice.
	ia, ib := any(a), any(b)
	if ia == nil || ib == nil {
		return ia == nil && ib == nil
	}
	if !reflect.ValueOf(ia).Comparable() || !reflect.ValueOf(ib).Comparable() {
		return false
	}
	return ia == ib
}

var _ SignalAccessor = (*Signal[int])(nil)
var _ SignalAccessor = (*Signal[string])(nil)
