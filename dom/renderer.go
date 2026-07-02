package dom

import (
	"fmt"
	"goowee/core"
	"goowee/hooks"
)

type DOMRenderer struct {
	nextID    int
	Bindings  *core.BindingRegistry
	Scheduler *core.Scheduler
	Registry  *NodeRegistry
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
	return muts, rootID
}

func (r *DOMRenderer) renderNode(n core.Node, muts *[]core.Mutation) int {
	switch v := n.(type) {
	case *core.ElementNode:
		id := r.allocID()
		v.ID = id
		*muts = append(*muts, core.Mutation{
			Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: v.Tag,
		})
		for key, val := range v.Props {
			if key == "class" {
				key = "className"
			}
			switch actual := val.(type) {
			case string:
				if isProperty(key) {
					*muts = append(*muts, core.Mutation{
						Type: core.MutSetProperty, NodeID: id, Key: key, Value: actual,
					})
				} else {
					*muts = append(*muts, core.Mutation{
						Type: core.MutSetAttribute, NodeID: id, Key: key, Value: actual,
					})
				}
			case bool:
				if isProperty(key) {
					*muts = append(*muts, core.Mutation{
						Type: core.MutSetProperty, NodeID: id, Key: key, Value: actual,
					})
				}
			case core.SignalAccessor:
				r.Bindings.Bind(id, actual, key)
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: id, Key: key,
					Value: actual.Value(),
				})
			case func(core.EventData):
				if len(key) > 2 && key[:2] == "on" {
					r.Registry.RegisterHandler(id, key[2:], actual)
				}
			}
		}
		for _, child := range v.Children {
			childID := r.renderNode(child, muts)
			*muts = append(*muts, core.Mutation{
				Type: core.MutAppendChild, NodeID: id, ChildID: childID,
			})
		}
		return id

	case *core.TextNode:
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
			r.Bindings.Bind(id, val, "textContent")
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: id, Key: "textContent",
				Value: val.Value(),
			})
		}
		return id

	case *core.FragmentNode:
		for _, child := range v.Children {
			r.renderNode(child, muts)
		}
		return 0

	case *core.ComponentNode:
		frame := core.PushComponent()
		inner := v.Render()
		id := r.renderNode(inner, muts)
		frame.RootNodeIDs = append(frame.RootNodeIDs, id)
		core.PopComponent()
		return id

	case *core.ScopeNode:
		if v.Prev != nil {
			// Re-render: diff new tree vs stored old tree
			oldFrames := v.Frames
			v.Frames = nil

			newInner := v.Render()
			newTree, frames := core.FlatTreeWithFrames(newInner)
			v.Frames = frames

			var diffMuts []core.Mutation
			r.diffNode(v.Prev, newTree, &diffMuts)
			r.Scheduler.Enqueue(diffMuts...)

			// Run cleanup for old frames (components removed by diff)
			for _, frame := range oldFrames {
				hooks.RunFrameCleanup(frame)
			}

			v.Prev = newTree
			id := rootIDFromTree(newTree)
			return id
		}
		// First render
		inner := v.Render()
		flat, frames := core.FlatTreeWithFrames(inner)
		v.Frames = frames
		id := r.renderNode(flat, muts)
		v.Prev = flat
		v.Unsubs = make([]func(), len(v.Deps))
		for i, dep := range v.Deps {
			v.Unsubs[i] = dep.Subscribe(func() {
				r.reRenderScope(v)
			})
		}
		return id
	}
	return 0
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

func (r *DOMRenderer) reRenderScope(s *core.ScopeNode) {
	oldFrames := s.Frames
	s.Frames = nil

	newInner := s.Render()
	newTree, frames := core.FlatTreeWithFrames(newInner)
	s.Frames = frames

	var muts []core.Mutation
	r.diffNode(s.Prev, newTree, &muts)
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
			return r.renderNode(new, muts)
		}
		new.ID = old.ID

		for key, newVal := range new.Props {
			nkey := key
			if nkey == "class" {
				nkey = "className"
			}
			if s, isStr := newVal.(string); isStr {
				oldVal, exists := old.Props[key]
				if exists && oldVal == s {
					continue
				}
				if isProperty(nkey) {
					*muts = append(*muts, core.Mutation{
						Type: core.MutSetProperty, NodeID: old.ID, Key: nkey, Value: s,
					})
				} else {
					*muts = append(*muts, core.Mutation{
						Type: core.MutSetAttribute, NodeID: old.ID, Key: nkey, Value: s,
					})
				}
			} else if b, isBool := newVal.(bool); isBool && isProperty(nkey) {
				oldVal, exists := old.Props[key]
				if exists && oldVal == b {
					continue
				}
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: nkey, Value: b,
				})
			}
		}

		maxLen := len(old.Children)
		if len(new.Children) > maxLen {
			maxLen = len(new.Children)
		}
		for i := 0; i < maxLen; i++ {
			var oldChild, newChild core.Node
			if i < len(old.Children) {
				oldChild = old.Children[i]
			}
			if i < len(new.Children) {
				newChild = new.Children[i]
			}
			oldID := nodeID(oldChild)
			childID := r.diffNode(oldChild, newChild, muts)
			if oldChild == nil && newChild != nil {
				*muts = append(*muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: old.ID, ChildID: childID,
				})
			} else if oldChild != nil && newChild != nil && childID != oldID {
				*muts = append(*muts, core.Mutation{
					Type: core.MutInsertBefore, NodeID: old.ID, ChildID: childID,
					Key: fmt.Sprintf("%d", i),
				})
			}
		}
		if len(new.Children) < len(old.Children) {
			for i := len(new.Children); i < len(old.Children); i++ {
				r.emitRemoveTree(old.Children[i], muts)
			}
		}
		return old.ID

	case *core.TextNode:
		new, ok := newNode.(*core.TextNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(new, muts)
		}
		new.ID = old.ID
		oldStr, oldIsStr := old.Value.(string)
		newStr, newIsStr := new.Value.(string)
		if oldIsStr && newIsStr && oldStr != newStr {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
			})
		}
		return old.ID

	case *core.FragmentNode:
		new, ok := newNode.(*core.FragmentNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(new, muts)
		}
		maxLen := len(old.Children)
		if len(new.Children) > maxLen {
			maxLen = len(new.Children)
		}
		for i := 0; i < maxLen; i++ {
			var oldChild, newChild core.Node
			if i < len(old.Children) {
				oldChild = old.Children[i]
			}
			if i < len(new.Children) {
				newChild = new.Children[i]
			}
			r.diffNode(oldChild, newChild, muts)
		}
		if len(new.Children) < len(old.Children) {
			for i := len(new.Children); i < len(old.Children); i++ {
				r.emitRemoveTree(old.Children[i], muts)
			}
		}
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

func isProperty(key string) bool {
	switch key {
	case "textContent", "checked", "value", "className", "disabled", "selected", "innerText":
		return true
	default:
		return false
	}
}
