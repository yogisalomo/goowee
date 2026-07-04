package hooks

import "goowee/core"

type effectState struct {
	Deps    []core.SignalAccessor
	Cleanup func()
	Fn      func() func()
	Unsubs  []func()
}

func UseEffect(deps []core.SignalAccessor, fn func() func()) {
	// Effects are lifecycle side-effects; they must not run during a server
	// render (no mount/unmount there, and RunFrameCleanup is never called
	// server-side, so any goroutine/subscription would leak per request).
	if core.CurrentEnv() == core.EnvServer {
		return
	}

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
		state.Unsubs = append(state.Unsubs, dep.Subscribe(exec))
	}
}

func RunFrameCleanup(frame *core.ComponentFrame) {
	for _, disposer := range frame.Disposers {
		disposer()
	}
	frame.Disposers = nil
	for _, hook := range frame.Hooks {
		if es, ok := hook.(*effectState); ok {
			for _, unsub := range es.Unsubs {
				if unsub != nil {
					unsub()
				}
			}
			es.Unsubs = nil
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
