package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

func El(tag string, items ...core.Item) *core.ElementNode {
	el := &core.ElementNode{Tag: tag}
	for _, it := range items {
		if it != nil {
			it.Apply(el)
		}
	}
	return el
}

// Fragment groups nodes without a wrapper element: its children render in its
// place. Fragment() with no children renders nothing; Empty is the same node
// under a name that says so.
func Fragment(children ...core.Node) *core.FragmentNode {
	return &core.FragmentNode{Children: children}
}

// Empty is a node that renders nothing — the "nothing here" return value of a
// function that returns core.Node:
//
//	func badge(n int) core.Node {
//	    if n == 0 {
//	        return Empty()
//	    }
//	    return Span(Class("badge"), Textf("%d", n))
//	}
//
// It is Fragment() with no children. (Group returns a core.Item, which isn't
// a core.Node.)
func Empty() *core.FragmentNode { return &core.FragmentNode{} }

// Ref creates a handle to an element's DOM node. Attach it with RefTo and call
// ref.Focus()/Blur()/Click()/ScrollIntoView() from handlers or effects.
func Ref() *core.Ref { return &core.Ref{} }

// RefTo attaches ref to an element (nil ref is a no-op).
func RefTo(ref *core.Ref) core.Item {
	if ref == nil {
		return nil
	}
	return refItem{ref}
}

type refItem struct{ ref *core.Ref }

func (r refItem) Apply(el *core.ElementNode) { el.Ref = r.ref }

// Portal renders children into a different DOM container (a modal or tooltip
// outside the current subtree). target is a CSS selector, e.g. "#modal-root".
// Portals are client-side. On a re-render their content is reconciled like
// any children, so state inside a portal survives — see core.PortalNode /
// ADR-017.
func Portal(target string, children ...core.Node) *core.PortalNode {
	return &core.PortalNode{Target: target, Children: children}
}

// ErrorBoundary renders child, but if rendering it panics, renders
// fallback(err) instead so the failure doesn't blank the page. Use it around
// a feature or route subtree. See ADR-018 for what it does and doesn't catch.
//
//	h.ErrorBoundary(
//	    func(err any) core.Node { return P(Text("Something went wrong.")) },
//	    riskyView(),
//	)
func ErrorBoundary(fallback func(err any) core.Node, child core.Node) *core.ErrorBoundaryNode {
	return &core.ErrorBoundaryNode{Fallback: fallback, Child: child}
}

func Key(k any) core.Item { return keyItem{k} }

type keyItem struct{ k any }

func (k keyItem) Apply(el *core.ElementNode) { el.Key = k.k }

// Dynamic marks an element's subtree as non-deterministic between server and
// client (e.g. dates, locale, per-request content). Hydration then re-applies
// its attributes/properties/text so the client value wins, instead of trusting
// the server-rendered value. Use it sparingly — see ADR-016.
func Dynamic() core.Item { return dynamicItem{} }

type dynamicItem struct{}

func (dynamicItem) Apply(el *core.ElementNode) { el.Dynamic = true }

func If(cond bool, item core.Item) core.Item {
	if cond {
		return item
	}
	return nil
}

func IfElse(cond bool, a, b core.Item) core.Item {
	if cond {
		return a
	}
	return b
}

// Group bundles items — attributes, props, events, children — into one item
// that applies them all, e.g. to return several from a helper. It is a
// core.Item, not a core.Node: to group nodes into one node, use Fragment, and
// to render nothing, Empty.
func Group(items ...core.Item) core.Item { return groupItem(items) }

type groupItem []core.Item

func (g groupItem) Apply(el *core.ElementNode) {
	for _, it := range g {
		if it != nil {
			it.Apply(el)
		}
	}
}

func Map[T any](xs []T, fn func(T) core.Item) core.Item {
	g := make(groupItem, 0, len(xs))
	for _, x := range xs {
		g = append(g, fn(x))
	}
	return g
}

// Raw renders a pre-rendered HTML string into the DOM, inside a <div>,
// without parsing it into individual element nodes. Use for markdown output,
// CMS content, or other HTML generated elsewhere. Raw is static; RawS follows
// a signal.
//
// WARNING: The HTML is inserted as-is. Do not use with untrusted input without
// sanitization (e.g. bluemonday). The node does not participate in
// reconciliation — the entire content is replaced on re-render.
func Raw(html string) *core.RawNode {
	return &core.RawNode{HTML: html}
}

// RawS is Raw that follows sig: when sig changes, the wrapper <div> keeps its
// place and its content is replaced with the new HTML (nothing happens when
// the HTML is unchanged). Use it for HTML that changes after mount, such as a
// rendered message body whose state changes. The same warning as Raw applies:
// sanitize untrusted input.
func RawS(sig *core.Signal[string]) core.Node {
	return hooks.UseScope(func() core.Node { return Raw(sig.Get()) }, sig)
}

// Nodes adapts a []core.Node into items so a slice of already-built nodes
// can be spread into an element constructor: El("div", Nodes(children)...).
// Every core.Node is already a core.Item; this just changes the slice type.
func Nodes(ns []core.Node) []core.Item {
	items := make([]core.Item, len(ns))
	for i, n := range ns {
		items[i] = n
	}
	return items
}

type attrItem struct{ name, value string }

func (a attrItem) Apply(el *core.ElementNode) {
	for i := range el.Attrs {
		if el.Attrs[i].Name == a.name {
			if a.name == "class" {
				el.Attrs[i].Value += " " + a.value
			} else {
				el.Attrs[i].Value = a.value
			}
			return
		}
	}
	el.Attrs = append(el.Attrs, core.Attr{Name: a.name, Value: a.value})
}

type propItem struct {
	name  string
	value any
}

func (p propItem) Apply(el *core.ElementNode) {
	el.Props = append(el.Props, core.Prop{Name: p.name, Value: p.value})
}

type bindItem struct {
	target core.BindTarget
	name   string
	sig    core.SignalAccessor
}

func (b bindItem) Apply(el *core.ElementNode) {
	el.Binds = append(el.Binds, core.Bind{Target: b.target, Name: b.name, Signal: b.sig})
}

type handlerItem struct{ h core.Handler }

func (hi handlerItem) Apply(el *core.ElementNode) {
	el.Handlers = append(el.Handlers, hi.h)
}
