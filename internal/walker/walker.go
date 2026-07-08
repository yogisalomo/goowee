package walker

import "github.com/yogisalomo/goowee/core"

// Walker assigns sequential IDs and traverses a core.Node tree, calling the
// Visitor for each node. Both DOM and SSR renderers use the same walker,
// guaranteeing identical node IDs by construction.
type Walker struct {
	nextID int
}

func New() *Walker {
	return &Walker{nextID: 1}
}

func (w *Walker) Reset() {
	w.nextID = 1
}

func (w *Walker) AllocID() int {
	id := w.nextID
	w.nextID++
	return id
}

// Visitor is implemented by DOM and SSR renderers. Each method receives the
// node (with its ID already set for element/text nodes) and a walkChild func
// that walks a single child node and returns its root ID.
type Visitor interface {
	VisitElement(id int, el *core.ElementNode, walkChild func(core.Node) int)
	VisitText(id int, tn *core.TextNode)
	VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int)
	VisitPortal(pn *core.PortalNode, walkChild func(core.Node) int)
	VisitErrorBoundary(ebn *core.ErrorBoundaryNode, walkInner func() int) int
	VisitComponentEnter(cn *core.ComponentNode)
	VisitComponentLeave(cn *core.ComponentNode, innerID int)
	VisitScopeEnter(sn *core.ScopeNode)
	VisitScopeLeave(sn *core.ScopeNode, innerID int)
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
		frame := core.PushComponent()
		inner := core.FlatTree(node.Render())
		id := w.Walk(inner, v)
		core.PopComponent()
		node.Prev = inner
		node.Frame = frame
		v.VisitComponentLeave(node, id)
		return id

	case *core.ScopeNode:
		if node == nil {
			return 0
		}
		v.VisitScopeEnter(node)
		inner := core.FlatTree(node.Render())
		id := w.Walk(inner, v)
		node.Prev = inner
		v.VisitScopeLeave(node, id)
		return id

	case *core.PortalNode:
		if node == nil {
			return 0
		}
		v.VisitPortal(node, func(child core.Node) int {
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
	}
	return 0
}
