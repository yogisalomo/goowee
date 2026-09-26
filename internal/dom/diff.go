package dom

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// Placement model. Every diff of a child list knows its DOM parent and an end
// reference: the DOM node that must stay right after the list (0 = the end of
// the parent). A scope's content is diffed with the scope's anchor as the end
// reference, so re-rendered content — several roots, or none — always lands in
// place. Children are diffed last-to-first so that, when a child is diffed,
// the nearest following child that was reused gives a stable reference for any
// placement it does internally; then a second pass inserts every fresh (or
// moved) child's roots before the root of the child after it.

// diffNode updates old in place to match nw, or replaces it. It reports fresh
// when nw was rendered from scratch (old removed): the caller must place
// rootIDs(nw). endRef is the stable DOM node after this child, used when the
// child is itself a sequence (a fragment or a boundary's fragment content).
func (r *DOMRenderer) diffNode(old, nw core.Node, parentID, endRef int, muts *[]core.Mutation) (fresh bool) {
	switch {
	case old == nil && nw == nil:
		return false
	case old == nil:
		r.renderFresh(nw, parentID, muts)
		return true
	case nw == nil:
		r.emitRemoveTree(old, muts)
		return false
	case old == nw:
		return false // the very same, already-mounted node: nothing changed
	}

	switch o := old.(type) {
	case *core.ElementNode:
		n, ok := nw.(*core.ElementNode)
		if !ok || o.Tag != n.Tag {
			return r.replace(old, nw, parentID, muts)
		}
		r.diffElement(o, n, muts)
		return false

	case *core.TextNode:
		n, ok := nw.(*core.TextNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		r.diffText(o, n, muts)
		return false

	case *core.RawNode:
		n, ok := nw.(*core.RawNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		n.ID = o.ID
		if o.HTML != n.HTML {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: o.ID, Key: "innerHTML", Value: n.HTML,
			})
		}
		return false

	case *core.FragmentNode:
		n, ok := nw.(*core.FragmentNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		core.FlatTree(n)
		r.diffChildren(parentID, o.Children, n.Children, endRef, muts)
		return false

	case *core.ComponentNode:
		n, ok := nw.(*core.ComponentNode)
		if !ok || o.Name != n.Name || !runtime.SafeEqual(o.Key, n.Key) {
			// A different component, or the same one under a different key:
			// unmount + mount.
			return r.replace(old, nw, parentID, muts)
		}
		// Same component: preserve it. Setup ran once at mount and the output
		// is a stable, self-updating subtree (bindings and inner scopes react
		// on their own), so we keep the frame and DOM untouched — this is what
		// gives components stable identity and state across scope re-renders.
		// A ComponentWithProps receives the new props through its signal.
		n.Adopt(o)
		if !n.HasProps() && n.Key == nil {
			warnPreservedPlainComponent(n.Name)
		}
		return false

	case *core.ScopeNode:
		n, ok := nw.(*core.ScopeNode)
		if !ok || r.scopes[o] == nil {
			return r.replace(old, nw, parentID, muts)
		}
		r.adoptScope(o, n, parentID, muts)
		return false

	case *core.MetadataNode:
		n, ok := nw.(*core.MetadataNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		// Head content is static; tear down the old tags and render the new.
		for _, c := range o.Children {
			r.emitRemoveTree(c, muts)
		}
		r.renderInto("head", n.Children, muts)
		return false

	case *core.PortalNode:
		n, ok := nw.(*core.PortalNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		// The target is a selector, not a node we can reconcile against, so
		// tear down the old portal content and render the new (ADR-017). Keep
		// portal state in signals outside the portal.
		for _, c := range o.Children {
			r.emitRemoveTree(c, muts)
		}
		r.renderInto(n.Target, n.Children, muts)
		return false

	case *core.ErrorBoundaryNode:
		n, ok := nw.(*core.ErrorBoundaryNode)
		if !ok {
			return r.replace(old, nw, parentID, muts)
		}
		return r.diffBoundary(o, n, parentID, endRef, muts)
	}
	return r.replace(old, nw, parentID, muts)
}

// replace removes old and renders nw fresh; the caller places it.
func (r *DOMRenderer) replace(old, nw core.Node, parentID int, muts *[]core.Mutation) bool {
	r.emitRemoveTree(old, muts)
	r.renderFresh(nw, parentID, muts)
	return true
}

func (r *DOMRenderer) diffElement(old, nw *core.ElementNode, muts *[]core.Mutation) {
	core.FlatTree(nw) // flattening is shallow; children are flattened as reached
	nw.ID = old.ID
	if nw.Ref != nil {
		nw.Ref.ID = old.ID // a ref created by this render points at the reused node
	}

	oldAttrs := map[string]string{}
	for _, a := range old.Attrs {
		oldAttrs[a.Name] = a.Value
	}
	for _, a := range nw.Attrs {
		if ov, ok := oldAttrs[a.Name]; !ok || ov != a.Value {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetAttribute, NodeID: old.ID, Key: a.Name, Value: a.Value,
			})
		}
		delete(oldAttrs, a.Name)
	}
	for name := range oldAttrs {
		*muts = append(*muts, core.Mutation{
			Type: core.MutRemoveAttribute, NodeID: old.ID, Key: name,
		})
	}

	oldProps := map[string]any{}
	for _, p := range old.Props {
		oldProps[p.Name] = p.Value
	}
	for _, p := range nw.Props {
		// SafeEqual: a slice/map prop value must count as changed, not
		// panic the differ (which would freeze the subtree).
		if ov, ok := oldProps[p.Name]; !ok || !runtime.SafeEqual(ov, p.Value) {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: p.Name, Value: p.Value,
			})
		}
		delete(oldProps, p.Name)
	}
	for name, ov := range oldProps {
		var zero any = ""
		if _, isBool := ov.(bool); isBool {
			zero = false
		}
		*muts = append(*muts, core.Mutation{
			Type: core.MutSetProperty, NodeID: old.ID, Key: name, Value: zero,
		})
	}

	r.Bindings.Unbind(old.ID)
	for _, b := range nw.Binds {
		r.Bindings.Bind(old.ID, b)
		*muts = append(*muts, runtime.MutationForBind(old.ID, b))
	}

	newEvents := map[string]bool{}
	for _, hd := range nw.Handlers {
		r.Registry.RegisterHandler(old.ID, hd.Event, hd.Fn, hd.Options)
		newEvents[hd.Event] = true
	}
	for _, hd := range old.Handlers {
		if !newEvents[hd.Event] {
			r.Registry.RemoveHandler(old.ID, hd.Event)
		}
	}

	r.parentStack = append(r.parentStack, old.ID)
	r.diffChildren(old.ID, old.Children, nw.Children, 0, muts)
	r.parentStack = r.parentStack[:len(r.parentStack)-1]
}

func (r *DOMRenderer) diffText(old, nw *core.TextNode, muts *[]core.Mutation) {
	nw.ID = old.ID
	oldStr, oldIsStr := old.Value.(string)
	newStr, newIsStr := nw.Value.(string)
	if oldIsStr && newIsStr {
		if oldStr != newStr {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
			})
		}
		return
	}
	r.Bindings.Unbind(old.ID)
	if newSig, ok := nw.Value.(core.SignalAccessor); ok {
		r.Bindings.Bind(old.ID, core.Bind{Target: core.BindToProp, Name: "textContent", Signal: newSig})
		*muts = append(*muts, core.Mutation{
			Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent",
			Value: runtime.TextValue(newSig.Value()),
		})
	} else if newIsStr {
		*muts = append(*muts, core.Mutation{
			Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
		})
	}
}

// diffBoundary updates a boundary on re-render by diffing its rendered subtree
// against the new child (preserving child state). If that panics, the partial
// work is released, the previous subtree removed, and the fallback rendered in
// its place — a half-applied diff can't be trusted to keep working. When the
// previous render was the fallback, a successful diff restores the child.
func (r *DOMRenderer) diffBoundary(old, nb *core.ErrorBoundaryNode, parentID, endRef int, muts *[]core.Mutation) (fresh bool) {
	baseStack := len(r.parentStack)
	frameDepth := runtime.SaveFrameStack()
	savedNS, savedDyn := r.currentNS, r.hydrateDynamic
	mark := r.beginRecoverable()
	owner := runtime.PushOwner()

	var childMuts []core.Mutation
	rec := func() (rec any) {
		defer func() { rec = recover() }()
		fresh = r.diffNode(old.Prev, nb.Child, parentID, endRef, &childMuts)
		return
	}()
	if rec != nil {
		r.parentStack = r.parentStack[:baseStack]
		runtime.RestoreFrameStack(frameDepth)
		r.currentNS, r.hydrateDynamic = savedNS, savedDyn
		r.abandon(mark, owner)
		core.Log(core.LogRecoverErrorBoundary, "panic during boundary update; rendering fallback", map[string]any{
			"phase": "update",
			"panic": rec,
		})
		r.emitRemoveTree(old.Prev, muts)
		fb := core.FlatTree(nb.Fallback(rec))
		nb.Prev = fb
		r.renderFresh(fb, parentID, muts)
		return true
	}
	runtime.PopComponent() // owner
	r.endRecoverable()
	*muts = append(*muts, childMuts...)
	nb.Prev = nb.Child
	return fresh
}

// place inserts roots, in order, before ref (0 = at the end of parentID).
func place(parentID int, roots []int, ref int, muts *[]core.Mutation) {
	for _, id := range roots {
		*muts = append(*muts, core.Mutation{
			Type: core.MutInsertBefore, NodeID: parentID, ChildID: id, RefID: ref,
		})
	}
}

func (r *DOMRenderer) diffChildren(parentID int, old, new []core.Node, endRef int, muts *[]core.Mutation) {
	if hasAnyKey(old) || hasAnyKey(new) {
		r.diffChildrenKeyed(parentID, old, new, endRef, muts)
		return
	}
	r.diffChildrenPositional(parentID, old, new, endRef, muts)
}

// keyOf returns a node's reconciliation key, if it carries one (elements and
// components). Keyed matching therefore works uniformly whether list rows are
// elements or components.
func keyOf(n core.Node) (any, bool) {
	var k any
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil {
			k = v.Key
		}
	case *core.ComponentNode:
		if v != nil {
			k = v.Key
		}
	}
	if k == nil {
		return nil, false
	}
	if !runtime.Comparable(k) {
		// Keys index a map; an uncomparable key (slice, map, …) would panic.
		// Treat the row as unkeyed instead and say so once per key type.
		warnUncomparableKey(k)
		return nil, false
	}
	return k, true
}

func hasAnyKey(nodes []core.Node) bool {
	for _, n := range nodes {
		if _, ok := keyOf(n); ok {
			return true
		}
	}
	return false
}

func (r *DOMRenderer) diffChildrenPositional(parentID int, old, new []core.Node, endRef int, muts *[]core.Mutation) {
	for i := len(new); i < len(old); i++ {
		r.emitRemoveTree(old[i], muts)
	}
	fresh := make([]bool, len(new))
	ref := endRef
	for i := len(new) - 1; i >= 0; i-- {
		var oldChild core.Node
		if i < len(old) {
			oldChild = old[i]
		}
		fresh[i] = r.diffNode(oldChild, new[i], parentID, ref, muts)
		if !fresh[i] {
			if id := firstRoot(new[i]); id != 0 {
				ref = id
			}
		}
	}
	// Place fresh children. Reused children keep their position under
	// positional matching, so they never move.
	ref = endRef
	for i := len(new) - 1; i >= 0; i-- {
		if fresh[i] {
			place(parentID, rootIDs(new[i]), ref, muts)
		}
		if id := firstRoot(new[i]); id != 0 {
			ref = id
		}
	}
}

func (r *DOMRenderer) diffChildrenKeyed(parentID int, old, new []core.Node, endRef int, muts *[]core.Mutation) {
	oldByKey := map[any]int{}
	oldUnkeyed := []int{}
	for i, n := range old {
		if k, ok := keyOf(n); ok {
			if _, dup := oldByKey[k]; dup {
				core.Log(core.LogWarn, "duplicate key in keyed children; treating extra as unkeyed", map[string]any{
					"key": k,
				})
				continue
			}
			oldByKey[k] = i
		} else {
			oldUnkeyed = append(oldUnkeyed, i)
		}
	}

	paired := make([]bool, len(old))
	oldIndexForNew := make([]int, len(new))

	unkeyedIdx := 0
	for i, n := range new {
		oldIndexForNew[i] = -1
		if k, ok := keyOf(n); ok {
			if oldIdx, ok := oldByKey[k]; ok {
				if paired[oldIdx] {
					core.Log(core.LogWarn, "duplicate key in keyed children; treating extra as unkeyed", map[string]any{
						"key": k,
					})
					continue
				}
				paired[oldIdx] = true
				oldIndexForNew[i] = oldIdx
			}
		} else {
			for unkeyedIdx < len(oldUnkeyed) {
				oi := oldUnkeyed[unkeyedIdx]
				unkeyedIdx++
				if !paired[oi] && typeCompatible(old[oi], n) {
					paired[oi] = true
					oldIndexForNew[i] = oi
					break
				}
			}
		}
	}

	for i, n := range old {
		if !paired[i] {
			r.emitRemoveTree(n, muts)
		}
	}

	fresh := make([]bool, len(new))
	ref := endRef
	for i := len(new) - 1; i >= 0; i-- {
		var oldChild core.Node
		if oldIndexForNew[i] >= 0 {
			oldChild = old[oldIndexForNew[i]]
		}
		fresh[i] = r.diffNode(oldChild, new[i], parentID, ref, muts)
		if !fresh[i] {
			if id := firstRoot(new[i]); id != 0 {
				ref = id
			}
		}
	}

	// LIS optimisation: compute which existing elements already appear in the
	// correct relative order and skip DOM moves for them. Only elements NOT in
	// the LIS, fresh ones, and reused slots whose node was replaced (fresh)
	// need inserting.
	needsMove := computeNeedsMove(oldIndexForNew)

	ref = endRef
	for i := len(new) - 1; i >= 0; i-- {
		if needsMove[i] || fresh[i] {
			place(parentID, rootIDs(new[i]), ref, muts)
		}
		if id := firstRoot(new[i]); id != 0 {
			ref = id
		}
	}
}

// computeNeedsMove returns a boolean slice parallel to oldIndexForNew where
// true means the element at that new position needs a DOM mutation. New
// elements (oldIdx == -1) always need a move; existing elements whose old
// index is NOT part of the longest increasing subsequence (by old index,
// ordered by new position) need a move; the rest stay in place.
func computeNeedsMove(oldIndexForNew []int) []bool {
	n := len(oldIndexForNew)
	needsMove := make([]bool, n)

	// Extract kept (existing) old indices in new-list order.
	kept := make([]int, 0, n)
	keptPos := make([]int, 0, n)
	for i, oldIdx := range oldIndexForNew {
		if oldIdx >= 0 {
			kept = append(kept, oldIdx)
			keptPos = append(keptPos, i)
		} else {
			needsMove[i] = true // new element, always needs a move
		}
	}
	if len(kept) == 0 {
		return needsMove
	}

	// LIS over kept old indices: which are already in the correct order?
	inLIS := lis(kept)

	for i, in := range inLIS {
		if !in {
			needsMove[keptPos[i]] = true
		}
	}
	return needsMove
}

// lis marks one longest strictly increasing subsequence of arr (the kept rows'
// old indices, in new order): true = the row is already in the right relative
// order and needn't move. O(n log n) patience sorting with predecessor links,
// plus an O(n) fast path for the common case where nothing moved (appends,
// removals, in-place edits).
func lis(arr []int) []bool {
	n := len(arr)
	in := make([]bool, n)
	if n == 0 {
		return in
	}
	sorted := true
	for i := 1; i < n; i++ {
		if arr[i] <= arr[i-1] {
			sorted = false
			break
		}
	}
	if sorted {
		for i := range in {
			in[i] = true
		}
		return in
	}
	tails := make([]int, 0, n) // tails[l] = index of the smallest tail of an increasing run of length l+1
	prev := make([]int, n)     // predecessor of arr[i] in its run, -1 at the start
	for i, v := range arr {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if arr[tails[mid]] < v {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		prev[i] = -1
		if lo > 0 {
			prev[i] = tails[lo-1]
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	for k := tails[len(tails)-1]; k >= 0; k = prev[k] {
		in[k] = true
	}
	return in
}

func typeCompatible(a, b core.Node) bool {
	if a == nil || b == nil {
		return false
	}
	switch va := a.(type) {
	case *core.ElementNode:
		vb, ok := b.(*core.ElementNode)
		return ok && va.Tag == vb.Tag
	case *core.TextNode:
		_, ok := b.(*core.TextNode)
		return ok
	case *core.RawNode:
		_, ok := b.(*core.RawNode)
		return ok
	case *core.FragmentNode:
		_, ok := b.(*core.FragmentNode)
		return ok
	case *core.ScopeNode:
		_, ok := b.(*core.ScopeNode)
		return ok
	case *core.ComponentNode:
		vb, ok := b.(*core.ComponentNode)
		return ok && va.Name == vb.Name
	}
	return false
}
