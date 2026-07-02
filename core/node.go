package core

import "fmt"

type Node interface {
	nodeMarker()
	String() string
}

type ElementNode struct {
	ID       int
	Tag      string
	Props    map[string]any
	Children []Node
}

func (e *ElementNode) nodeMarker() {}
func (e *ElementNode) String() string {
	return fmt.Sprintf("Element(%s)", e.Tag)
}

type TextNode struct {
	ID    int
	Value any
}

func (t *TextNode) nodeMarker() {}
func (t *TextNode) String() string {
	return fmt.Sprintf("Text(%v)", t.Value)
}

type FragmentNode struct {
	Children []Node
}

func (f *FragmentNode) nodeMarker() {}
func (f *FragmentNode) String() string {
	return fmt.Sprintf("Fragment(%d children)", len(f.Children))
}

type ComponentNode struct {
	Name   string
	Render func() Node
	Prev   Node            // set by renderer: last rendered inner tree
	Frame  *ComponentFrame // set after each render
}

func (c *ComponentNode) nodeMarker() {}
func (c *ComponentNode) String() string {
	return fmt.Sprintf("Component(%s)", c.Name)
}

func Component(name string, render func() Node) *ComponentNode {
	return &ComponentNode{Name: name, Render: render}
}

type ScopeNode struct {
	Render func() Node
	Deps   []SignalAccessor
	Prev   Node              // set by renderer after each render (expanded tree with IDs)
	Frames []*ComponentFrame // component frames from last render, for cleanup
	Unsubs []func()          // signal subscription cancellations, set by renderer
}

func (s *ScopeNode) nodeMarker() {}
func (s *ScopeNode) String() string {
	return fmt.Sprintf("Scope(%d deps)", len(s.Deps))
}

func FlatTree(n Node) Node {
	return flatTree(n, nil)
}

func flatTree(n Node, frames *[]*ComponentFrame) Node {
	switch v := n.(type) {
	case *ElementNode:
		var flatChildren []Node
		for _, child := range v.Children {
			flattened := flatTree(child, frames)
			if frag, ok := flattened.(*FragmentNode); ok {
				flatChildren = append(flatChildren, frag.Children...)
			} else {
				flatChildren = append(flatChildren, flattened)
			}
		}
		v.Children = flatChildren
		return v
	case *TextNode:
		return v
	case *FragmentNode:
		var flatChildren []Node
		for _, child := range v.Children {
			flattened := flatTree(child, frames)
			if frag, ok := flattened.(*FragmentNode); ok {
				flatChildren = append(flatChildren, frag.Children...)
			} else {
				flatChildren = append(flatChildren, flattened)
			}
		}
		v.Children = flatChildren
		return v
	case *ComponentNode:
		frame := PushComponent()
		inner := v.Render()
		v.Frame = frame
		if frames != nil {
			*frames = append(*frames, frame)
		}
		result := flatTree(inner, frames)
		PopComponent()
		return result
	case *ScopeNode:
		return v
	}
	return nil
}

func CollectIDs(n Node) []int {
	var ids []int
	switch v := n.(type) {
	case *ElementNode:
		if v.ID > 0 {
			ids = append(ids, v.ID)
		}
		for _, child := range v.Children {
			ids = append(ids, CollectIDs(child)...)
		}
	case *TextNode:
		if v.ID > 0 {
			ids = append(ids, v.ID)
		}
	case *FragmentNode:
		for _, child := range v.Children {
			ids = append(ids, CollectIDs(child)...)
		}
	case *ComponentNode:
		if v.Prev != nil {
			ids = append(ids, CollectIDs(v.Prev)...)
		}
	case *ScopeNode:
		if v.Prev != nil {
			ids = append(ids, CollectIDs(v.Prev)...)
		}
	}
	return ids
}

// FlatTreeWithFrames is like FlatTree but also collects all ComponentFrame
// pointers created during flattening into the provided slice.
func FlatTreeWithFrames(n Node) (Node, []*ComponentFrame) {
	var frames []*ComponentFrame
	result := flatTree(n, &frames)
	return result, frames
}

type EventData struct {
	Type   string
	Target int
	Data   map[string]any
}
