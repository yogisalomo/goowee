package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/runtime"
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
//
// render runs only for new keys and for items that changed (compared with ==;
// slices/maps count as changed). An unchanged item reuses its row as-is, so a
// list update costs work proportional to what changed. When the item behind a
// surviving key changes:
//
//   - an element row is re-rendered and diffed in place;
//   - a core.ComponentWithProps row keeps its state and receives the new item
//     through its props signal;
//   - a plain core.Component row is remounted, since it can only show the
//     values it was created with.
func For[T any, K comparable](items *core.Signal[[]T], key func(T) K, render func(T) core.Node) core.Node {
	type row struct {
		item T
		node core.Node
		gen  int // bumped to remount a plain component row whose item changed
	}
	var rows map[K]*row
	return hooks.UseScope(func() core.Node {
		xs := items.Get()
		next := make(map[K]*row, len(xs))
		children := make([]core.Node, 0, len(xs))
		for _, x := range xs {
			k := key(x)
			if _, dup := next[k]; dup {
				// Duplicate key: render it uncached; the differ warns and
				// matches the extra row as unkeyed.
				n := render(x)
				core.SetKey(n, k)
				children = append(children, n)
				continue
			}
			prev := rows[k]
			if prev != nil && runtime.SafeEqual(any(prev.item), any(x)) {
				next[k] = prev // unchanged: the mounted row, untouched
				children = append(children, prev.node)
				continue
			}
			n := render(x)
			gen := 0
			if prev != nil {
				gen = prev.gen
				if c, ok := n.(*core.ComponentNode); ok && !c.HasProps() {
					gen++
				}
			}
			var rowKey any = k
			if gen > 0 {
				rowKey = remountKey[K]{k, gen}
			}
			core.SetKey(n, rowKey)
			next[k] = &row{item: x, node: n, gen: gen}
			children = append(children, n)
		}
		rows = next
		return &core.FragmentNode{Children: children}
	}, items)
}

// remountKey is the key of a plain component row whose item changed: a new
// key makes the differ unmount the stale row and mount a fresh one in place.
type remountKey[K comparable] struct {
	key K
	gen int
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
