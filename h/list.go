package h

import "github.com/yogisalomo/goowee/core"

// ForList renders a core.List, one row per item, and applies each change
// the list records to its rows alone (#140): an Append mounts one row, an
// Update sets one row's item signal, a Remove unmounts one row and a Move
// moves one, with no comparison of the whole list. A Set (or a change the
// rows fell too far behind to replay) renders the list again, reusing rows
// by key, as For does.
//
// render gets the row's item as a signal: bind it (TextS of a Computed,
// BindAttr, …) so an Update changes only what depends on it.
//
//	ForList(todos, func(t *core.Signal[Todo]) core.Node {
//	    title := core.Computed([]core.SignalAccessor{t}, func() string { return t.Get().Title })
//	    return Li(TextS(title))
//	})
//
// Keys must be unique.
func ForList[K comparable, T any](list *core.List[K, T], render func(item *core.Signal[T]) core.Node) core.Node {
	type row struct {
		item *core.Signal[T]
		node core.Node
	}
	rows := map[K]*row{}
	var seen uint64 // the list version the rows show
	mount := func(item T) *row {
		k := list.Key(item)
		sig := core.NewSignal(item)
		r := &row{item: sig, node: core.SetKey(core.Component("ListRow", func() core.Node { return render(sig) }), k)}
		rows[k] = r
		return r
	}
	sn := &core.ScopeNode{Deps: []core.SignalAccessor{list}}
	sn.Render = func() core.Node {
		seen = list.Version()
		items := list.Items()
		kids := make([]core.Node, 0, len(items))
		next := make(map[K]*row, len(items))
		for _, it := range items {
			k := list.Key(it)
			r, ok := rows[k]
			if !ok || next[k] != nil {
				r = mount(it)
			} else {
				r.item.Set(it)
			}
			next[k] = r
			kids = append(kids, r.node)
		}
		rows = next
		return &core.FragmentNode{Children: kids}
	}
	sn.Patch = func(p core.ScopePatcher) bool {
		ops, ok := list.Changes(seen)
		if !ok {
			return false
		}
		for _, op := range ops {
			if op.Kind == core.ListReplace {
				return false
			}
		}
		for _, op := range ops {
			switch op.Kind {
			case core.ListInsert:
				p.Insert(op.Index, mount(op.Item).node)
			case core.ListUpdate:
				if r := rows[op.Key]; r != nil {
					r.item.Set(op.Item)
				}
			case core.ListRemove:
				p.Remove(op.Index)
				delete(rows, op.Key)
			case core.ListMove:
				p.Move(op.Index, op.To)
			}
		}
		seen = list.Version()
		return true
	}
	return sn
}
