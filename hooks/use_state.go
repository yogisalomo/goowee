package hooks

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

func UseState[T any](initial T) (*core.Signal[T], func(T)) {
	sig := core.NewSignal(initial)
	if frame := runtime.CurrentComponent(); frame != nil {
		frame.Hooks = append(frame.Hooks, sig)
	}
	return sig, func(v T) {
		sig.Set(v)
	}
}
