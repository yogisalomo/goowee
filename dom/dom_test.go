package dom

import (
	"fmt"
	"goowee/core"
	"goowee/hooks"
	"testing"
)

func TestDOMRenderElement(t *testing.T) {
	n := &core.ElementNode{
		Tag: "div",
		Props: map[string]any{"class": "greeting"},
		Children: []core.Node{
			&core.TextNode{Value: "hello"},
		},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) == 0 {
		t.Fatal("expected mutations")
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "div" {
		t.Fatalf("expected CreateElement(div), got %v", muts[0])
	}
}

func TestDOMReactiveUpdate(t *testing.T) {
	count := core.NewSignal(0)
	n := &core.ElementNode{
		Tag: "span",
		Props: map[string]any{
			"textContent": count,
		},
	}
	r := New()
	r.Render(n)

	count.Set(1)
	updates := r.Bindings.GetMutationsFor(1)
	if len(updates) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(updates))
	}
	if updates[0].Value != 1 {
		t.Fatalf("expected 1, got %v", updates[0].Value)
	}
}

func TestDOMTextNodeSignal(t *testing.T) {
	name := core.NewSignal("world")
	n := &core.ElementNode{
		Tag: "div",
		Children: []core.Node{
			&core.TextNode{Value: name},
		},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) == 0 {
		t.Fatal("expected mutations")
	}
	name.Set("universe")
	updates := r.Bindings.GetMutationsFor(2)
	if len(updates) != 1 || updates[0].Value != "universe" {
		t.Fatalf("expected 'universe', got %v", updates)
	}
}

func TestDOMFragment(t *testing.T) {
	n := &core.FragmentNode{
		Children: []core.Node{
			&core.ElementNode{Tag: "span", Props: map[string]any{"class": "a"}},
			&core.ElementNode{Tag: "span", Props: map[string]any{"class": "b"}},
		},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) != 4 {
		t.Fatalf("expected 4 mutations, got %d: %v", len(muts), muts)
	}
}

func TestDOMPropertyVsAttribute(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		wantType core.MutationType
	}{
		{"class", "foo", core.MutSetProperty},
		{"id", "bar", core.MutSetAttribute},
		{"href", "/x", core.MutSetAttribute},
		{"textContent", "hi", core.MutSetProperty},
		{"checked", "true", core.MutSetProperty},
		{"value", "42", core.MutSetProperty},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			n := &core.ElementNode{
				Tag:   "div",
				Props: map[string]any{tt.key: tt.value},
			}
	r := New()
		muts, _ := r.Render(n)
		if len(muts) < 2 {
			t.Fatalf("expected at least 2 mutations, got %d", len(muts))
		}
		if muts[1].Type != tt.wantType {
			t.Fatalf("key=%q: expected %v, got %v", tt.key, tt.wantType, muts[1].Type)
		}
	})
	}
}

func TestDOMComponentNode(t *testing.T) {
	greeting := core.Component("Greeting", func() core.Node {
		return &core.ElementNode{
			Tag:   "p",
			Props: map[string]any{"textContent": "hello"},
		}
	})
	r := New()
	muts, id := r.Render(greeting)
	if id != 1 {
		t.Fatalf("expected root id 1, got %d", id)
	}
	if len(muts) < 2 {
		t.Fatalf("expected at least 2 mutations, got %d", len(muts))
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "p" {
		t.Fatalf("expected CreateElement(p), got %v", muts[0])
	}
	if muts[1].Type != core.MutSetProperty || muts[1].Key != "textContent" {
		t.Fatalf("expected SetProperty(textContent), got %v", muts[1])
	}
}

func TestDOMNestedComponent(t *testing.T) {
	inner := core.Component("Inner", func() core.Node {
		return &core.ElementNode{Tag: "span", Props: map[string]any{"class": "inner"}}
	})
	outer := core.Component("Outer", func() core.Node {
		return &core.ElementNode{
			Tag:      "div",
			Children: []core.Node{inner},
		}
	})
	r := New()
	muts, _ := r.Render(outer)
	// div + span + className prop + appendChild
	if len(muts) != 4 {
		t.Fatalf("expected 4 mutations, got %d: %v", len(muts), muts)
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "div" {
		t.Fatalf("expected CreateElement(div), got %v", muts[0])
	}
	// span is the inner component's element
	if muts[1].Type != core.MutCreateElement || muts[1].Value != "span" {
		t.Fatalf("expected CreateElement(span), got %v", muts[1])
	}
}

func TestDOMScopeNodeFirstRender(t *testing.T) {
	signal := core.NewSignal("hello")
	scope := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": signal, "class": "greeting"},
			}
		},
		Deps: []core.SignalAccessor{signal},
	}
	r := New()
	muts, id := r.Render(scope)
	if id != 1 {
		t.Fatalf("expected root id 1, got %d", id)
	}
	// CreateElement(p) + SetProperty(className) + SetProperty(textContent via bind)
	if len(muts) < 2 {
		t.Fatalf("expected at least 2 mutations, got %d", len(muts))
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "p" {
		t.Fatalf("expected CreateElement(p), got %v", muts[0])
	}
}

func TestDOMScopeReRender(t *testing.T) {
	show := core.NewSignal(true)
	
	// Track renders to verify the render function is re-called
	renderCount := 0
	scope := &core.ScopeNode{
		Render: func() core.Node {
			renderCount++
			if show.Get() {
				return &core.ElementNode{
					Tag:   "div",
					Props: map[string]any{"class": "visible"},
				}
			}
			return &core.ElementNode{
				Tag:   "div",
				Props: map[string]any{"class": "hidden"},
			}
		},
		Deps: []core.SignalAccessor{show},
	}

	r := New()
	initMuts, id := r.Render(scope)
	if id != 1 {
		t.Fatalf("expected root id 1, got %d", id)
	}
	if renderCount != 1 {
		t.Fatalf("expected 1 render, got %d", renderCount)
	}
	
	// Initially visible
	if len(initMuts) < 2 {
		t.Fatalf("expected mutations, got %d", len(initMuts))
	}
	if scope.Prev == nil {
		t.Fatal("expected Prev tree after first render")
	}

	// Change signal to trigger re-render
	show.Set(false)

	// Scheduler should have diff mutations
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations after signal change")
	}

	// Should have SetProperty for className (visible -> hidden)
	foundSet := false
	for _, m := range diffMuts {
		if m.Type == core.MutSetProperty && m.Key == "className" {
			foundSet = true
			if m.Value != "hidden" {
				t.Fatalf("expected className=hidden, got %s", m.Value)
			}
		}
	}
	if !foundSet {
		t.Fatalf("expected SetProperty(className=hidden), got %v", diffMuts)
	}

	// Verify render was called again
	if renderCount != 2 {
		t.Fatalf("expected 2 renders, got %d", renderCount)
	}

	// Prev should be updated to new tree
	el, ok := scope.Prev.(*core.ElementNode)
	if !ok {
		t.Fatalf("expected ElementNode in Prev, got %T", scope.Prev)
	}
	if el.Props["class"] != "hidden" {
		t.Fatalf("expected class=hidden in Prev, got %v", el.Props["class"])
	}
}

func TestDOMScopeStructuralChange(t *testing.T) {
	show := core.NewSignal(true)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			if show.Get() {
				return &core.ElementNode{Tag: "div"}
			}
			return &core.ElementNode{Tag: "span"}
		},
		Deps: []core.SignalAccessor{show},
	}

	r := New()
	initMuts, id := r.Render(scope)
	if id != 1 || initMuts[0].Value != "div" {
		t.Fatalf("expected div, got %v", initMuts[0])
	}

	// Change to span — structural change (different tag)
	show.Set(false)
	diffMuts := r.Scheduler.Flush()

	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}

	// Should remove old (div ID 1) and create new (span)
	hasRemove := false
	hasCreate := false
	for _, m := range diffMuts {
		if m.Type == core.MutCreateElement && m.Value == "span" {
			hasCreate = true
		}
		if m.Type == core.MutRemoveNode && m.NodeID == 1 {
			hasRemove = true
		}
	}
	if !hasRemove {
		t.Fatal("expected RemoveNode for old div")
	}
	if !hasCreate {
		t.Fatal("expected CreateElement for new span")
	}
}

func TestDOMScopeUnmountCleanup(t *testing.T) {
	cleanupCalled := 0
	toggle := core.NewSignal(true)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			if toggle.Get() {
				return core.Component("WithEffect", func() core.Node {
					hooks.UseEffect(nil, func() func() {
						return func() {
							cleanupCalled++
						}
					})
					return &core.ElementNode{Tag: "p", Props: map[string]any{"textContent": "visible"}}
				})
			}
			return &core.ElementNode{Tag: "p", Props: map[string]any{"textContent": "hidden"}}
		},
		Deps: []core.SignalAccessor{toggle},
	}

	r := New()
	r.Render(scope)

	if cleanupCalled != 0 {
		t.Fatalf("expected 0 cleanups before unmount, got %d", cleanupCalled)
	}

	// Toggle to remove the component
	toggle.Set(false)
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}
	if cleanupCalled != 1 {
		t.Fatalf("expected 1 cleanup after unmount, got %d", cleanupCalled)
	}

	// Toggle back — component re-creates, no stale cleanup call
	toggle.Set(true)
	_ = r.Scheduler.Flush()
	if cleanupCalled != 1 {
		t.Fatalf("expected still 1 cleanup (no extra call on re-create), got %d", cleanupCalled)
	}
}

// TestDOMScopeInsideComponentInsideScope: mirrors the form example pattern where
// a ScopeNode (preview) lives inside a ComponentNode (FormPage) inside another
// ScopeNode (Route). Verifies signal subscribers are wired up for the nested scope.
func TestDOMScopeInsideComponentInsideScope(t *testing.T) {
	name := core.NewSignal("")

	// Inner scope — like the form's preview UseScope
	innerRenderCount := 0
	innerScope := &core.ScopeNode{
		Render: func() core.Node {
			innerRenderCount++
			return &core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": "Hello " + name.Get()},
			}
		},
		Deps: []core.SignalAccessor{name},
	}

	// Component that contains the inner scope — like FormPage
	component := core.Component("Wrapper", func() core.Node {
		return &core.ElementNode{
			Tag:      "div",
			Children: []core.Node{innerScope},
		}
	})

	// Outer scope — like the Route scope
	show := core.NewSignal(true)
	outerRenderCount := 0
	outerScope := &core.ScopeNode{
		Render: func() core.Node {
			outerRenderCount++
			if show.Get() {
				return &core.ElementNode{
					Tag:      "div",
					Children: []core.Node{component},
				}
			}
			return &core.ElementNode{Tag: "span"}
		},
		Deps: []core.SignalAccessor{show},
	}

	r := New()
	initMuts, rootID := r.Render(outerScope)
	if rootID == 0 {
		t.Fatal("expected non-zero root ID")
	}
	if len(initMuts) == 0 {
		t.Fatal("expected initial mutations")
	}

	// Both should have rendered once
	if outerRenderCount != 1 {
		t.Fatalf("expected 1 outer render, got %d", outerRenderCount)
	}
	if innerRenderCount != 1 {
		t.Fatalf("expected 1 inner render, got %d", innerRenderCount)
	}

	// Change the inner scope's dep — this is the bug: subscribers weren't set up
	name.Set("World")
	innerMuts := r.Scheduler.Flush()
	if len(innerMuts) == 0 {
		t.Fatal("expected mutations when inner scope dep changes")
	}
	foundTextContent := false
	for _, m := range innerMuts {
		if m.Type == core.MutSetProperty && m.Key == "textContent" {
			foundTextContent = true
			if m.Value != "Hello World" {
				t.Fatalf("expected 'Hello World', got %v", m.Value)
			}
		}
	}
	if !foundTextContent {
		t.Fatalf("expected SetProperty(textContent), got %v", innerMuts)
	}
	if innerRenderCount != 2 {
		t.Fatalf("expected 2 inner renders, got %d", innerRenderCount)
	}

	// Change the outer scope to hide the component — should clean up inner content
	show.Set(false)
	outerMuts := r.Scheduler.Flush()
	if len(outerMuts) == 0 {
		t.Fatal("expected mutations when outer scope changes")
	}
	hasRemove := false
	for _, m := range outerMuts {
		if m.Type == core.MutRemoveNode {
			hasRemove = true
			break
		}
	}
	if !hasRemove {
		t.Fatalf("expected RemoveNode for old content, got %v", outerMuts)
	}
}

// TestDOMDiffReplaceChild: verifies that when a child is replaced with a
// different type (e.g. Li -> div spacer), an InsertBefore mutation is emitted
// so the new element actually appears in the DOM at the correct position.
func TestDOMDiffReplaceChild(t *testing.T) {
	visible := core.NewSignal(0)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			start := visible.Get()
			var children []core.Node
			if start > 0 {
				children = append(children, &core.ElementNode{
					Tag: "div", Props: map[string]any{"style": "height:200px;flex-shrink:0;"},
				})
			}
			end := start + 3
			for i := start; i < end; i++ {
				children = append(children, &core.ElementNode{
					Tag:   "span",
					Props: map[string]any{"textContent": fmt.Sprintf("Item %d", i)},
				})
			}
			return &core.ElementNode{
				Tag:      "div",
				Props:    map[string]any{"style": "overflow-y:auto;height:100px;"},
				Children: children,
			}
		},
		Deps: []core.SignalAccessor{visible},
	}

	r := New()
	initMuts, _ := r.Render(scope)
	if len(initMuts) == 0 {
		t.Fatal("expected initial mutations")
	}

	// First render: 3 spans, no spacer
	hasInsert := false
	for _, m := range initMuts {
		if m.Type == core.MutInsertBefore {
			hasInsert = true
		}
	}
	if hasInsert {
		t.Fatal("expected no InsertBefore on first render")
	}

	// Trigger re-render: now start=1, so a spacer div is prepended
	visible.Set(1)
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}

	// Should have at least one InsertBefore for the new spacer at position 0
	foundInsert := false
	for _, m := range diffMuts {
		if m.Type == core.MutInsertBefore {
			foundInsert = true
			if m.Key != "0" {
				t.Fatalf("expected InsertBefore at index 0, got index %s", m.Key)
			}
			break
		}
	}
	if !foundInsert {
		t.Fatalf("expected InsertBefore mutation, got: %v", diffMuts)
	}
}

// TestDOMDiffEventProp: verifies diffNode handles non-comparable func props
// (e.g. event handlers like onscroll) without panicking. Each scope re-render
// creates a new function closure — two func(core.EventData) values can't be
// compared with != in Go.
func TestDOMDiffEventProp(t *testing.T) {
	scrollTop := core.NewSignal(0.0)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag: "div",
				Props: map[string]any{
					"onscroll": func(ed core.EventData) {
						if st, ok := ed.Data["scrollTop"].(float64); ok {
							scrollTop.Set(st)
						}
					},
				},
			}
		},
		Deps: []core.SignalAccessor{scrollTop},
	}

	r := New()
	initMuts, _ := r.Render(scope)
	if len(initMuts) == 0 {
		t.Fatal("expected initial mutations")
	}

	// Trigger re-render — creates a new onscroll closure
	scrollTop.Set(100.0)
	diffMuts := r.Scheduler.Flush()
	// Should NOT panic. No mutations expected for the func prop (not string/bool).
	// The only mutation might be nothing — verify no crash.
	_ = diffMuts
}

func TestDOMEventNotEmitted(t *testing.T) {
	n := &core.ElementNode{
		Tag: "button",
		Props: map[string]any{
			"onclick": func(ed core.EventData) {},
		},
	}
	r := New()
	muts, _ := r.Render(n)
	for _, m := range muts {
		if m.Key == "onclick" {
			t.Fatal("event handlers should not be emitted as mutations")
		}
	}
}
