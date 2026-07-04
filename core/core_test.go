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

func TestFlatTreeLeavesComponentsIntact(t *testing.T) {
	// FlatTree must not execute components: a component is a stable boundary
	// the renderer mounts once, not something flattened away at build time.
	ran := 0
	comp := Component("TestComp", func() Node {
		ran++
		return &ElementNode{Tag: "span"}
	})
	flat := FlatTree(comp)
	if ran != 0 {
		t.Fatalf("FlatTree should not run component setup, ran=%d", ran)
	}
	if flat != Node(comp) {
		t.Fatalf("expected the component to pass through unchanged, got %T", flat)
	}

	// A component nested in an element stays a ComponentNode child.
	el := &ElementNode{Tag: "div", Children: []Node{comp}}
	flatEl := FlatTree(el).(*ElementNode)
	if len(flatEl.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(flatEl.Children))
	}
	if _, ok := flatEl.Children[0].(*ComponentNode); !ok {
		t.Fatalf("expected child to remain a ComponentNode, got %T", flatEl.Children[0])
	}
	if ran != 0 {
		t.Fatalf("component still should not have run, ran=%d", ran)
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
	el := &ElementNode{Tag: "div"}
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

func TestItemApplyNilSafety(t *testing.T) {
	parent := &ElementNode{Tag: "div"}
	(*ElementNode)(nil).Apply(parent)
	if len(parent.Children) != 0 {
		t.Fatal("nil ElementNode Apply should be no-op")
	}
	(*TextNode)(nil).Apply(parent)
	if len(parent.Children) != 0 {
		t.Fatal("nil TextNode Apply should be no-op")
	}
	(*FragmentNode)(nil).Apply(parent)
	if len(parent.Children) != 0 {
		t.Fatal("nil FragmentNode Apply should be no-op")
	}
	(*ComponentNode)(nil).Apply(parent)
	if len(parent.Children) != 0 {
		t.Fatal("nil ComponentNode Apply should be no-op")
	}
	(*ScopeNode)(nil).Apply(parent)
	if len(parent.Children) != 0 {
		t.Fatal("nil ScopeNode Apply should be no-op")
	}
}

func TestEventDataAccessors(t *testing.T) {
	e := EventData{Data: map[string]any{
		"value":     "hello",
		"key":       "Enter",
		"checked":   true,
		"scrollTop": 100.0,
		"clientX":   200.0,
	}}
	if e.Value() != "hello" {
		t.Fatalf("expected hello, got %s", e.Value())
	}
	if e.Key() != "Enter" {
		t.Fatalf("expected Enter, got %s", e.Key())
	}
	if !e.Checked() {
		t.Fatal("expected checked true")
	}
	if e.ScrollTop() != 100.0 {
		t.Fatalf("expected 100, got %f", e.ScrollTop())
	}
	if e.ClientX() != 200.0 {
		t.Fatalf("expected 200, got %f", e.ClientX())
	}

	vals := e.FormValues()
	if len(vals) != 0 {
		t.Fatal("expected empty form values")
	}

	e2 := EventData{Data: map[string]any{
		"values": map[string]any{"name": "alice"},
	}}
	vals2 := e2.FormValues()
	if vals2["name"] != "alice" {
		t.Fatalf("expected alice, got %v", vals2)
	}
}

func TestVoidElements(t *testing.T) {
	if !VoidElements["br"] {
		t.Fatal("expected br to be void")
	}
	if !VoidElements["input"] {
		t.Fatal("expected input to be void")
	}
	if VoidElements["div"] {
		t.Fatal("expected div not to be void")
	}
}

func TestElementNodeApply(t *testing.T) {
	parent := &ElementNode{Tag: "div"}
	child := &ElementNode{Tag: "span"}
	child.Apply(parent)
	if len(parent.Children) != 1 {
		t.Fatal("expected 1 child")
	}
	if parent.Children[0] != child {
		t.Fatal("expected child to be appended")
	}
}

func TestTextNodeApply(t *testing.T) {
	parent := &ElementNode{Tag: "div"}
	child := &TextNode{Value: "hello"}
	child.Apply(parent)
	if len(parent.Children) != 1 {
		t.Fatal("expected 1 child")
	}
}

func TestFragmentNodeApply(t *testing.T) {
	parent := &ElementNode{Tag: "div"}
	child := &FragmentNode{Children: []Node{&TextNode{Value: "a"}}}
	child.Apply(parent)
	if len(parent.Children) != 1 {
		t.Fatal("expected 1 child")
	}
}

func TestMutationRefID(t *testing.T) {
	m := Mutation{Type: MutInsertBefore, NodeID: 1, ChildID: 2, RefID: 0}
	if m.RefID != 0 {
		t.Fatal("expected RefID 0")
	}
	m2 := Mutation{Type: MutRemoveAttribute, NodeID: 1, Key: "class"}
	if m2.Type != MutRemoveAttribute {
		t.Fatal("expected MutRemoveAttribute")
	}
}

func TestComputedRecomputesAndDisposes(t *testing.T) {
	a := NewSignal(1)
	b := NewSignal(2)

	PushComponent()
	comp := Computed([]SignalAccessor{a, b}, func() int { return a.Get() + b.Get() })

	if comp.Get() != 3 {
		t.Fatalf("expected 3, got %d", comp.Get())
	}

	a.Set(10)
	if comp.Get() != 12 {
		t.Fatalf("expected 12 after a change, got %d", comp.Get())
	}

	b.Set(20)
	if comp.Get() != 30 {
		t.Fatalf("expected 30 after b change, got %d", comp.Get())
	}

	frame := CurrentComponent()
	PopComponent()

	for _, d := range frame.Disposers {
		d()
	}

	a.Set(100)
	b.Set(200)
	if got := comp.Get(); got != 30 {
		t.Fatalf("after disposal, expected last cached value 30, got %d", got)
	}
}
