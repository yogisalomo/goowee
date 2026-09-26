package hooks

// OnMount runs fn once, after the component mounts: its DOM is in the
// document by then, so refs are set — focus an input, measure an element, or
// hand it to a JavaScript library (see bridge.Element). If fn returns a
// non-nil cleanup function, that cleanup runs when the component unmounts
// (e.g. a route change tears the component down). Use it for setup that owns
// a resource — timers, goroutines, subscriptions — so the resource is
// released instead of leaking on every remount.
//
//	hooks.OnMount(func() func() {
//	    stop := startTicker()
//	    return func() { stop() }
//	})
//
// It is a thin wrapper over UseEffect with no dependencies, so the effect
// never re-runs; it fires on mount and its cleanup fires on unmount.
func OnMount(fn func() func()) {
	UseEffect(nil, fn)
}
