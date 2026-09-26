package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// renderFx and flushFx mirror the bridge's frame: render/flush, then run the
// effects that frame queued (they wait for the DOM to be applied).
func renderFx(r *DOMRenderer, n core.Node) ([]core.Mutation, int) {
	muts, id := r.Render(n)
	r.Scheduler.RunEffects()
	return muts, id
}

func flushFx(r *DOMRenderer) []core.Mutation {
	muts := r.Scheduler.Flush()
	r.Scheduler.RunEffects()
	return muts
}

func TestDOMRenderElement(t *testing.T) {
	n := &core.ElementNode{
		Tag:   "div",
		Attrs: []core.Attr{{Name: "class", Value: "greeting"}},
		Children: []core.Node{
			&core.TextNode{Value: "hello"},
		},
	}
	r := New()
	muts, _ := renderFx(r, n)
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
		Tag:   "span",
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
	muts, _ := renderFx(r, n)
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
	renderFx(r, n)

	count.Set(1)
	updates := flushFx(r)
	if len(updates) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(updates))
	}
	// Text is formatted in Go (%v, like SSR) and sent as a string.
	if updates[0].Value != "1" {
		t.Fatalf(`expected "1", got %#v`, updates[0].Value)
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
	muts, _ := renderFx(r, n)
	if len(muts) == 0 {
		t.Fatal("expected mutations")
	}
	name.Set("universe")
	updates := flushFx(r)
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
	muts, _ := renderFx(r, n)
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
	muts, id := renderFx(r, greeting)
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
	muts, _ := renderFx(r, outer)
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
	muts, id := renderFx(r, scope)
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
	initMuts, id := renderFx(r, scope)
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
	diffMuts := flushFx(r)
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
	initMuts, id := renderFx(r, scope)
	if id != 1 || initMuts[0].Value != "div" {
		t.Fatalf("expected div, got %v", initMuts[0])
	}

	show.Set(false)
	diffMuts := flushFx(r)
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}

	hasRemove := false
	hasCreate := false
	hasAppend := false
	for _, m := range diffMuts {
		if m.Type == core.MutCreateElement && m.Value == "span" {
			hasCreate = true
		}
		if m.Type == core.MutRemoveNode && m.NodeID == 1 {
			hasRemove = true
		}
		// New content goes in before the scope's end anchor.
		if m.Type == core.MutInsertBefore && m.RefID == scope.Anchor && m.ChildID != 0 {
			hasAppend = true
		}
	}
	if !hasRemove {
		t.Fatal("expected RemoveNode for old div")
	}
	if !hasCreate {
		t.Fatal("expected CreateElement for new span")
	}
	if !hasAppend {
		t.Fatal("expected the new span inserted before the scope's anchor")
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
	renderFx(r, scope)

	if cleanupCalled != 0 {
		t.Fatalf("expected 0 cleanups before unmount, got %d", cleanupCalled)
	}

	toggle.Set(false)
	diffMuts := flushFx(r)
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}
	if cleanupCalled != 1 {
		t.Fatalf("expected 1 cleanup after unmount, got %d", cleanupCalled)
	}

	toggle.Set(true)
	_ = flushFx(r)
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
	initMuts, rootID := renderFx(r, outerScope)
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
	innerMuts := flushFx(r)
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
	outerMuts := flushFx(r)
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
	renderFx(r, scope)

	show.Set("b")
	muts := flushFx(r)
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
	renderFx(r, scope)
	sig.Set(false)
	muts := flushFx(r)
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
	muts, _ := renderFx(r, n)
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
	initMuts, _ := renderFx(r, outer)
	if len(initMuts) == 0 {
		t.Fatal("expected mutations")
	}

	outerSig.Set(1)
	muts := flushFx(r)
	if len(muts) == 0 {
		t.Fatal("expected mutations after outerSig change")
	}

	innerSig.Set("hello")
	muts2 := flushFx(r)
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
	muts, _ := renderFx(r, n)
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
	renderFx(r, scope)

	sig.Set("b")
	muts := flushFx(r)
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
	renderFx(r, scope)

	sig.Set(1)
	_ = flushFx(r)

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
	renderFx(r, n)

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
	renderFx(r, n)

	r.Bindings.Unbind(1)

	sig.Set(42)
	muts := flushFx(r)
	for _, m := range muts {
		if m.Type == core.MutSetProperty {
			t.Fatal("expected no mutations after unbind")
		}
	}
}

func TestDOMRootMounting(t *testing.T) {
	n := &core.ElementNode{Tag: "div"}
	r := New()
	muts, _ := renderFx(r, n)
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

func TestDOMScopeStructuralChangeWithParent(t *testing.T) {
	sig := core.NewSignal(true)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			if sig.Get() {
				return &core.ElementNode{Tag: "div"}
			}
			return &core.ElementNode{Tag: "span"}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	parent := &core.ElementNode{
		Tag:      "main",
		Children: []core.Node{scope},
	}
	initMuts, _ := renderFx(r, parent)
	_ = initMuts

	sig.Set(false)
	diffMuts := flushFx(r)
	if len(diffMuts) == 0 {
		t.Fatal("expected diff mutations")
	}

	hasInsert := false
	for _, m := range diffMuts {
		if m.Type == core.MutInsertBefore && m.NodeID == 1 {
			hasInsert = true
			break
		}
	}
	if !hasInsert {
		t.Fatalf("expected InsertBefore on parent (node 1), got: %v", diffMuts)
	}
}

// A scope whose root goes element -> empty fragment -> element (the h.Show
// pattern) must re-attach the re-shown element to its parent. Regression for
// the case where the old root has no DOM node to anchor against.
func TestDOMScopeEmptyToElementReattaches(t *testing.T) {
	sig := core.NewSignal(true)
	scope := &core.ScopeNode{
		Render: func() core.Node {
			if sig.Get() {
				return &core.ElementNode{Tag: "p", Children: []core.Node{&core.TextNode{Value: "hi"}}}
			}
			return &core.FragmentNode{}
		},
		Deps: []core.SignalAccessor{sig},
	}

	r := New()
	parent := &core.ElementNode{Tag: "div", Children: []core.Node{scope}}
	renderFx(r, parent)

	sig.Set(false) // hide
	_ = flushFx(r)

	sig.Set(true) // show again
	muts := flushFx(r)

	var createdID int
	for _, m := range muts {
		if m.Type == core.MutCreateElement && m.Value == "p" {
			createdID = m.NodeID
		}
	}
	if createdID == 0 {
		t.Fatalf("expected a <p> to be created on re-show, got: %v", muts)
	}
	attached := false
	for _, m := range muts {
		if (m.Type == core.MutAppendChild || m.Type == core.MutInsertBefore) && m.ChildID == createdID {
			attached = true
			break
		}
	}
	if !attached {
		t.Fatalf("re-shown <p> (id=%d) was never attached to a parent, got: %v", createdID, muts)
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
	renderFx(r, scope)

	sig.Set(1)
	muts := flushFx(r)
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
		Tag:   "span",
		Key:   key,
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
	renderFx(r, &core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, rev, 0, &muts)

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
	renderFx(r, &core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, 0, &muts)

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
	renderFx(r, &core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, 0, &muts)

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
	renderFx(r, &core.ElementNode{Tag: "div", Children: old})
	parentID := 1
	var muts []core.Mutation
	r.diffChildren(parentID, old, new, 0, &muts)

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
	attached bool
}

type fakeDOM struct {
	nodes map[int]*fakeNode
}

func newFakeDOM() *fakeDOM {
	return &fakeDOM{nodes: map[int]*fakeNode{0: {ID: 0, Tag: "#root", attached: true}}}
}

// detach removes n from its current parent's child list (a DOM move).
func (d *fakeDOM) detach(n *fakeNode) {
	if !n.attached {
		return
	}
	if p, ok := d.nodes[n.ParentID]; ok {
		for i := len(p.Children) - 1; i >= 0; i-- {
			if p.Children[i] == n.ID {
				p.Children = append(p.Children[:i], p.Children[i+1:]...)
			}
		}
	}
	n.attached = false
}

// apply mirrors runtime/goowee.js: AppendChild only attaches a detached node;
// InsertBefore moves the node and inserts at the reference's actual parent.
func (d *fakeDOM) apply(muts []core.Mutation) {
	for _, m := range muts {
		switch m.Type {
		case core.MutCreateElement:
			d.nodes[m.NodeID] = &fakeNode{ID: m.NodeID, Tag: m.Value.(string)}
		case core.MutAppendChild:
			p, c := d.nodes[m.NodeID], d.nodes[m.ChildID]
			if p != nil && c != nil && !c.attached {
				p.Children = append(p.Children, c.ID)
				c.ParentID, c.attached = p.ID, true
			}
		case core.MutInsertBefore:
			c := d.nodes[m.ChildID]
			if c == nil {
				continue
			}
			pid := m.NodeID
			if rn := d.nodes[m.RefID]; m.RefID != 0 && rn != nil && rn.attached {
				pid = rn.ParentID
			}
			p := d.nodes[pid]
			if p == nil {
				continue
			}
			d.detach(c)
			idx := len(p.Children)
			for i, cid := range p.Children {
				if cid == m.RefID {
					idx = i
					break
				}
			}
			p.Children = append(p.Children, 0)
			copy(p.Children[idx+1:], p.Children[idx:])
			p.Children[idx] = c.ID
			c.ParentID, c.attached = p.ID, true
		case core.MutRemoveNode:
			if n := d.nodes[m.NodeID]; n != nil {
				d.detach(n)
				delete(d.nodes, m.NodeID)
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
		renderFx(r, &core.ElementNode{Tag: "div", Children: oldNodes})

		parentID := 1
		var muts []core.Mutation
		r.diffChildren(parentID, oldNodes, newNodes, 0, &muts)

		fd := newFakeDOM()
		fd.nodes[parentID] = &fakeNode{ID: parentID, Tag: "div"}

		for _, n := range oldNodes {
			if el, ok := n.(*core.ElementNode); ok {
				fd.nodes[el.ID] = &fakeNode{ID: el.ID, Tag: el.Tag, ParentID: parentID, attached: true}
				fd.nodes[parentID].Children = append(fd.nodes[parentID].Children, el.ID)
			}
		}

		fd.apply(muts)

		fd2 := newFakeDOM()
		fd2.nodes[parentID] = &fakeNode{ID: parentID, Tag: "div"}
		for _, n := range newNodes {
			if el, ok := n.(*core.ElementNode); ok {
				id := el.ID
				if id == 0 {
					id = r.AllocID()
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
	renderFx(r, &core.ElementNode{Tag: "div", Children: old})
	var muts []core.Mutation
	r.diffChildren(1, old, new, 0, &muts)

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

// Shrinking an unkeyed list must emit exactly one RemoveNode per removed
// subtree (regression: surplus children were removed twice).
func TestPositionalShrinkRemovesOnce(t *testing.T) {
	old := []core.Node{
		&core.ElementNode{Tag: "li", Children: []core.Node{&core.TextNode{Value: "a"}}},
		&core.ElementNode{Tag: "li", Children: []core.Node{&core.TextNode{Value: "b"}}},
		&core.ElementNode{Tag: "li", Children: []core.Node{&core.TextNode{Value: "c"}}},
	}
	r := New()
	parent := &core.ElementNode{Tag: "ul", Children: old}
	renderFx(r, parent)

	newKids := []core.Node{
		&core.ElementNode{Tag: "li", Children: []core.Node{&core.TextNode{Value: "a"}}},
	}
	var muts []core.Mutation
	r.diffChildren(parent.ID, old, newKids, 0, &muts)

	removes := map[int]int{}
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			removes[m.NodeID]++
		}
	}
	for id, n := range removes {
		if n != 1 {
			t.Errorf("node %d removed %d times, want exactly 1", id, n)
		}
	}
}

// The heart of the Solid "components run once" model: a component nested in a
// scope must keep its identity — setup and effects run exactly once — even
// when the scope re-renders for an unrelated reason. Regression for the old
// behavior where every scope re-render tore down and re-ran all nested
// components.
func TestComponentPreservedAcrossScopeReRender(t *testing.T) {
	other := core.NewSignal(0)
	setups, effectRuns, cleanups := 0, 0, 0

	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{other},
		Render: func() core.Node {
			return &core.ElementNode{Tag: "div", Children: []core.Node{
				core.Component("Stable", func() core.Node {
					setups++
					hooks.OnMount(func() func() {
						effectRuns++
						return func() { cleanups++ }
					})
					return &core.ElementNode{Tag: "span"}
				}),
				&core.TextNode{Value: fmt.Sprintf("%d", other.Get())},
			}}
		},
	}

	r := New()
	renderFx(r, scope)
	if setups != 1 || effectRuns != 1 || cleanups != 0 {
		t.Fatalf("after mount want 1/1/0, got setups=%d effectRuns=%d cleanups=%d", setups, effectRuns, cleanups)
	}

	other.Set(1) // re-render the scope for an unrelated reason
	flushFx(r)
	if setups != 1 || effectRuns != 1 || cleanups != 0 {
		t.Fatalf("component churned on unrelated re-render: setups=%d effectRuns=%d cleanups=%d, want 1/1/0", setups, effectRuns, cleanups)
	}

	other.Set(2)
	flushFx(r)
	if setups != 1 || effectRuns != 1 || cleanups != 0 {
		t.Fatalf("component churned on 2nd re-render: setups=%d effectRuns=%d cleanups=%d, want 1/1/0", setups, effectRuns, cleanups)
	}
}

// Component added to / removed from a scope mounts (setup once) / unmounts
// (cleanup once); re-showing mounts a fresh instance.
func TestComponentMountUnmountInScope(t *testing.T) {
	shown := core.NewSignal(true)
	setups, cleanups := 0, 0

	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{shown},
		Render: func() core.Node {
			if shown.Get() {
				return core.Component("C", func() core.Node {
					setups++
					hooks.OnMount(func() func() { return func() { cleanups++ } })
					return &core.ElementNode{Tag: "p"}
				})
			}
			return &core.FragmentNode{}
		},
	}

	r := New()
	renderFx(r, scope)
	if setups != 1 || cleanups != 0 {
		t.Fatalf("after mount want 1/0, got %d/%d", setups, cleanups)
	}

	shown.Set(false) // unmount
	flushFx(r)
	if setups != 1 || cleanups != 1 {
		t.Fatalf("after unmount want 1/1, got %d/%d", setups, cleanups)
	}

	shown.Set(true) // remount, fresh instance
	flushFx(r)
	if setups != 2 || cleanups != 1 {
		t.Fatalf("after remount want 2/1, got %d/%d", setups, cleanups)
	}
}

// A fragment-rooted list (what For produces) nested in an element must attach
// its rows to that element — on initial render and on later inserts — not to
// the root. Regression for fragments orphaning / attaching to node 0.
func TestFragmentListAttachesToParent(t *testing.T) {
	order := core.NewSignal([]int{1, 2})
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{order},
		Render: func() core.Node {
			var kids []core.Node
			for _, id := range order.Get() {
				kids = append(kids, &core.ElementNode{Tag: "li", Key: id})
			}
			return &core.FragmentNode{Children: kids}
		},
	}
	ul := &core.ElementNode{Tag: "ul", Children: []core.Node{scope}}
	r := New()
	initMuts, _ := renderFx(r, ul)

	appended := 0
	for _, m := range initMuts {
		if m.Type == core.MutAppendChild && m.NodeID == ul.ID {
			appended++
		}
	}
	if appended != 3 { // two rows, then the scope's end anchor
		t.Fatalf("want 2 <li> + anchor appended under <ul> on init, got %d (muts=%v)", appended, initMuts)
	}

	order.Set([]int{1, 2, 3}) // append a third
	muts := flushFx(r)
	insertedUnderUl := false
	for _, m := range muts {
		if m.Type == core.MutInsertBefore && m.NodeID == ul.ID {
			insertedUnderUl = true
		}
		if (m.Type == core.MutInsertBefore || m.Type == core.MutAppendChild) && m.NodeID == 0 {
			t.Fatalf("list row wrongly placed under root: %+v", m)
		}
	}
	if !insertedUnderUl {
		t.Fatalf("expected new <li> inserted under <ul>, got %v", muts)
	}
}

// Reordering a keyed list of *components* must reuse each component (setup and
// effects run once, output DOM reused) and reorder within the real parent —
// the whole point of keyed matching for component rows.
func TestKeyedComponentListReorderPreservesIdentity(t *testing.T) {
	order := core.NewSignal([]int{1, 2, 3})
	setups := map[int]int{}
	cleanups := map[int]int{}

	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{order},
		Render: func() core.Node {
			var kids []core.Node
			for _, id := range order.Get() {
				id := id
				c := core.Component("Row", func() core.Node {
					setups[id]++
					hooks.OnMount(func() func() { return func() { cleanups[id]++ } })
					return &core.ElementNode{Tag: "li"}
				})
				c.Key = id
				kids = append(kids, c)
			}
			return &core.FragmentNode{Children: kids}
		},
	}
	ul := &core.ElementNode{Tag: "ul", Children: []core.Node{scope}}
	r := New()
	initMuts, _ := renderFx(r, ul)

	outID := map[int]int{}
	for _, n := range scope.Prev.(*core.FragmentNode).Children {
		c := n.(*core.ComponentNode)
		outID[c.Key.(int)] = firstRoot(c.Prev)
	}
	for id := 1; id <= 3; id++ {
		if setups[id] != 1 || outID[id] == 0 {
			t.Fatalf("mount id=%d: setups=%d outID=%d", id, setups[id], outID[id])
		}
	}
	appended := map[int]bool{}
	for _, m := range initMuts {
		if m.Type == core.MutAppendChild && m.NodeID == ul.ID {
			appended[m.ChildID] = true
		}
	}
	for id := 1; id <= 3; id++ {
		if !appended[outID[id]] {
			t.Fatalf("component row %d (node %d) not attached under <ul> on init", id, outID[id])
		}
	}

	order.Set([]int{3, 1, 2}) // reorder
	muts := flushFx(r)

	for id := 1; id <= 3; id++ {
		if setups[id] != 1 {
			t.Fatalf("component %d re-mounted on reorder: setups=%d, want 1", id, setups[id])
		}
		if cleanups[id] != 0 {
			t.Fatalf("component %d unmounted on reorder: cleanups=%d, want 0", id, cleanups[id])
		}
	}
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			t.Fatalf("reorder created a node (should reuse all): %+v", m)
		}
	}
	// Identity preserved: each key still maps to its original output node.
	for _, n := range scope.Prev.(*core.FragmentNode).Children {
		c := n.(*core.ComponentNode)
		if got := firstRoot(c.Prev); got != outID[c.Key.(int)] {
			t.Fatalf("component %v output changed %d -> %d (identity lost)", c.Key, outID[c.Key.(int)], got)
		}
	}
	reorderedUnderUl := false
	for _, m := range muts {
		if m.Type == core.MutInsertBefore && m.NodeID == ul.ID {
			reorderedUnderUl = true
		}
	}
	if !reorderedUnderUl {
		t.Fatalf("expected reorder InsertBefore under <ul>, got %v", muts)
	}
}

// The same guarantee as above, but driven through the public h.For helper and
// covering the shapes a real list UI hits: appending and removing items, not
// just reordering them. A row's component state (and its effects) must survive
// any list change that keeps its key, and a removed row must dispose exactly
// once. h/flow.go once documented the opposite as a limitation — this test
// exists so that claim can't quietly become true again.
func TestForRowComponentStateSurvivesListChanges(t *testing.T) {
	items := core.NewSignal([]int{1, 2, 3})
	setups := map[int]int{}
	cleanups := map[int]int{}
	counts := map[int]*core.Signal[int]{}

	tree := &core.ElementNode{Tag: "ul", Children: []core.Node{
		h.For(items, func(i int) int { return i }, func(i int) core.Node {
			return core.Component("Row", func() core.Node {
				setups[i]++
				count, _ := hooks.UseState(0)
				counts[i] = count
				hooks.OnMount(func() func() { return func() { cleanups[i]++ } })
				return &core.ElementNode{Tag: "li"}
			})
		}),
	}}

	r := New()
	renderFx(r, tree)
	counts[2].Set(42) // give row 2 some state to lose

	items.Set([]int{1, 2, 3, 4}) // append
	flushFx(r)
	for _, id := range []int{1, 2, 3} {
		if setups[id] != 1 || cleanups[id] != 0 {
			t.Fatalf("append churned row %d: setups=%d cleanups=%d, want 1/0", id, setups[id], cleanups[id])
		}
	}
	if got := counts[2].Get(); got != 42 {
		t.Fatalf("append lost row 2 state: got %d, want 42", got)
	}

	items.Set([]int{1, 3, 4}) // remove the middle row
	flushFx(r)
	if cleanups[2] != 1 {
		t.Fatalf("removed row 2 disposed %d times, want 1", cleanups[2])
	}
	for _, id := range []int{1, 3, 4} {
		if setups[id] != 1 || cleanups[id] != 0 {
			t.Fatalf("remove churned surviving row %d: setups=%d cleanups=%d, want 1/0", id, setups[id], cleanups[id])
		}
	}

	items.Set([]int{4, 1, 3}) // reorder
	flushFx(r)
	for _, id := range []int{1, 3, 4} {
		if setups[id] != 1 || cleanups[id] != 0 {
			t.Fatalf("reorder churned row %d: setups=%d cleanups=%d, want 1/0", id, setups[id], cleanups[id])
		}
	}
}

// A row that owns an inner scope (the expand/collapse shape) keeps that scope's
// state across list changes too — the case that pushed app authors to hoist row
// state out of For.
func TestForRowInnerScopeStateSurvivesListChanges(t *testing.T) {
	items := core.NewSignal([]int{1, 2})
	setups := map[int]int{}
	open := map[int]*core.Signal[bool]{}

	tree := &core.ElementNode{Tag: "ul", Children: []core.Node{
		h.For(items, func(i int) int { return i }, func(i int) core.Node {
			return core.Component("Row", func() core.Node {
				setups[i]++
				expanded, _ := hooks.UseState(false)
				open[i] = expanded
				return &core.ElementNode{Tag: "li", Children: []core.Node{
					h.Show(expanded, func() core.Node {
						return &core.ElementNode{Tag: "p"}
					}),
				}}
			})
		}),
	}}

	r := New()
	renderFx(r, tree)
	open[1].Set(true) // expand row 1
	flushFx(r)

	items.Set([]int{1, 2, 3})
	flushFx(r)
	if setups[1] != 1 {
		t.Fatalf("row 1 setup re-ran on append: %d", setups[1])
	}
	if !open[1].Get() {
		t.Fatalf("row 1 lost its expanded state across a list change")
	}
}

// A component nested inside a keyed element row (rather than being the row
// root) is preserved by the row's reuse.
func TestNestedComponentInKeyedRowPreserved(t *testing.T) {
	items := core.NewSignal([]int{1, 2, 3})
	setups := map[int]int{}
	cleanups := map[int]int{}

	tree := &core.ElementNode{Tag: "ul", Children: []core.Node{
		h.For(items, func(i int) int { return i }, func(i int) core.Node {
			return &core.ElementNode{Tag: "li", Children: []core.Node{
				core.Component("Inner", func() core.Node {
					setups[i]++
					hooks.OnMount(func() func() { return func() { cleanups[i]++ } })
					return &core.ElementNode{Tag: "span"}
				}),
			}}
		}),
	}}

	r := New()
	renderFx(r, tree)
	items.Set([]int{1, 2, 3, 4})
	flushFx(r)
	items.Set([]int{4, 2, 1}) // drops 3, reorders the rest
	flushFx(r)

	for _, id := range []int{1, 2, 4} {
		if setups[id] != 1 || cleanups[id] != 0 {
			t.Fatalf("row %d churned: setups=%d cleanups=%d, want 1/0", id, setups[id], cleanups[id])
		}
	}
	if cleanups[3] != 1 {
		t.Fatalf("removed row 3 disposed %d times, want 1", cleanups[3])
	}
}

// Multiple signal writes in one frame must produce a single scope re-render,
// not one per write (dirty-scope batching).
func TestSchedulerBatchesScopeReRenders(t *testing.T) {
	a := core.NewSignal(0)
	b := core.NewSignal(0)
	renders := 0
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{a, b},
		Render: func() core.Node {
			renders++
			return &core.ElementNode{Tag: "div", Attrs: []core.Attr{
				{Name: "data-v", Value: fmt.Sprintf("%d-%d", a.Get(), b.Get())},
			}}
		},
	}
	r := New()
	renderFx(r, scope)
	if renders != 1 {
		t.Fatalf("initial renders=%d, want 1", renders)
	}

	a.Set(1)
	a.Set(2)
	b.Set(5)
	if renders != 1 {
		t.Fatalf("re-render happened before flush (not batched): renders=%d", renders)
	}

	muts := flushFx(r)
	if renders != 2 {
		t.Fatalf("3 writes should batch into 1 re-render (renders=2), got renders=%d", renders)
	}
	// And the single write reflects the final state, coalesced.
	setV := 0
	for _, m := range muts {
		if m.Type == core.MutSetAttribute && m.Key == "data-v" {
			setV++
			if m.Value != "2-5" {
				t.Fatalf("data-v should reflect final state 2-5, got %v", m.Value)
			}
		}
	}
	if setV != 1 {
		t.Fatalf("expected a single coalesced data-v write, got %d", setV)
	}
}

// A parent scope that re-renders and removes a dirty child in the same frame
// must cancel the child's pending re-render (no re-render of a removed subtree).
func TestParentReRenderCancelsDirtyChild(t *testing.T) {
	show := core.NewSignal(true)
	inner := core.NewSignal(0)
	childRenders := 0

	childScope := &core.ScopeNode{
		Deps: []core.SignalAccessor{inner},
		Render: func() core.Node {
			childRenders++
			return &core.ElementNode{Tag: "span", Attrs: []core.Attr{
				{Name: "v", Value: fmt.Sprintf("%d", inner.Get())},
			}}
		},
	}
	parent := &core.ScopeNode{
		Deps: []core.SignalAccessor{show},
		Render: func() core.Node {
			if show.Get() {
				return &core.ElementNode{Tag: "div", Children: []core.Node{childScope}}
			}
			return &core.ElementNode{Tag: "p"}
		},
	}
	root := &core.ElementNode{Tag: "main", Children: []core.Node{parent}}
	r := New()
	renderFx(r, root)
	if childRenders != 1 {
		t.Fatalf("initial child renders=%d, want 1", childRenders)
	}

	// Same frame: dirty the child, and remove it via the parent.
	inner.Set(1)
	show.Set(false)
	flushFx(r)

	if childRenders != 1 {
		t.Fatalf("child re-rendered after parent removed it: childRenders=%d, want 1", childRenders)
	}
}

func TestSVGNamespacePropagation(t *testing.T) {
	// <section><svg><g><path/></g></svg><div/></section>: the svg and its
	// descendants carry the SVG namespace on CreateElement; the div does not.
	tree := &core.ElementNode{Tag: "section", Children: []core.Node{
		&core.ElementNode{Tag: "svg", Namespace: core.SVGNamespace, Children: []core.Node{
			&core.ElementNode{Tag: "g", Children: []core.Node{
				&core.ElementNode{Tag: "path"},
			}},
		}},
		&core.ElementNode{Tag: "div"},
	}}

	muts, _ := New().Render(tree)
	ns := map[string]string{}
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			ns[m.Value.(string)] = m.NS
		}
	}
	for _, tag := range []string{"svg", "g", "path"} {
		if ns[tag] != core.SVGNamespace {
			t.Errorf("<%s> should carry SVG namespace, got %q", tag, ns[tag])
		}
	}
	if ns["section"] != "" || ns["div"] != "" {
		t.Errorf("HTML elements must not be namespaced: section=%q div=%q", ns["section"], ns["div"])
	}
}

func TestRefIDSetOnRender(t *testing.T) {
	ref := &core.Ref{}
	el := &core.ElementNode{Tag: "input", Ref: ref}
	New().Render(el)
	if ref.ID == 0 || ref.ID != el.ID {
		t.Fatalf("ref.ID=%d should equal rendered el.ID=%d", ref.ID, el.ID)
	}
}

func TestPortalEmitsPortalAppend(t *testing.T) {
	portal := &core.PortalNode{Target: "#modal", Children: []core.Node{
		&core.ElementNode{Tag: "span", Children: []core.Node{&core.TextNode{Value: "hi"}}},
	}}
	muts, _ := New().Render(portal)
	appends, creates := 0, 0
	for _, m := range muts {
		switch m.Type {
		case core.MutPortalAppend:
			appends++
			if m.Value != "#modal" {
				t.Fatalf("portal target=%v, want #modal", m.Value)
			}
		case core.MutCreateElement:
			creates++
		}
	}
	if appends != 1 {
		t.Fatalf("want 1 PortalAppend, got %d", appends)
	}
	if creates < 2 {
		t.Fatalf("want span+text created, got %d", creates)
	}
}

func TestPortalRendersFreshDuringHydration(t *testing.T) {
	portal := &core.PortalNode{Target: "#modal", Children: []core.Node{
		&core.ElementNode{Tag: "div"},
	}}
	r := New()
	r.SetHydrating(true)
	muts, _ := renderFx(r, portal)
	sawCreate, sawHydrate := false, false
	for _, m := range muts {
		switch m.Type {
		case core.MutCreateElement:
			sawCreate = true
		case core.MutHydrate:
			sawHydrate = true
		}
	}
	if !sawCreate {
		t.Fatal("portal content must be created fresh during hydration")
	}
	if sawHydrate {
		t.Fatal("portal content must not emit a hydrate claim (server doesn't render portals)")
	}
}

func TestErrorBoundaryCatchesInitialPanic(t *testing.T) {
	b := &core.ErrorBoundaryNode{
		Fallback: func(err any) core.Node {
			return &core.ElementNode{Tag: "p", Children: []core.Node{&core.TextNode{Value: "fallback"}}}
		},
		Child: core.Component("Boom", func() core.Node { panic("boom") }),
	}
	muts, id := New().Render(b) // must not panic
	if id == 0 {
		t.Fatal("expected a fallback root node id")
	}
	found := false
	for _, m := range muts {
		if m.Type == core.MutSetProperty && m.Key == "textContent" && m.Value == "fallback" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the fallback to render after catching the panic")
	}
}

func TestErrorBoundaryPassesThroughWhenOK(t *testing.T) {
	b := &core.ErrorBoundaryNode{
		Fallback: func(err any) core.Node { return &core.ElementNode{Tag: "p"} },
		Child:    &core.ElementNode{Tag: "span", Children: []core.Node{&core.TextNode{Value: "ok"}}},
	}
	muts, _ := New().Render(b)
	for _, m := range muts {
		if m.Type == core.MutCreateElement && m.Value == "p" {
			t.Fatal("fallback rendered despite no panic")
		}
	}
	ok := false
	for _, m := range muts {
		if m.Type == core.MutSetProperty && m.Value == "ok" {
			ok = true
		}
	}
	if !ok {
		t.Fatal("child content not rendered")
	}
}

func TestReRenderPanicIsContained(t *testing.T) {
	trigger := core.NewSignal(false)
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{trigger},
		Render: func() core.Node {
			if trigger.Get() {
				panic("re-render boom")
			}
			return &core.ElementNode{Tag: "div"}
		},
	}
	r := New()
	renderFx(r, scope) // initial render OK
	trigger.Set(true)
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Flush should contain the re-render panic, but it propagated: %v", rec)
		}
	}()
	flushFx(r) // must not panic — contained + logged
}
