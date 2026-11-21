package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/runtime"
	"reflect"
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
			return keyBranch(orEmpty(then), branchThen)
		}
		return keyBranch(orEmpty(otherwise), branchElse)
	}, cond)
}

// Switch re-renders on sig changes, choosing cases[sig.Get()] or def.
func Switch[T comparable](sig *core.Signal[T], cases map[T]func() core.Node, def func() core.Node) core.Node {
	return hooks.UseScope(func() core.Node {
		v := sig.Get()
		if fn, ok := cases[v]; ok {
			return keyBranch(orEmpty(fn), switchCase{v})
		}
		return keyBranch(orEmpty(def), branchDefault)
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
				var dupKey any = k
				if c, ok := n.(*core.ComponentNode); ok && !c.HasProps() && comparableValue(x) {
					dupKey = itemKey{k, any(x)} // as below: remount when the item changes
				}
				core.SetKey(n, dupKey)
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
			if c, ok := n.(*core.ComponentNode); ok && !c.HasProps() && comparableValue(x) {
				// A plain component row captures its item at mount, so it
				// must remount when the item changes. Keying it by the item
				// too does that even for a For built afresh (inside a parent
				// row that re-rendered), which has no gen history (#161).
				rowKey = itemKey{k, any(x)}
			} else if gen > 0 {
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

// itemKey keys a plain component row by its key and its item.
type itemKey struct {
	key  any
	item any
}

// comparableValue reports whether v can be compared with ==, as a key must.
func comparableValue(v any) bool {
	return v != nil && reflect.ValueOf(v).Comparable()
}

// Branch keys: the content of a ShowElse or Switch branch is keyed by the
// branch, so switching branches mounts the new branch's content even when
// both render a component of the same name (#161, found by FuzzDiffer; the
// router does the same for routes, #124). A key the app set is kept inside.
type branchKey struct {
	branch any
	key    any
}

type switchCase struct{ v any }

const (
	branchThen    = "then"
	branchElse    = "else"
	branchDefault = "default"
)

func keyBranch(n core.Node, branch any) core.Node {
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil {
			v.Key = branchKey{branch, v.Key}
		}
	case *core.ComponentNode:
		if v != nil {
			v.Key = branchKey{branch, v.Key}
		}
	case *core.FragmentNode:
		if v != nil {
			for i, c := range v.Children {
				v.Children[i] = keyBranch(c, branch)
			}
		}
	case *core.ScopeNode, *core.PortalNode, *core.ErrorBoundaryNode:
		// These have no key: wrap them in a component that does, so another
		// branch's scope (a For, say), portal or boundary isn't adopted in
		// their place. Within one branch the wrapper is kept, and the content
		// updates itself (a scope re-renders, a portal's content is reactive).
		inner := n
		return core.SetKey(core.Component("Branch", func() core.Node { return inner }), branchKey{branch, nil})
	}
	return n
}
