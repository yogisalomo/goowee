package hooks

import "goowee/core"

func UseState[T any](initial T) (*core.Signal[T], func(T)) {
	sig := core.NewSignal(initial)
	if frame := core.CurrentComponent(); frame != nil {
		frame.Hooks = append(frame.Hooks, sig)
	}
	return sig, func(v T) {
		sig.Set(v)
	}
}
