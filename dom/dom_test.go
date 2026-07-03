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

func makeKeyedSpan(key string, text string) *core.ElementNode {
	return &core.ElementNode{
		Tag: "span",
		Key: key,
		Attrs: []core.Attr{{Name: "class", Value: text}},
	}
}

func TestKeyedReorderReusesIDs(t *testing.T) {
	old := []core.Node{
		makeKeyedSpan("a", "a"),
		makeKeyedSpan("b", "b"),
		makeKeyedSpan("c", "c"),
		makeKeyedSpan("d", "d"),
		makeKeyedSpan("e", "e"),
	}
	rev := []core.Node{
		makeKeyedSpan("e", "e"),
		makeKeyedSpan("d", "d"),
		makeKeyedSpan("c", "c"),
		makeKeyedSpan("b", "b"),
		makeKeyedSpan("a", "a"),
	}

	r := New()
	r.Render(&core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, rev, &muts)

	removeCount := 0
	createCount := 0
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			removeCount++
		}
		if m.Type == core.MutCreateElement {
			createCount++
		}
	}
	if removeCount != 0 || createCount != 0 {
		t.Fatalf("expected no removes (%d) or creates (%d) on reorder", removeCount, createCount)
	}
}

func TestKeyedRemoveFirst(t *testing.T) {
	old := []core.Node{
		makeKeyedSpan("a", "a"),
		makeKeyedSpan("b", "b"),
		makeKeyedSpan("c", "c"),
	}
	new := []core.Node{
		makeKeyedSpan("b", "b"),
		makeKeyedSpan("c", "c"),
	}

	r := New()
	r.Render(&core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, &muts)

	removeCount := 0
	createCount := 0
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			removeCount++
		}
		if m.Type == core.MutCreateElement {
			createCount++
		}
	}
	if removeCount != 1 {
		t.Fatalf("expected exactly 1 remove, got %d", removeCount)
	}
	if createCount != 0 {
		t.Fatalf("expected 0 creates, got %d", createCount)
	}
}

func TestKeyedInsertMiddle(t *testing.T) {
	old := []core.Node{
		makeKeyedSpan("a", "a"),
		makeKeyedSpan("b", "b"),
	}
	new := []core.Node{
		makeKeyedSpan("a", "a"),
		makeKeyedSpan("c", "c"),
		makeKeyedSpan("b", "b"),
	}

	r := New()
	r.Render(&core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, &muts)

	createCount := 0
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			createCount++
		}
	}
	if createCount != 1 {
		t.Fatalf("expected exactly 1 create, got %d", createCount)
	}

	insertCount := 0
	for _, m := range muts {
		if m.Type == core.MutInsertBefore {
			insertCount++
		}
	}
	if insertCount == 0 {
		t.Fatal("expected at least 1 InsertBefore")
	}
}

func TestKeyedAndUnkeyedMix(t *testing.T) {
	old := []core.Node{
		makeKeyedSpan("k1", "k1"),
		&core.TextNode{Value: "unkeyed1"},
		makeKeyedSpan("k2", "k2"),
	}
	new := []core.Node{
		&core.TextNode{Value: "unkeyed1"},
		makeKeyedSpan("k1", "k1"),
		&core.TextNode{Value: "unkeyed2"},
	}

	r := New()
	r.Render(&core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, &muts)

	createCount := 0
	removeCount := 0
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			createCount++
		}
		if m.Type == core.MutRemoveNode {
			removeCount++
		}
	}
	if createCount != 1 {
		t.Fatalf("expected exactly 1 create (unkeyed2), got %d", createCount)
	}
	if removeCount != 1 {
		t.Fatalf("expected exactly 1 remove (k2), got %d", removeCount)
	}
}

type fakeNode struct {
	ID       int
	ParentID int
	Tag      string
	Children []int
}

type fakeDOM struct {
	nodes map[int]*fakeNode
}

func newFakeDOM() *fakeDOM {
	return &fakeDOM{nodes: map[int]*fakeNode{0: {ID: 0, Tag: "#root"}}}
}

func (d *fakeDOM) apply(muts []core.Mutation) {
	for _, m := range muts {
		switch m.Type {
		case core.MutCreateElement:
			d.nodes[m.NodeID] = &fakeNode{ID: m.NodeID, Tag: m.Value.(string)}
		case core.MutAppendChild:
			if p, ok := d.nodes[m.NodeID]; ok {
				p.Children = append(p.Children, m.ChildID)
				if c, ok := d.nodes[m.ChildID]; ok {
					c.ParentID = m.NodeID
				}
			}
		case core.MutInsertBefore:
			if p, ok := d.nodes[m.NodeID]; ok {
				ins := m.ChildID
				ref := m.RefID
				idx := len(p.Children)
				for i, cid := range p.Children {
					if cid == ref {
						idx = i
						break
					}
				}
				p.Children = append(p.Children, 0)
				copy(p.Children[idx+1:], p.Children[idx:])
				p.Children[idx] = ins
				if c, ok := d.nodes[ins]; ok {
					c.ParentID = m.NodeID
				}
			}
		case core.MutRemoveNode:
			delete(d.nodes, m.NodeID)
			for _, n := range d.nodes {
				for i := len(n.Children) - 1; i >= 0; i-- {
					if n.Children[i] == m.NodeID {
						n.Children = append(n.Children[:i], n.Children[i+1:]...)
					}
				}
			}
		case core.MutSetAttribute:
		case core.MutSetProperty:
		case core.MutRemoveAttribute:
		}
	}
}

func (d *fakeDOM) childTags(parentID int) string {
	p, ok := d.nodes[parentID]
	if !ok {
		return ""
	}
	var s string
	for _, cid := range p.Children {
		if c, ok := d.nodes[cid]; ok {
			s += c.Tag
		}
	}
	return s
}

func TestFakeDOMKeyedProperty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}
	keys := []string{"a", "b", "c", "d", "e"}
	for seed := int64(0); seed < 1000; seed++ {
		oldIdx := pickPerm(seed*2, len(keys), len(keys))
		newIdx := pickPerm(seed*2+1, len(keys), len(keys))

		oldNodes := make([]core.Node, len(oldIdx))
		for i, ki := range oldIdx {
			oldNodes[i] = &core.ElementNode{Tag: keys[ki], Key: keys[ki]}
		}
		newNodes := make([]core.Node, len(newIdx))
		for i, ki := range newIdx {
			newNodes[i] = &core.ElementNode{Tag: keys[ki], Key: keys[ki]}
		}

		r := New()
		r.Render(&core.ElementNode{Tag: "div", Children: oldNodes})

		parentID := 1
		var muts []core.Mutation
		r.diffChildren(parentID, oldNodes, newNodes, &muts)

		fd := newFakeDOM()
		fd.nodes[parentID] = &fakeNode{ID: parentID, Tag: "div"}

		oldSeen := map[int]bool{}
		for _, n := range oldNodes {
			if el, ok := n.(*core.ElementNode); ok {
				fd.nodes[el.ID] = &fakeNode{ID: el.ID, Tag: el.Tag, ParentID: parentID}
				oldSeen[el.ID] = true
			}
		}

		fd.apply(muts)

		fd2 := newFakeDOM()
		fd2.nodes[parentID] = &fakeNode{ID: parentID, Tag: "div"}
		for _, n := range newNodes {
			if el, ok := n.(*core.ElementNode); ok {
				id := el.ID
				if id == 0 {
					id = r.allocID()
				}
				fd2.nodes[id] = &fakeNode{ID: id, Tag: el.Tag, ParentID: parentID}
				fd2.nodes[parentID].Children = append(fd2.nodes[parentID].Children, id)
			}
		}

		got := fd.childTags(parentID)
		want := fd2.childTags(parentID)
		if got != want {
			t.Fatalf("seed=%d: got %q, want %q\nold=%v new=%v muts=%v",
				seed, got, want, oldIdx, newIdx, muts)
		}
	}
}

func pickPerm(seed int64, pool, count int) []int {
	rng := seed
	perm := make([]int, pool)
	for i := 0; i < pool; i++ {
		perm[i] = i
	}
	for i := pool - 1; i > 0; i-- {
		rng = rng*1103515245 + 12345
		if rng < 0 {
			rng = -rng
		}
		j := int(rng % int64(i+1))
		perm[i], perm[j] = perm[j], perm[i]
	}
	n := int(seed%int64(pool)) + 1
	if n > pool {
		n = pool
	}
	return perm[:n]
}

func TestDuplicateKeysLoggedNotPanic(t *testing.T) {
	old := []core.Node{
		makeKeyedSpan("dup", "first"),
	}
	new := []core.Node{
		makeKeyedSpan("dup", "first"),
		makeKeyedSpan("dup", "second"),
	}

	r := New()
	r.Render(&core.ElementNode{Tag: "div", Children: old})
	var muts []core.Mutation
	r.diffChildren(1, old, new, &muts)

	createCount := 0
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			createCount++
		}
	}
	if createCount == 0 {
		t.Fatalf("expected at least 1 create for duplicate, got 0")
	}
}
