package core

type subscriber struct {
	id int
	fn func()
}

type Signal[T any] struct {
	value     T
	subs      []subscriber
	nextSubID int
	eq        func(a, b T) bool
	notifying bool
	dirty     bool
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

func (s *Signal[T]) Get() T { return s.value }

func (s *Signal[T]) Value() any { return s.value }

func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

func (s *Signal[T]) Subscribe(fn func()) func() {
	s.nextSubID++
	id := s.nextSubID
	s.subs = append(s.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range s.subs {
			if s.subs[i].id == id {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

const maxNotifyPasses = 1000

// Set stores v and notifies subscribers, unless v equals the current
// value (see equal). Notification semantics:
//
//   - Subscribers run in subscription order.
//   - The subscriber list is snapshotted per pass: subscribers added
//     during a pass do not run in that pass; subscribers removed during
//     a pass are skipped (liveness is re-checked before each call).
//   - If a subscriber calls Set with a different value, the current
//     pass finishes, then all subscribers run again observing the
//     final value (bounded by maxNotifyPasses; a cycle logs and stops).
//   - A nested Set on the same signal never recurses
//     (it marks dirty and returns; the outer loop re-notifies).
//   - Equality is interface equality (any(a) == any(b)). For pointer-
//     typed signals (Signal[*Foo]), the pointer is compared, not the
//     pointee — mutating through the same pointer and calling Set again
//     is silently skipped. Use WithEquals for value-level equality.
func (s *Signal[T]) Set(v T) {
	if s.equal(s.value, v) {
		return
	}
	s.value = v
	if s.notifying {
		s.dirty = true
		return
	}
	s.notifying = true
	defer func() { s.notifying = false }()

	for pass := 0; ; pass++ {
		if pass >= maxNotifyPasses {
			Log(LogSignalCycle, "signal update cycle detected; stopping notification", map[string]any{
				"passes": maxNotifyPasses,
			})
			return
		}
		s.dirty = false

		ids := make([]int, len(s.subs))
		for i, sub := range s.subs {
			ids[i] = sub.id
		}
		for _, id := range ids {
			if fn := s.lookup(id); fn != nil {
				fn()
			}
		}
		if !s.dirty {
			return
		}
	}
}

func (s *Signal[T]) lookup(id int) func() {
	for i := range s.subs {
		if s.subs[i].id == id {
			return s.subs[i].fn
		}
	}
	return nil
}

func (s *Signal[T]) equal(a, b T) (eq bool) {
	if s.eq != nil {
		return s.eq(a, b)
	}
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return any(a) == any(b)
}

var _ SignalAccessor = (*Signal[int])(nil)
var _ SignalAccessor = (*Signal[string])(nil)
