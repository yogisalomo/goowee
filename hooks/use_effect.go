package hooks

import "github.com/yogisalomo/goowee/core"

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

// RunFrameCleanup disposes a single frame's own resources: registered
// disposers (signal subscriptions from Computed/Watch, scope teardowns) and
// effect cleanups. It does not recurse into child frames — the renderer walks
// the node tree when unmounting a subtree and cleans each component frame it
// reaches, so recursing here would double-dispose.
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
}
