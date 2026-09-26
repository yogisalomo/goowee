package core

// ComponentWithProps is a component whose parent can hand it new inputs.
// render runs once, at mount, and receives the props as a signal. When a later
// re-render of the parent produces the same component (same name and key) with
// different props, the mounted instance is kept — its state and effects
// survive — and the props signal is set to the new value, so everything bound
// to it or derived from it updates:
//
//	func UserCard(u User) core.Node {
//	    return core.ComponentWithProps("UserCard", u, func(p *core.Signal[User]) core.Node {
//	        expanded, setExpanded := hooks.UseState(false) // survives prop changes
//	        name := core.Computed([]core.SignalAccessor{p}, func() string { return p.Get().Name })
//	        return h.Div(h.TextS(name), …)
//	    })
//	}
//
// A plain Component captures whatever values it was called with at mount; use
// this when those values can change (or give the component a key to remount it).
// Props are compared like signal values: == for comparable types, and always
// "changed" for slices, maps, and funcs (use WithEquals on your own signal, or
// pass a comparable struct, to avoid needless updates).
func ComponentWithProps[P any](name string, props P, render func(props *Signal[P]) Node) *ComponentNode {
	c := &ComponentNode{Name: name, props: props, hasProps: true}
	c.Render = func() Node {
		sig := NewSignal(props)
		c.setProps = func(v any) { sig.Set(v.(P)) }
		RegisterSignal(sig, "props")
		return render(sig)
	}
	return c
}
