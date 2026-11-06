package core

// Context is a typed value that a subtree provides to everything below it,
// for dependencies that would otherwise be threaded through every component
// layer or kept in a package-level signal: the session, an API client, the
// theme, the locale.
//
//	var Session = core.NewContext(Session{Anonymous: true})
//
//	// near the root, per request on the server:
//	Session.Provide(core.NewSignal(current), App())
//
//	// anywhere below:
//	func Greeting() core.Node {
//	    return Session.Use(func(s *core.Signal[Session]) core.Node {
//	        return P(Textf("Hello, %s", core.Computed([]core.SignalAccessor{s}, func() string { return s.Get().Name })))
//	    })
//	}
//
// The value is a signal, so consumers bind it and update when it changes;
// the nearest Provide of the same Context wins. Lookup happens in the
// renderer's walk, not through a hook: a hook is a plain function and can't
// tell which of several concurrent server renders it belongs to, while each
// walk has its own providers, so concurrent SSR renders never see each
// other's values (ADR-027).
type Context[T any] struct {
	def *Signal[T]
}

// NewContext creates a context whose consumers get def when no provider is
// above them.
func NewContext[T any](def T) *Context[T] {
	return &Context[T]{def: NewSignal(def)}
}

// Provide renders children with value as c's value for every consumer below
// them, until a nearer Provide of c shadows it. Keep providing the same signal
// for the subtree's lifetime and change its value to update consumers. On a
// re-render of the parent, children are diffed like a Show's content.
func (c *Context[T]) Provide(value *Signal[T], children ...Node) Node {
	if value == nil {
		value = c.def
	}
	return &ScopeNode{
		Render:   func() Node { return &FragmentNode{Children: children} },
		Provides: &ContextValue{Key: c, Value: value},
	}
}

// Use renders render with the signal the nearest Provide of c supplies, or,
// with no provider above, the default given to NewContext (a shared signal:
// read it, don't Set it; Provide a signal of your own to have a value to
// change). Like a Show's content, render runs again when a parent re-render
// rebuilds it, so keep state in a component inside it. Bind the signal (or
// list it as a dependency) to follow its changes; reading it with Get in
// render takes a snapshot, which dev mode reports.
func (c *Context[T]) Use(render func(value *Signal[T]) Node) Node {
	return &ScopeNode{
		Consumes: &ContextUse{
			Key:     c,
			Default: c.def,
			Render: func(v any) Node {
				var n Node
				checkedEval("context consumer (Context.Use)", nil, func() { n = render(v.(*Signal[T])) })
				return n
			},
		},
	}
}

// ContextValue is what a provider scope supplies: Value for Key. Renderers
// read it; apps use Context.Provide.
type ContextValue struct {
	Key   any
	Value any
}

// ContextUse is what a consumer scope asks for: the nearest value for Key
// (Default when none) to Render with. Renderers read it; apps use
// Context.Use.
type ContextUse struct {
	Key     any
	Default any
	Render  func(value any) Node
}
