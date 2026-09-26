package hooks

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

type effectState struct {
	Deps     []core.SignalAccessor
	Cleanup  func()
	Fn       func() func()
	Unsubs   []func()
	disposed bool // unmounted (possibly before the first run)
}

// UseEffect runs fn once the component is mounted — after its DOM has been
// applied to the document, so refs are set and elements can be measured or
// handed to a JavaScript library — and again whenever a dep changes. If fn
// returns a cleanup, it runs before each re-run and on unmount. Client only:
// never runs during SSR.
func UseEffect(deps []core.SignalAccessor, fn func() func()) {
	// Effects are lifecycle side-effects; they must not run during a server
	// render (no mount/unmount there, and RunFrameCleanup is never called
	// server-side, so any goroutine/subscription would leak per request).
	if runtime.CurrentEnv() == runtime.EnvServer {
		return
	}

	frame := runtime.CurrentComponent()
	state := &effectState{Deps: deps, Fn: fn}
	if frame != nil {
		frame.Hooks = append(frame.Hooks, state)
	}

	// The first run waits for the DOM (see runtime.QueueEffect); deps are
	// subscribed then, so a change before mount can't run the effect early.
	runtime.QueueEffect(func() {
		if state.disposed {
			return // unmounted before it ever ran
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
	})
}

// DisposeFrameTree disposes f and every frame below it, children first. Used
// when a subtree is abandoned without a node tree to walk — an error boundary
// discarding the part of its child that rendered before a panic.
func DisposeFrameTree(f *core.ComponentFrame) {
	if f == nil {
		return
	}
	for _, c := range f.Children {
		DisposeFrameTree(c)
	}
	f.Children = nil
	RunFrameCleanup(f)
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
			es.disposed = true
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
