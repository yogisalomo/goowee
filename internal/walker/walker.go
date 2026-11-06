package walker

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// Walker assigns sequential IDs and traverses a core.Node tree, calling the
// Visitor for each node. Both DOM and SSR renderers use the same walker,
// guaranteeing identical node IDs by construction.
type Walker struct {
	nextID int
	ctx    *Context // context values in scope at this point of the walk
}

// Context is the chain of context values in scope at a point of a walk,
// innermost first. It is immutable, so a renderer can keep one (for a
// scope's later re-renders) and restore it with SetContext. nil is the root:
// nothing provided.
type Context struct {
	key, value any
	parent     *Context
}

// lookup is the nearest value provided for key, or def.
func (c *Context) lookup(key, def any) any {
	for ; c != nil; c = c.parent {
		if c.key == key {
			return c.value
		}
	}
	return def
}

// with is c plus p's value, innermost.
func (c *Context) with(p *core.ContextValue) *Context {
	if p == nil {
		return c
	}
	return &Context{key: p.Key, value: p.Value, parent: c}
}

// Context returns the context values in scope at this point of the walk.
func (w *Walker) Context() *Context { return w.ctx }

// SetContext makes c the context values in scope, e.g. to re-render a scope
// where it was mounted.
func (w *Walker) SetContext(c *Context) { w.ctx = c }

// ScopeContent renders a scope's content: a context consumer's render with
// the nearest provided value, otherwise the scope's render.
func (w *Walker) ScopeContent(sn *core.ScopeNode) core.Node {
	if use := sn.Consumes; use != nil {
		return use.Render(w.ctx.lookup(use.Key, use.Default))
	}
	return core.RenderScope(sn)
}

// WithProvided runs fn with sn's provided value (if any) in scope, restoring
// the previous values afterwards, panic or not.
func (w *Walker) WithProvided(sn *core.ScopeNode, fn func()) {
	if sn.Provides == nil {
		fn()
		return
	}
	outer := w.ctx
	w.ctx = outer.with(sn.Provides)
	defer func() { w.ctx = outer }()
	fn()
}

func New() *Walker {
	return &Walker{nextID: 1}
}

func (w *Walker) Reset() {
	w.nextID = 1
	w.ctx = nil
}

func (w *Walker) AllocID() int {
	id := w.nextID
	w.nextID++
	return id
}

// NextID returns the id the next AllocID will hand out. Ids are sequential, so
// the ids allocated by a walk are exactly [NextID before, NextID after).
func (w *Walker) NextID() int { return w.nextID }

// Visitor is implemented by DOM and SSR renderers. Each method receives the
// node (with its ID already set for element/text nodes) and a walkChild func
// that walks a single child node and returns its root ID.
type Visitor interface {
	VisitElement(id int, el *core.ElementNode, walkChild func(core.Node) int)
	VisitText(id int, tn *core.TextNode)
	VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int)
	VisitPortal(pn *core.PortalNode, walkChild func(core.Node) int)
	VisitMetadata(mn *core.MetadataNode, walkChild func(core.Node) int)
	VisitErrorBoundary(ebn *core.ErrorBoundaryNode, walkInner func() int) int
	VisitComponentEnter(cn *core.ComponentNode)
	VisitComponentLeave(cn *core.ComponentNode, innerID int)
	VisitScopeEnter(sn *core.ScopeNode)
	// VisitScopeLeave runs after the scope's content is walked and its end
	// anchor id (sn.Anchor) allocated; its return value is the scope's id as
	// seen by the parent (0 when the visitor attached the content itself).
	VisitScopeLeave(sn *core.ScopeNode, innerID int) int
	VisitRaw(id int, rn *core.RawNode)
}

func (w *Walker) Walk(n core.Node, v Visitor) int {
	n = core.FlatTree(n)
	switch node := n.(type) {
	case *core.ElementNode:
		if node == nil {
			return 0
		}
		id := w.AllocID()
		node.ID = id
		if node.Ref != nil {
			node.Ref.ID = id
		}
		v.VisitElement(id, node, func(child core.Node) int {
			return w.Walk(child, v)
		})
		return id

	case *core.TextNode:
		if node == nil {
			return 0
		}
		id := w.AllocID()
		node.ID = id
		v.VisitText(id, node)
		return id

	case *core.FragmentNode:
		if node == nil {
			return 0
		}
		v.VisitFragment(node, func(child core.Node) int {
			return w.Walk(child, v)
		})
		return 0

	case *core.ComponentNode:
		if node == nil {
			return 0
		}
		v.VisitComponentEnter(node)
		frame := runtime.PushComponent()
		inner := core.FlatTree(node.Render())
		id := w.Walk(inner, v)
		runtime.PopComponent()
		node.Prev = inner
		node.Frame = frame
		v.VisitComponentLeave(node, id)
		return id

	case *core.ScopeNode:
		if node == nil {
			return 0
		}
		v.VisitScopeEnter(node)
		inner := core.FlatTree(w.ScopeContent(node))
		var id int
		w.WithProvided(node, func() { id = w.Walk(inner, v) })
		node.Prev = inner
		// The end anchor is allocated after the content, in both renderers, so
		// SSR and client ids stay in parity.
		node.Anchor = w.AllocID()
		return v.VisitScopeLeave(node, id)

	case *core.PortalNode:
		if node == nil {
			return 0
		}
		// The end anchor is allocated here, in both renderers, so ids after a
		// portal stay in SSR/client parity (the server renders no portal).
		node.Anchor = w.AllocID()
		v.VisitPortal(node, func(child core.Node) int {
			return w.Walk(child, v)
		})
		return 0

	case *core.MetadataNode:
		if node == nil {
			return 0
		}
		v.VisitMetadata(node, func(child core.Node) int {
			return w.Walk(child, v)
		})
		return 0

	case *core.ErrorBoundaryNode:
		if node == nil {
			return 0
		}
		return v.VisitErrorBoundary(node, func() int {
			return w.Walk(node.Child, v)
		})

	case *core.RawNode:
		if node == nil {
			return 0
		}
		id := w.AllocID()
		node.ID = id
		v.VisitRaw(id, node)
		return id
	}
	return 0
}
