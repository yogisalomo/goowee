package core

func Computed[T any](deps []SignalAccessor, compute func() T) *Signal[T] {
	sig := NewSignal(compute())
	RegisterSignal(sig, "computed")
	for _, dep := range deps {
		unsub := dep.Subscribe(func() { sig.Set(compute()) })
		RegisterDisposer(unsub)
	}
	return sig
}
