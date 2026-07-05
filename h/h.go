package h

import "github.com/yogisalomo/goowee/core"

func El(tag string, items ...core.Item) *core.ElementNode {
	return ElNS(tag, "", items...)
}

func ElNS(tag string, ns string, items ...core.Item) *core.ElementNode {
	el := &core.ElementNode{Tag: tag, Namespace: ns}
	for _, it := range items {
		if it != nil {
			it.Apply(el)
		}
	}
	return el
}

func Fragment(children ...core.Node) *core.FragmentNode {
	return &core.FragmentNode{Children: children}
}

func Key(k any) core.Item { return keyItem{k} }

type keyItem struct{ k any }

func (k keyItem) Apply(el *core.ElementNode) { el.Key = k.k }

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
