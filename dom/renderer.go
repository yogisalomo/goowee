package dom

import (
	"goowee/core"
	"goowee/hooks"
)

type DOMRenderer struct {
	nextID      int
	Bindings    *core.BindingRegistry
	Scheduler   *core.Scheduler
	Registry    *NodeRegistry
	parentStack []int
}

func New() *DOMRenderer {
	sched := core.NewScheduler()
	return &DOMRenderer{
		nextID:    1,
		Scheduler: sched,
		Bindings:  core.NewBindingRegistry(sched),
		Registry:  NewNodeRegistry(),
	}
}

func (r *DOMRenderer) Reset() {
	r.Scheduler = core.NewScheduler()
	r.nextID = 1
	r.Bindings = core.NewBindingRegistry(r.Scheduler)
	r.Registry = NewNodeRegistry()
}

func (r *DOMRenderer) allocID() int {
	id := r.nextID
	r.nextID++
	return id
}

func (r *DOMRenderer) Render(n core.Node) ([]core.Mutation, int) {
	var muts []core.Mutation
	rootID := r.renderNode(n, &muts)
	if rootID != 0 {
		muts = append(muts, core.Mutation{
			Type: core.MutAppendChild, NodeID: 0, ChildID: rootID,
		})
	} else if frag, ok := n.(*core.FragmentNode); ok {
		for _, c := range frag.Children {
			if id := nodeID(c); id != 0 {
				muts = append(muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: 0, ChildID: id,
				})
			}
		}
	}
	return muts, rootID
}

func (r *DOMRenderer) renderNode(n core.Node, muts *[]core.Mutation) int {
	switch v := n.(type) {
	case *core.ElementNode:
		if v == nil {
			return 0
		}
		id := r.allocID()
		v.ID = id
		*muts = append(*muts, core.Mutation{
			Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: v.Tag,
		})

		for _, a := range v.Attrs {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetAttribute, NodeID: id, Key: a.Name, Value: a.Value,
			})
		}
		for _, p := range v.Props {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: id, Key: p.Name, Value: p.Value,
			})
		}
		for _, b := range v.Binds {
			r.Bindings.Bind(id, b)
			*muts = append(*muts, bindMutation(id, b))
		}
		for _, hd := range v.Handlers {
			r.Registry.RegisterHandler(id, hd.Event, hd.Fn, hd.Options)
		}
		r.parentStack = append(r.parentStack, id)
		for _, child := range v.Children {
			childID := r.renderNode(child, muts)
			if childID != 0 {
				*muts = append(*muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: id, ChildID: childID,
				})
			}
		}
		r.parentStack = r.parentStack[:len(r.parentStack)-1]
		return id

	case *core.TextNode:
		if v == nil {
			return 0
		}
		id := r.allocID()
		v.ID = id
		*muts = append(*muts, core.Mutation{
			Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: "#text",
		})
		switch val := v.Value.(type) {
		case string:
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: val,
			})
		case core.SignalAccessor:
			r.Bindings.Bind(id, core.Bind{
				Target: core.BindToProp, Name: "textContent", Signal: val,
			})
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: id, Key: "textContent",
				Value: val.Value(),
			})
		}
		return id

	case *core.FragmentNode:
		if v == nil {
			return 0
		}
		for _, child := range v.Children {
			r.renderNode(child, muts)
		}
		return 0

	case *core.ComponentNode:
		if v == nil {
			return 0
		}
		frame := core.PushComponent()
		inner := v.Render()
		id := r.renderNode(inner, muts)
		frame.RootNodeIDs = append(frame.RootNodeIDs, id)
		core.PopComponent()
		return id

	case *core.ScopeNode:
		if v == nil {
			return 0
		}
		if v.Prev != nil {
			oldFrames := v.Frames
			v.Frames = nil

			newInner := v.Render()
			newTree, frames := core.FlatTreeWithFrames(newInner)
			v.Frames = frames

			var diffMuts []core.Mutation
			r.diffNode(v.Prev, newTree, &diffMuts)
			r.Scheduler.Enqueue(diffMuts...)

			for _, frame := range oldFrames {
				hooks.RunFrameCleanup(frame)
			}

			v.Prev = newTree
			id := rootIDFromTree(newTree)
			return id
		}

		inner := v.Render()
		flat, frames := core.FlatTreeWithFrames(inner)
		v.Frames = frames
		id := r.renderNode(flat, muts)
		v.Prev = flat
		scopeParentID := 0
		if len(r.parentStack) > 0 {
			scopeParentID = r.parentStack[len(r.parentStack)-1]
		}
		v.Unsubs = make([]func(), len(v.Deps))
		for i, dep := range v.Deps {
			depIdx := i
			_ = depIdx
			v.Unsubs[i] = dep.Subscribe(func() {
				r.reRenderScope(v, scopeParentID)
			})
		}
		return id
	}
	return 0
}

func bindMutation(nodeID int, b core.Bind) core.Mutation {
	if b.Target == core.BindToAttr {
		return core.Mutation{
			Type: core.MutSetAttribute, NodeID: nodeID,
			Key: b.Name, Value: b.Signal.Value(),
		}
	}
	return core.Mutation{
		Type: core.MutSetProperty, NodeID: nodeID,
		Key: b.Name, Value: b.Signal.Value(),
	}
}

func nodeID(n core.Node) int {
	if n == nil {
		return 0
	}
	switch v := n.(type) {
	case *core.ElementNode:
		return v.ID
	case *core.TextNode:
		return v.ID
	}
	return 0
}

func rootIDFromTree(n core.Node) int {
	switch v := n.(type) {
	case *core.ElementNode:
		return v.ID
	case *core.TextNode:
		return v.ID
	case *core.FragmentNode:
		if len(v.Children) > 0 {
			return rootIDFromTree(v.Children[0])
		}
	}
	return 0
}

func rootTypeChanged(old, new core.Node) bool {
	if old == nil || new == nil {
		return old != new
	}
	switch a := old.(type) {
	case *core.ElementNode:
		b, ok := new.(*core.ElementNode)
		return !ok || a.Tag != b.Tag
	case *core.TextNode:
		_, ok := new.(*core.TextNode)
		return !ok
	case *core.FragmentNode:
		_, ok := new.(*core.FragmentNode)
		return !ok
	case *core.ScopeNode:
		_, ok := new.(*core.ScopeNode)
		return !ok
	}
	return true
}

func (r *DOMRenderer) reRenderScope(s *core.ScopeNode, parentID int) {
	oldFrames := s.Frames
	s.Frames = nil

	newInner := s.Render()
	newTree, frames := core.FlatTreeWithFrames(newInner)
	s.Frames = frames

	var muts []core.Mutation

	if rootTypeChanged(s.Prev, newTree) {
		newRootID := r.renderNode(newTree, &muts)
		oldRootID := rootIDFromTree(s.Prev)
		if oldRootID != 0 {
			if parentID != 0 {
				muts = append(muts, core.Mutation{
					Type: core.MutInsertBefore, NodeID: parentID,
					ChildID: newRootID, RefID: oldRootID,
				})
			} else {
				muts = append(muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: 0, ChildID: newRootID,
				})
			}
		}
		if s.Prev != nil {
			r.emitRemoveTree(s.Prev, &muts)
		}
	} else {
		r.diffNode(s.Prev, newTree, &muts)
	}

	if len(muts) > 0 {
		r.Scheduler.Enqueue(muts...)
	}

	for _, frame := range oldFrames {
		hooks.RunFrameCleanup(frame)
	}

	s.Prev = newTree
}

func (r *DOMRenderer) diffNode(oldNode, newNode core.Node, muts *[]core.Mutation) int {
	if oldNode == nil && newNode == nil {
		return 0
	}
	if oldNode == nil {
		return r.renderNode(newNode, muts)
	}
	if newNode == nil {
		r.emitRemoveTree(oldNode, muts)
		return 0
	}

	switch old := oldNode.(type) {
	case *core.ElementNode:
		new, ok := newNode.(*core.ElementNode)
		if !ok || old.Tag != new.Tag {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		new.ID = old.ID

		oldAttrs := map[string]string{}
		for _, a := range old.Attrs {
			oldAttrs[a.Name] = a.Value
		}
		for _, a := range new.Attrs {
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
		for _, p := range new.Props {
			if ov, ok := oldProps[p.Name]; !ok || ov != p.Value {
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
		for _, b := range new.Binds {
			r.Bindings.Bind(old.ID, b)
			*muts = append(*muts, core.MutationForBind(old.ID, b))
		}

		newEvents := map[string]bool{}
		for _, hd := range new.Handlers {
			r.Registry.RegisterHandler(old.ID, hd.Event, hd.Fn, hd.Options)
			newEvents[hd.Event] = true
		}
		for _, hd := range old.Handlers {
			if !newEvents[hd.Event] {
				r.Registry.RemoveHandler(old.ID, hd.Event)
			}
		}

		r.diffChildren(old.ID, old.Children, new.Children, muts)
		return old.ID

	case *core.TextNode:
		new, ok := newNode.(*core.TextNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		new.ID = old.ID

		oldStr, oldIsStr := old.Value.(string)
		newStr, newIsStr := new.Value.(string)
		if oldIsStr && newIsStr && oldStr != newStr {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
			})
			return old.ID
		}

		if oldIsStr != newIsStr || (!oldIsStr && !newIsStr) {
			r.Bindings.Unbind(old.ID)
			if newSig, ok := new.Value.(core.SignalAccessor); ok {
				r.Bindings.Bind(old.ID, core.Bind{
					Target: core.BindToProp, Name: "textContent", Signal: newSig,
				})
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent",
					Value: newSig.Value(),
				})
			} else if newStr, ok := new.Value.(string); ok {
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
				})
			}
		}
		return old.ID

	case *core.FragmentNode:
		new, ok := newNode.(*core.FragmentNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		r.diffChildren(0, old.Children, new.Children, muts)
		return 0

	case *core.ScopeNode:
		if old == newNode {
			return rootIDFromTree(old.Prev)
		}
		if old.Prev != nil {
			r.emitRemoveTree(old.Prev, muts)
		}
		for _, unsub := range old.Unsubs {
			if unsub != nil {
				unsub()
			}
		}
		old.Unsubs = nil
		newScope, ok := newNode.(*core.ScopeNode)
		if !ok {
			return 0
		}
		return r.renderNode(newScope, muts)
	}
	return 0
}

func (r *DOMRenderer) diffChildren(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	hasKeys := hasAnyKey(old) || hasAnyKey(new)
	if hasKeys {
		r.diffChildrenKeyed(parentID, old, new, muts)
		return
	}
	r.diffChildrenPositional(parentID, old, new, muts)
}

func hasAnyKey(nodes []core.Node) bool {
	for _, n := range nodes {
		if el, ok := n.(*core.ElementNode); ok && el != nil && el.Key != nil {
			return true
		}
	}
	return false
}

func (r *DOMRenderer) diffChildrenPositional(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	var created []bool
	var ids []int

	maxLen := len(old)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	for i := 0; i < maxLen; i++ {
		var oldChild, newChild core.Node
		if i < len(old) {
			oldChild = old[i]
		}
		if i < len(new) {
			newChild = new[i]
		}

		oldID := nodeID(oldChild)
		childID := r.diffNode(oldChild, newChild, muts)
		created = append(created, oldChild == nil && newChild != nil)
		ids = append(ids, childID)

		if newChild == nil && oldChild != nil {
			created[i] = false
		} else if childID != oldID {
			created[i] = true
		}
	}

	if len(new) < len(old) {
		for i := len(new); i < len(old); i++ {
			r.emitRemoveTree(old[i], muts)
		}
	}

	refID := 0
	for i := len(new) - 1; i >= 0; i-- {
		if created[i] && ids[i] != 0 {
			*muts = append(*muts, core.Mutation{
				Type: core.MutInsertBefore, NodeID: parentID,
				ChildID: ids[i], RefID: refID,
			})
		}
		if ids[i] != 0 {
			refID = ids[i]
		}
	}

	for i := 0; i < maxLen; i++ {
		if i < len(old) && i < len(new) {
			if !created[i] && nodeID(new[i]) != nodeID(old[i]) {
				refID := 0
				for j := i + 1; j < len(new); j++ {
					if ids[j] != 0 {
						refID = ids[j]
						break
					}
				}
				*muts = append(*muts, core.Mutation{
					Type: core.MutInsertBefore, NodeID: parentID,
					ChildID: ids[i], RefID: refID,
				})
			}
		}
	}
}

func (r *DOMRenderer) diffChildrenKeyed(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	oldByKey := map[any]int{}
	oldUnkeyed := []int{}
	for i, n := range old {
		if el, ok := n.(*core.ElementNode); ok && el != nil && el.Key != nil {
			if _, dup := oldByKey[el.Key]; dup {
				continue
			}
			oldByKey[el.Key] = i
		} else {
			oldUnkeyed = append(oldUnkeyed, i)
		}
	}

	paired := make([]bool, len(old))
	oldIndexForNew := make([]int, len(new))
	newIDs := make([]int, len(new))
	created := make([]bool, len(new))

	unkeyedIdx := 0
	for i, n := range new {
		oldIndexForNew[i] = -1
		if el, ok := n.(*core.ElementNode); ok && el != nil && el.Key != nil {
			if oldIdx, ok := oldByKey[el.Key]; ok {
				if paired[oldIdx] {
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

	for i := len(new) - 1; i >= 0; i-- {
		var oldChild core.Node
		if oldIndexForNew[i] >= 0 {
			oldChild = old[oldIndexForNew[i]]
		}
		childID := r.diffNode(oldChild, new[i], muts)
		newIDs[i] = childID
		created[i] = oldChild == nil || nodeID(oldChild) != childID
	}

	for i, n := range old {
		if !paired[i] {
			r.emitRemoveTree(n, muts)
		}
	}

	refID := 0
	for i := len(new) - 1; i >= 0; i-- {
		if newIDs[i] != 0 {
			*muts = append(*muts, core.Mutation{
				Type: core.MutInsertBefore, NodeID: parentID,
				ChildID: newIDs[i], RefID: refID,
			})
			refID = newIDs[i]
		}
	}
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
	case *core.FragmentNode:
		_, ok := b.(*core.FragmentNode)
		return ok
	case *core.ScopeNode:
		_, ok := b.(*core.ScopeNode)
		return ok
	}
	return false
}

func (r *DOMRenderer) emitRemoveTree(n core.Node, muts *[]core.Mutation) {
	ids := core.CollectIDs(n)
	for _, id := range ids {
		r.Bindings.Unbind(id)
		r.Registry.Remove(id)
		*muts = append(*muts, core.Mutation{
			Type: core.MutRemoveNode, NodeID: id,
		})
	}
}
