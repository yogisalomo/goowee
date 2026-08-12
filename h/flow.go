package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

// Show renders then() while cond is true and nothing otherwise. It
// re-renders whenever cond changes.
func Show(cond *core.Signal[bool], then func() core.Node) core.Node {
	return ShowElse(cond, then, nil)
}

// ShowElse renders then() while cond is true, otherwise() when false.
func ShowElse(cond *core.Signal[bool], then, otherwise func() core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		if cond.Get() {
			return orEmpty(then)
		}
		return orEmpty(otherwise)
	}, cond)
}

// Switch re-renders on sig changes, choosing cases[sig.Get()] or def.
func Switch[T comparable](sig *core.Signal[T], cases map[T]func() core.Node, def func() core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		if fn, ok := cases[sig.Get()]; ok {
			return orEmpty(fn)
		}
		return orEmpty(def)
	}, sig)
}

// For renders a keyed reactive list. key must return a unique, stable value
// per item — it becomes the row's Key so the DOM differ can reuse and reorder
// rows instead of rebuilding them. render should return an *ElementNode or a
// *ComponentNode; other node types are rendered but matched positionally.
//
// Rows keep their identity across list changes: as long as an item's key
// survives, its DOM nodes are reused (preserving focus and scroll) and a
// component row keeps its frame — setup does not re-run, so its state and
// effects survive appends, removals, and reorders. A row whose key disappears
// is unmounted and disposed exactly once.
func For[T any, K comparable](items *core.Signal[[]T], key func(T) K, render func(T) core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		xs := items.Get()
		children := make([]core.Node, 0, len(xs))
		for _, x := range xs {
			n := render(x)
			core.SetKey(n, key(x))
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
