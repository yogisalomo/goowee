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
		Attrs: []core.Attr{{Name: "class", Value: "greeting"}},
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

func TestDOMRenderTypedFields(t *testing.T) {
	count := core.NewSignal(0)
	n := &core.ElementNode{
		Tag: "span",
		Attrs: []core.Attr{{Name: "class", Value: "greeting"}},
		Props: []core.Prop{{Name: "value", Value: "hello"}},
		Binds: []core.Bind{{
			Target: core.BindToProp, Name: "textContent", Signal: count,
		}},
		Handlers: []core.Handler{{
			Event: "click", Fn: func(core.EventData) {},
		}},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) < 4 {
		t.Fatalf("expected at least 4 mutations, got %d", len(muts))
	}
	if muts[1].Type != core.MutSetAttribute && muts[1].Key != "class" {
		t.Fatalf("expected SetAttribute for class, got %v", muts[1])
	}
	foundProp := false
	foundBind := false
	for _, m := range muts {
		if m.Type == core.MutSetProperty && m.Key == "value" {
			foundProp = true
		}
		if m.Type == core.MutSetProperty && m.Key == "textContent" {
			foundBind = true
		}
	}
	if !foundProp {
		t.Fatal("expected SetProperty for value")
	}
	if !foundBind {
		t.Fatal("expected SetProperty for binding")
	}
}

func TestDOMReactiveUpdate(t *testing.T) {
	count := core.NewSignal(0)
	n := &core.ElementNode{
		Tag: "span",
		Binds: []core.Bind{{
			Target: core.BindToProp, Name: "textContent", Signal: count,
		}},
	}
	r := New()
	r.Render(n)

	count.Set(1)
	updates := r.Scheduler.Flush()
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
	updates := r.Scheduler.Flush()
	if len(updates) != 1 || updates[0].Value != "universe" {
		t.Fatalf("expected 'universe', got %v", updates)
	}
}

func TestDOMFragment(t *testing.T) {
	n := &core.FragmentNode{
		Children: []core.Node{
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "a"}}},
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "b"}}},
		},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) != 6 {
		t.Fatalf("expected 6 mutations (2 create + 2 attr + 2 append to root), got %d: %v", len(muts), muts)
	}
}

func TestDOMComponentNode(t *testing.T) {
	greeting := core.Component("Greeting", func() core.Node {
		return &core.ElementNode{
			Tag:   "p",
			Props: []core.Prop{{Name: "textContent", Value: "hello"}},
		}
	})
	r := New()
	muts, id := r.Render(greeting)
	if id != 1 {
		t.Fatalf("expected root id 1, got %d", id)
	}
	if len(muts) < 3 {
		t.Fatalf("expected at least 3 mutations (create + prop + append root), got %d", len(muts))
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "p" {
		t.Fatalf("expected CreateElement(p), got %v", muts[0])
	}
}

func TestDOMNestedComponent(t *testing.T) {
	inner := core.Component("Inner", func() core.Node {
		return &core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "inner"}}}
	})
	outer := core.Component("Outer", func() core.Node {
		return &core.ElementNode{
			Tag:      "div",
			Children: []core.Node{inner},
		}
	})
	r := New()
	muts, _ := r.Render(outer)
	if len(muts) < 4 {
		t.Fatalf("expected at least 4 mutations, got %d: %v", len(muts), muts)
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "div" {
		t.Fatalf("expected CreateElement(div), got %v", muts[0])
	}
}

func TestDOMScopeNodeFirstRender(t *testing.T) {
	signal := core.NewSignal("hello")
	scope := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:   "p",
				Attrs: []core.Attr{{Name: "class", Value: "greeting"}},
				Binds: []core.Bind{{
					Target: core.BindToProp, Name: "textContent", Signal: signal,
				}},
			}
		},
		Deps: []core.SignalAccessor{signal},
	}
	r := New()
	muts, id := r.Render(scope)
	if id != 1 {
		t.Fatalf("expected root id 1, got %d", id)
	}
	if len(muts) < 3 {
		t.Fatalf("expected at least 3 mutations, got %d", len(muts))
	}
	if muts[0].Type != core.MutCreateElement || muts[0].Value != "p" {
		t.Fatalf("expected CreateElement(p), got %v", muts[0])
	}
}

func TestDOMScopeReRender(t *testing.T) {
	show := core.NewSignal(true)

	renderCount := 0
	scope := &core.ScopeNode{
		Render: func() core.Node {
			renderCount++
			if show.Get() {
				return &core.ElementNode{
					Tag:   "div",
					Attrs: []core.Attr{{Name: "class", Value: "visible"}},
				}
			}
			return &core.ElementNode{
				Tag:   "div",
				Attrs: []core.Attr{{Name: "class", Value: "hidden"}},
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
	if len(initMuts) < 2 {
		t.Fatalf("expected mutations, got %d", len(initMuts))
	}
	if scope.Prev == nil {
		t.Fatal("expected Prev tree after first render")
	}

	show.Set(false)
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations after signal change")
	}

	foundSet := false
	for _, m := range diffMuts {
		if m.Type == core.MutSetAttribute && m.Key == "class" {
			foundSet = true
			if m.Value != "hidden" {
				t.Fatalf("expected class=hidden, got %s", m.Value)
			}
		}
	}
	if !foundSet {
		t.Fatalf("expected MutSetAttribute(class=hidden), got %v", diffMuts)
	}
	if renderCount != 2 {
		t.Fatalf("expected 2 renders, got %d", renderCount)
	}

	el, ok := scope.Prev.(*core.ElementNode)
	if !ok {
		t.Fatalf("expected ElementNode in Prev, got %T", scope.Prev)
	}
	if len(el.Attrs) == 0 || el.Attrs[0].Value != "hidden" {
		t.Fatalf("expected class=hidden in Prev, got %v", el.Attrs)
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

	show.Set(false)
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}

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
					return &core.ElementNode{Tag: "p", Props: []core.Prop{{Name: "textContent", Value: "visible"}}}
				})
			}
			return &core.ElementNode{Tag: "p", Props: []core.Prop{{Name: "textContent", Value: "hidden"}}}
		},
		Deps: []core.SignalAccessor{toggle},
	}

	r := New()
	r.Render(scope)

	if cleanupCalled != 0 {
		t.Fatalf("expected 0 cleanups before unmount, got %d", cleanupCalled)
	}

	toggle.Set(false)
	diffMuts := r.Scheduler.Flush()
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}
	if cleanupCalled != 1 {
		t.Fatalf("expected 1 cleanup after unmount, got %d", cleanupCalled)
	}

	toggle.Set(true)
	_ = r.Scheduler.Flush()
	if cleanupCalled != 1 {
		t.Fatalf("expected still 1 cleanup (no extra call on re-create), got %d", cleanupCalled)
	}
}

func TestDOMScopeInsideComponentInsideScope(t *testing.T) {
	name := core.NewSignal("")

	innerRenderCount := 0
	innerScope := &core.ScopeNode{
		Render: func() core.Node {
			innerRenderCount++
			return &core.ElementNode{
				Tag:   "p",
				Props: []core.Prop{{Name: "textContent", Value: "Hello " + name.Get()}},
			}
		},
		Deps: []core.SignalAccessor{name},
	}

	component := core.Component("Wrapper", func() core.Node {
		return &core.ElementNode{
			Tag:      "div",
			Children: []core.Node{innerScope},
		}
	})

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

	if outerRenderCount != 1 {
		t.Fatalf("expected 1 outer render, got %d", outerRenderCount)
	}
	if innerRenderCount != 1 {
		t.Fatalf("expected 1 inner render, got %d", innerRenderCount)
	}

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

	_ = initMuts
}

func TestDOMDiffRemovesStaleAttrsAndProps(t *testing.T) {
	show := core.NewSignal("a")
	scope := &core.ScopeNode{
		Render: func() core.Node {
			v := show.Get()
			return &core.ElementNode{
				Tag:   "div",
				Attrs: []core.Attr{{Name: "class", Value: v}},
				Props: []core.Prop{{Name: "value", Value: v}},
			}
		},
		Deps: []core.SignalAccessor{show},
	}

	r := New()
	r.Render(scope)

	show.Set("b")
	muts := r.Scheduler.Flush()
	if len(muts) == 0 {
		t.Fatal("expected mutations")
	}
	hasAttr := false
	hasProp := false
	for _, m := range muts {
		if m.Type == core.MutSetAttribute && m.Key == "class" {
			hasAttr = true
		}
		if m.Type == core.MutSetProperty && m.Key == "value" {
			hasProp = true
		}
	}
	if !hasAttr {
		t.Fatal("expected SetAttribute for class")
	}
	if !hasProp {
		t.Fatal("expected SetProperty for value")
	}
}

func TestDiffTypeChangeNoPanic(t *testing.T) {
	sig := core.NewSignal(true)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			if sig.Get() {
				return &core.ElementNode{Tag: "div"}
			}
			return &core.TextNode{Value: "text"}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	r.Render(scope)
	sig.Set(false)
	muts := r.Scheduler.Flush()
	if len(muts) == 0 {
		t.Fatal("expected mutations after type change")
	}
	hasRemove := false
	hasCreate := false
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			hasRemove = true
		}
		if m.Type == core.MutCreateElement && m.Value == "#text" {
			hasCreate = true
		}
	}
	if !hasRemove {
		t.Fatal("expected RemoveNode for old element")
	}
	if !hasCreate {
		t.Fatal("expected CreateElement for new text")
	}
}

func TestDOMFragmentInsideElement(t *testing.T) {
	frag := &core.FragmentNode{
		Children: []core.Node{
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "a"}}},
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "b"}}},
		},
	}
	n := &core.ElementNode{
		Tag:      "div",
		Children: []core.Node{frag},
	}
	r := New()
	muts, _ := r.Render(n)
	if len(muts) == 0 {
		t.Fatal("expected mutations")
	}

	spanCount := 0
	for _, m := range muts {
		if m.Type == core.MutCreateElement && m.Value == "span" {
			spanCount++
		}
	}
	if spanCount != 2 {
		t.Fatalf("expected 2 span elements created, got %d", spanCount)
	}
}

func TestDOMDeepNestedScopes(t *testing.T) {
	outerSig := core.NewSignal(0)
	innerSig := core.NewSignal("")

	deepest := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:   "p",
				Props: []core.Prop{{Name: "textContent", Value: innerSig.Get() + " " + fmt.Sprint(outerSig.Get())}},
			}
		},
		Deps: []core.SignalAccessor{innerSig, outerSig},
	}

	middle := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:      "div",
				Children: []core.Node{core.Component("Inner", func() core.Node { return deepest })},
			}
		},
		Deps: []core.SignalAccessor{outerSig},
	}

	outer := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:      "div",
				Children: []core.Node{middle},
			}
		},
		Deps: []core.SignalAccessor{outerSig},
	}

	r := New()
	initMuts, _ := r.Render(outer)
	if len(initMuts) == 0 {
		t.Fatal("expected mutations")
	}

	outerSig.Set(1)
	muts := r.Scheduler.Flush()
	if len(muts) == 0 {
		t.Fatal("expected mutations after outerSig change")
	}

	innerSig.Set("hello")
	muts2 := r.Scheduler.Flush()
	if len(muts2) == 0 {
		t.Fatal("expected mutations after innerSig change")
	}
}

func TestDOMEventNotEmitted(t *testing.T) {
	n := &core.ElementNode{
		Tag: "button",
		Handlers: []core.Handler{{
			Event: "click", Fn: func(core.EventData) {},
		}},
	}
	r := New()
	muts, _ := r.Render(n)
	for _, m := range muts {
		if m.Key == "click" && m.Type != core.MutInsertBefore {
			t.Fatal("event handlers should not be emitted as mutations")
		}
	}
}

func TestDOMDiffRebindsSignals(t *testing.T) {
	sig := core.NewSignal("a")
	scope := &core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag: "div",
				Binds: []core.Bind{{
					Target: core.BindToProp, Name: "textContent", Signal: sig,
				}},
			}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	r.Render(scope)

	sig.Set("b")
	muts := r.Scheduler.Flush()
	if len(muts) == 0 {
		t.Fatal("expected mutations after signal change")
	}
	found := false
	for _, m := range muts {
		if m.Type == core.MutSetProperty && m.Key == "textContent" && m.Value == "b" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SetProperty(textContent=b), got %v", muts)
	}
}

func TestDOMDiffReplacesHandlers(t *testing.T) {
	var called bool
	sig := core.NewSignal(0)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			v := sig.Get()
			return &core.ElementNode{
				Tag: "button",
				Handlers: []core.Handler{{
					Event: "click",
					Fn: func(core.EventData) {
						called = v == 1
					},
				}},
			}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	r.Render(scope)

	sig.Set(1)
	_ = r.Scheduler.Flush()

	r.Registry.Dispatch(1, "click", `{}`)
	if !called {
		t.Fatal("expected handler to reflect new closure")
	}
}

func TestDOMDispatchReturnsOptionsAndRecovers(t *testing.T) {
	r := New()
	n := &core.ElementNode{
		Tag: "button",
		Handlers: []core.Handler{{
			Event: "click",
			Fn:    func(core.EventData) {},
			Options: core.HandlerOptions{
				PreventDefault:  true,
				StopPropagation: false,
			},
		}},
	}
	r.Render(n)

	opts, handled := r.Registry.Dispatch(1, "click", `{}`)
	if !handled {
		t.Fatal("expected handler to run")
	}
	if !opts.PreventDefault {
		t.Fatal("expected PreventDefault")
	}

	_, handled2 := r.Registry.Dispatch(1, "input", `{}`)
	if handled2 {
		t.Fatal("expected no handler for input")
	}

	nilNode := &core.ElementNode{
		Tag: "button",
		Handlers: []core.Handler{{
			Event: "click", Fn: func(core.EventData) { panic("test") },
		}},
	}
	r2 := New()
	r2.Render(nilNode)
	_, handled3 := r2.Registry.Dispatch(1, "click", `{}`)
	if !handled3 {
		t.Fatal("expected handler to run despite panic")
	}
}

func TestUnbindCancelsSubscription(t *testing.T) {
	sig := core.NewSignal(0)
	n := &core.ElementNode{
		Tag: "div",
		Binds: []core.Bind{{
			Target: core.BindToProp, Name: "textContent", Signal: sig,
		}},
	}
	r := New()
	r.Render(n)

	r.Bindings.Unbind(1)

	sig.Set(42)
	muts := r.Scheduler.Flush()
	for _, m := range muts {
		if m.Type == core.MutSetProperty {
			t.Fatal("expected no mutations after unbind")
		}
	}
}

func TestDOMRootMounting(t *testing.T) {
	n := &core.ElementNode{Tag: "div"}
	r := New()
	muts, _ := r.Render(n)
	hasAppend := false
	for _, m := range muts {
		if m.Type == core.MutAppendChild && m.NodeID == 0 && m.ChildID == 1 {
			hasAppend = true
		}
	}
	if !hasAppend {
		t.Fatalf("expected append to root container (node 0), got %v", muts)
	}
}

func TestDOMChildrenInsertBeforeRefID(t *testing.T) {
	sig := core.NewSignal(0)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			v := sig.Get()
			if v == 0 {
				return &core.ElementNode{
					Tag: "div",
					Children: []core.Node{
						&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "a"}}},
						&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "b"}}},
						&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "c"}}},
					},
				}
			}
			return &core.ElementNode{
				Tag: "div",
				Children: []core.Node{
					&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "x"}}},
					&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "a"}}},
					&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "b"}}},
					&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "c"}}},
				},
			}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	r.Render(scope)

	sig.Set(1)
	muts := r.Scheduler.Flush()
	foundInsert := false
	for _, m := range muts {
		if m.Type == core.MutInsertBefore {
			foundInsert = true
			break
		}
	}
	if !foundInsert {
		t.Fatalf("expected InsertBefore mutation, got: %v", muts)
	}
}
