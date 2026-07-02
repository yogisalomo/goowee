package hooks

import "goowee/core"

type effectState struct {
	Deps    []core.SignalAccessor
	Cleanup func()
	Fn      func() func()
}

func UseEffect(deps []core.SignalAccessor, fn func() func()) {
	frame := core.CurrentComponent()
	state := &effectState{Deps: deps, Fn: fn}
	if frame != nil {
		frame.Hooks = append(frame.Hooks, state)
	}

	state.Cleanup = fn()
	exec := func() {
		if state.Cleanup != nil {
			state.Cleanup()
		}
		state.Cleanup = fn()
	}
	for _, dep := range deps {
		dep.Subscribe(exec)
	}
}

func RunFrameCleanup(frame *core.ComponentFrame) {
	for _, hook := range frame.Hooks {
		if es, ok := hook.(*effectState); ok {
			if es.Cleanup != nil {
				es.Cleanup()
				es.Cleanup = nil
			}
		}
	}
	for _, child := range frame.Children {
		RunFrameCleanup(child)
	}
}
