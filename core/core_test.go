package core

import (
	"strings"
	"sync"
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
	// Head void elements (rendered by h.Metadata) must self-close.
	for _, tag := range []string{"meta", "link", "base"} {
		if !VoidElements[tag] {
			t.Fatalf("expected %s to be void", tag)
		}
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

func TestSchedulerCoalescesWrites(t *testing.T) {
	s := NewScheduler()
	s.Enqueue(
		Mutation{Type: MutSetProperty, NodeID: 1, Key: "x", Value: "a"},
		Mutation{Type: MutAppendChild, NodeID: 0, ChildID: 1},
		Mutation{Type: MutSetProperty, NodeID: 1, Key: "x", Value: "b"}, // supersedes x=a
		Mutation{Type: MutSetProperty, NodeID: 1, Key: "y", Value: "1"},
		Mutation{Type: MutSetAttribute, NodeID: 1, Key: "x", Value: "attr"}, // different type, kept
	)
	out := s.Flush()
	if len(out) != 4 {
		t.Fatalf("want 4 mutations after coalesce, got %d: %+v", len(out), out)
	}
	// Superseded x=a is dropped; x=b survives at its (later) position, so
	// non-property mutations keep their relative order.
	propXCount := 0
	var propX Mutation
	for _, m := range out {
		if m.Type == MutSetProperty && m.NodeID == 1 && m.Key == "x" {
			propXCount++
			propX = m
		}
	}
	if propXCount != 1 || propX.Value != "b" {
		t.Fatalf("SetProperty x should coalesce to a single last value b, got count=%d %v", propXCount, propX.Value)
	}
	if out[0].Type != MutAppendChild {
		t.Fatalf("AppendChild order should be preserved (x=a dropped), got %+v", out)
	}
}

func TestSchedulerOnWorkSuppressedDuringFlush(t *testing.T) {
	s := NewScheduler()
	calls := 0
	s.OnWork = func() { calls++ }

	s.Enqueue(Mutation{Type: MutAppendChild, NodeID: 0, ChildID: 1})
	if calls != 1 {
		t.Fatalf("enqueue should signal work once, got %d", calls)
	}
	s.MarkDirty("scope", 0, func() {
		// A re-render enqueues during flush; that must not schedule a new frame.
		s.Enqueue(Mutation{Type: MutSetProperty, NodeID: 1, Key: "x", Value: "v"})
	})
	if calls != 2 {
		t.Fatalf("markdirty should signal work, got %d", calls)
	}
	s.Flush()
	if calls != 2 {
		t.Fatalf("enqueues during flush must not signal work, got %d", calls)
	}
}

func TestSchedulerMarkDirtyDedups(t *testing.T) {
	s := NewScheduler()
	renders := 0
	render := func() { renders++ }
	s.MarkDirty("scope", 0, render)
	s.MarkDirty("scope", 0, render)
	s.MarkDirty("scope", 0, render)
	s.Flush()
	if renders != 1 {
		t.Fatalf("repeated MarkDirty on the same key should re-render once, got %d", renders)
	}
}

func TestSchedulerPostGoroutineSafe(t *testing.T) {
	s := NewScheduler()
	got := 0
	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	// Many goroutines Post concurrently; the inbox is mutex-protected.
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s.Post(func() { got++ }) // fn bodies run single-threaded, inside Flush
		}()
	}
	wg.Wait()
	s.Flush() // single drain on this goroutine
	if got != n {
		t.Fatalf("expected %d posted callbacks to run, got %d", n, got)
	}
}

func TestSchedulerPostRunsOnFlushSetsSignals(t *testing.T) {
	s := NewScheduler()
	sig := NewSignal(0)
	fired := 0
	sig.Subscribe(func() { fired++ })

	s.Post(func() { sig.Set(1) })
	if sig.Get() != 0 {
		t.Fatal("Post must defer the update until flush, not run inline")
	}
	s.Flush()
	if sig.Get() != 1 || fired != 1 {
		t.Fatalf("after flush want value=1 fired=1, got value=%d fired=%d", sig.Get(), fired)
	}
}

func TestRefInvokeEnqueuesMutation(t *testing.T) {
	s := NewScheduler()
	SetActiveScheduler(s)
	defer SetActiveScheduler(nil)

	(&Ref{ID: 7}).Focus()
	out := s.Flush()
	if len(out) != 1 || out[0].Type != MutInvoke || out[0].NodeID != 7 || out[0].Key != "focus" {
		t.Fatalf("Focus should enqueue MutInvoke{7,focus}, got %+v", out)
	}
}

func TestRefNoopWhenUnusable(t *testing.T) {
	s := NewScheduler()
	SetActiveScheduler(s)
	defer SetActiveScheduler(nil)

	(&Ref{}).Focus() // zero id → no-op (element not rendered yet)
	if s.Len() != 0 {
		t.Fatal("Focus on an unrendered ref (ID 0) must not enqueue")
	}
	SetActiveScheduler(nil)
	(&Ref{ID: 4}).Focus() // no active scheduler → no panic
}
