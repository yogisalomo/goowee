package core

import (
	"strings"
	"testing"
)

func TestSignal(t *testing.T) {
	s := NewSignal(0)
	if s.Get() != 0 {
		t.Fatal("expected 0")
	}
	s.Set(42)
	if s.Get() != 42 {
		t.Fatal("expected 42")
	}
}

func TestSignalSubscribe(t *testing.T) {
	s := NewSignal(0)
	var called bool
	unsub := s.Subscribe(func() { called = true })
	s.Set(1)
	if !called {
		t.Fatal("expected subscriber to be called")
	}
	called = false
	unsub()
	s.Set(2)
	if called {
		t.Fatal("expected unsubscribed subscriber to not be called")
	}
}

func TestSignalAccessor(t *testing.T) {
	s := NewSignal("hello")
	var acc SignalAccessor = s
	if acc.Value() != "hello" {
		t.Fatal("SignalAccessor.Value() failed")
	}
}

func TestBindingRegistry(t *testing.T) {
	s := NewSignal(0)
	sched := NewScheduler()
	reg := NewBindingRegistry(sched)
	reg.Bind(1, s, "textContent")
	muts := reg.GetMutationsFor(1)
	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	if muts[0].Value != 0 {
		t.Fatalf("expected 0, got %v", muts[0].Value)
	}
	s.Set(42)
	muts = reg.GetMutationsFor(1)
	if muts[0].Value != 42 {
		t.Fatalf("expected 42, got %v", muts[0].Value)
	}
}

func TestScheduler(t *testing.T) {
	sched := NewScheduler()
	if sched.Len() != 0 {
		t.Fatal("expected empty scheduler")
	}
	sched.Enqueue(Mutation{Type: MutCreateElement, NodeID: 1, Key: "tag", Value: "div"})
	if sched.Len() != 1 {
		t.Fatal("expected 1 mutation in queue")
	}
	out := sched.Flush()
	if len(out) != 1 {
		t.Fatal("expected 1 mutation flushed")
	}
	if sched.Len() != 0 {
		t.Fatal("expected empty after flush")
	}
}

func TestComponentFrame(t *testing.T) {
	frame := PushComponent()
	if frame.Path != "/" {
		t.Fatalf("expected root path '/', got %q", frame.Path)
	}
	if CurrentComponent() != frame {
		t.Fatal("expected current component to be root")
	}
	PopComponent()
	if CurrentComponent() != nil {
		t.Fatal("expected nil after pop")
	}
}

func TestComponentTreeNesting(t *testing.T) {
	root := PushComponent()
	child1 := PushComponent()
	PopComponent()
	child2 := PushComponent()
	PopComponent()
	PopComponent()

	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}
	if child1.Parent != root {
		t.Fatal("child1 parent should be root")
	}
	if child2.Parent != root {
		t.Fatal("child2 parent should be root")
	}
}

func TestFlatTree(t *testing.T) {
	inner := Component("Inner", func() Node {
		return &ElementNode{Tag: "span"}
	})
	scope := &ScopeNode{
		Render: func() Node {
			return &ElementNode{Tag: "div", Children: []Node{inner}}
		},
	}
	flat := FlatTree(scope)
	// ScopeNodes are no longer expanded by FlatTree — renderNode handles them.
	// FlatTree still expands ComponentNodes into their inner elements.
	s, ok := flat.(*ScopeNode)
	if !ok {
		t.Fatalf("expected ScopeNode, got %T", flat)
	}
	if s.Render == nil {
		t.Fatal("expected non-nil Render on ScopeNode")
	}
}

func TestCollectIDs(t *testing.T) {
	el := &ElementNode{ID: 5, Tag: "div", Children: []Node{
		&ElementNode{ID: 3, Tag: "span"},
		&TextNode{ID: 7, Value: "hello"},
	}}
	ids := CollectIDs(el)
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %v", ids)
	}
	if ids[0] != 5 || ids[1] != 3 || ids[2] != 7 {
		t.Fatalf("expected [5 3 7], got %v", ids)
	}
}

func TestCollectIDsFiltersZero(t *testing.T) {
	el := &ElementNode{ID: 0, Tag: "div", Children: []Node{
		&ElementNode{ID: 2, Tag: "span"},
	}}
	ids := CollectIDs(el)
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("expected [2], got %v", ids)
	}
}

func TestFlatTreeWithFrames(t *testing.T) {
	comp := Component("TestComp", func() Node {
		return &ElementNode{Tag: "span"}
	})
	_, frames := FlatTreeWithFrames(comp)
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].Path != "/" {
		t.Fatalf("expected path /, got %s", frames[0].Path)
	}

	// Nested components
	outer := Component("Outer", func() Node {
		return &ElementNode{
			Tag: "div",
			Children: []Node{
				Component("Inner", func() Node {
					return &ElementNode{Tag: "p"}
				}),
			},
		}
	})
	_, frames2 := FlatTreeWithFrames(outer)
	if len(frames2) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames2))
	}
}

func TestFlatTreeFragmentInElement(t *testing.T) {
	el := &ElementNode{
		Tag: "div",
		Children: []Node{
			&FragmentNode{Children: []Node{
				&ElementNode{Tag: "span"},
				&ElementNode{Tag: "span"},
			}},
		},
	}
	flat := FlatTree(el)
	e, ok := flat.(*ElementNode)
	if !ok {
		t.Fatalf("expected ElementNode, got %T", flat)
	}
	if len(e.Children) != 2 {
		t.Fatalf("expected 2 children after Fragment flattening, got %d", len(e.Children))
	}
	for i, c := range e.Children {
		if _, ok := c.(*ElementNode); !ok {
			t.Fatalf("child %d: expected ElementNode, got %T", i, c)
		}
	}
}

func TestNodeTypes(t *testing.T) {
	el := &ElementNode{Tag: "div", Props: map[string]any{"class": "foo"}}
	if !strings.Contains(el.String(), "Element(div)") {
		t.Fatal("bad ElementNode String()")
	}
	txt := &TextNode{Value: "hello"}
	if !strings.Contains(txt.String(), "Text(hello)") {
		t.Fatal("bad TextNode String()")
	}
	frag := &FragmentNode{Children: []Node{el, txt}}
	if !strings.Contains(frag.String(), "Fragment(2") {
		t.Fatal("bad FragmentNode String()")
	}
	comp := Component("MyComp", func() Node { return txt })
	if !strings.Contains(comp.String(), "Component(MyComp)") {
		t.Fatalf("bad ComponentNode String(): %s", comp.String())
	}
}
