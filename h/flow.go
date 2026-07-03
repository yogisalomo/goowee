package h

import (
	"goowee/core"
	"goowee/hooks"
)

func Show(cond *core.Signal[bool], then func() core.Node) core.Node {
	return ShowElse(cond, then, nil)
}

func ShowElse(cond *core.Signal[bool], then, otherwise func() core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		if cond.Get() {
			return orEmpty(then)
		}
		return orEmpty(otherwise)
	}, cond)
}

func Switch[T comparable](sig *core.Signal[T], cases map[T]func() core.Node, def func() core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		if fn, ok := cases[sig.Get()]; ok {
			return orEmpty(fn)
		}
		return orEmpty(def)
	}, sig)
}

func For[T any, K comparable](items *core.Signal[[]T], key func(T) K, render func(T) core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		xs := items.Get()
		children := make([]core.Node, 0, len(xs))
		for _, x := range xs {
			n := render(x)
			if el, ok := n.(*core.ElementNode); ok && el != nil {
				el.Key = key(x)
			}
			children = append(children, n)
		}
		return &core.FragmentNode{Children: children}
	}, items)
}

func orEmpty(fn func() core.Node) core.Node {
	if fn == nil {
		return &core.FragmentNode{}
	}
	if n := fn(); n != nil {
		return n
	}
	return &core.FragmentNode{}
}
