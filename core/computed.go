package core

// Computed returns a read-only signal derived from deps by compute. It is lazy:
//
//   - While something observes it (a binding, a scope, Watch, another observed
//     Computed), it subscribes to deps and pushes changes to its observers.
//   - With no observers it holds no subscriptions at all — nothing to leak, and
//     nothing to dispose — and Get recomputes on demand when a dep has changed.
//
// So a Computed created in a scope's render (h.Textf does this) lives exactly as
// long as the node bound to it, and one created during SSR never subscribes to
// the (possibly package-level) signals it reads.
func Computed[T any](deps []SignalAccessor, compute func() T) *Signal[T] {
	sig := &Signal[T]{derived: &derivation[T]{deps: deps, compute: compute}}
	RegisterSignal(sig, "computed")
	return sig
}

// derivation is the Computed half of a Signal.
type derivation[T any] struct {
	deps    []SignalAccessor
	compute func() T

	evaluated bool
	depVers   []uint64 // dep versions at the last evaluation (0 = unknown)
	unsubs    []func() // dep subscriptions while active
	notified  T        // the value observers were last told about
}

// versioned is implemented by *Signal; other SignalAccessor implementations
// are treated as always possibly-changed.
type versioned interface{ currentVersion() uint64 }

func (d *derivation[T]) stale() bool {
	if !d.evaluated {
		return true
	}
	for i, dep := range d.deps {
		v, ok := dep.(versioned)
		if !ok || v.currentVersion() != d.depVers[i] {
			return true
		}
	}
	return false
}

// refresh re-evaluates if any dep changed since the last evaluation. It
// updates the cached value (and version, so downstream Computeds see the
// change) but does not notify — notification is the dep subscription's job, so
// a read can never swallow a pending notification.
func (d *derivation[T]) refresh(s *Signal[T]) {
	if !d.stale() {
		return
	}
	var v T
	checkedEval("Computed", d.deps, func() { v = d.compute() })
	if d.depVers == nil {
		d.depVers = make([]uint64, len(d.deps))
	}
	for i, dep := range d.deps {
		if vd, ok := dep.(versioned); ok {
			d.depVers[i] = vd.currentVersion()
		}
	}
	if !d.evaluated || !s.equal(s.value, v) {
		s.value = v
		s.version++
	}
	d.evaluated = true
}

func (d *derivation[T]) activate(s *Signal[T]) {
	d.refresh(s)
	d.notified = s.value
	for _, dep := range d.deps {
		d.unsubs = append(d.unsubs, dep.Subscribe(func() { d.onDepChange(s) }))
	}
}

func (d *derivation[T]) deactivate() {
	for _, u := range d.unsubs {
		u()
	}
	d.unsubs = nil
}

// onDepChange runs when a dep notifies: bring the value up to date (a read may
// already have done so) and tell observers if it differs from what they last
// saw.
func (d *derivation[T]) onDepChange(s *Signal[T]) {
	d.refresh(s)
	if s.equal(d.notified, s.value) {
		return
	}
	d.notified = s.value
	s.publish()
}
