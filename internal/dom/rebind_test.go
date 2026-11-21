package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// boundRows is a scope whose re-render is triggered by tick and rebuilds a
// list of n rows, each with a bound class and bound text.
func boundRows(n int, tick core.SignalAccessor, labels []*core.Signal[string]) *core.ScopeNode {
	return &core.ScopeNode{
		Deps: []core.SignalAccessor{tick},
		Render: func() core.Node {
			rows := make([]core.Node, n)
			for i := range rows {
				rows[i] = h.Li(h.ClassS(labels[i]), h.TextS(labels[i]))
			}
			return h.Ul(h.Nodes(rows)...)
		},
	}
}

// #137: a re-render that changes no bound signal sends nothing.
func TestReRenderWithUnchangedBindingsSendsNothing(t *testing.T) {
	tick := core.NewSignal(0)
	labels := make([]*core.Signal[string], 100)
	for i := range labels {
		labels[i] = core.NewSignal(fmt.Sprintf("row-%d", i))
	}
	r := New()
	renderFx(r, boundRows(100, tick, labels))
	tick.Set(1)
	if muts := flushFx(r); len(muts) != 0 {
		t.Fatalf("a no-op re-render sent %d mutations, e.g. %+v", len(muts), muts[0])
	}
	// The kept bindings still work.
	labels[3].Set("changed")
	muts := flushFx(r)
	if len(muts) != 2 {
		t.Fatalf("one signal change sent %d mutations: %+v", len(muts), muts)
	}
}

// A re-render that swaps a binding's signal rebinds: the new signal drives
// the node, the old one doesn't.
func TestReRenderSwapsABindingsSignal(t *testing.T) {
	a, b := core.NewSignal("a"), core.NewSignal("b")
	useB := core.NewSignal(false)
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{useB},
		Render: func() core.Node {
			s := a
			if useB.Get() {
				s = b
			}
			return h.Div(h.ClassS(s), h.TextS(s))
		},
	}
	r := New()
	renderFx(r, scope)
	useB.Set(true)
	muts := flushFx(r)
	got := map[string]any{}
	for _, m := range muts {
		got[m.Key] = m.Value
	}
	if got["class"] != "b" || got["textContent"] != "b" {
		t.Fatalf("after the swap: %+v", muts)
	}
	a.Set("a2")
	if muts := flushFx(r); len(muts) != 0 {
		t.Fatalf("the old signal still drives the node: %+v", muts)
	}
	b.Set("b2")
	if muts := flushFx(r); len(muts) != 2 {
		t.Fatalf("the new signal doesn't drive the node: %+v", muts)
	}
}

// A per-render Computed (Textf) with the same value sends nothing; one whose
// value changed sends it.
func TestReRenderComputedBindingComparesValues(t *testing.T) {
	tick, n := core.NewSignal(0), core.NewSignal(1)
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{tick},
		Render: func() core.Node {
			return h.P(h.Textf("n=%d", n))
		},
	}
	r := New()
	renderFx(r, scope)
	tick.Set(1)
	if muts := flushFx(r); len(muts) != 0 {
		t.Fatalf("same value re-sent: %+v", muts)
	}
	n.Set(2)
	muts := flushFx(r)
	if len(muts) != 1 || muts[0].Value != "n=2" {
		t.Fatalf("after n changed: %+v", muts)
	}
	tick.Set(2)
	n.Set(3) // the old Computed and the new render both see 3
	muts = flushFx(r)
	last := ""
	for _, m := range muts {
		if m.Key == "textContent" {
			last, _ = m.Value.(string)
		}
	}
	if last != "n=3" {
		t.Fatalf("text after a re-render and a change in one frame: %+v", muts)
	}
}

// Text switching between static and bound sends only real changes.
func TestReRenderTextStaticAndBound(t *testing.T) {
	bound := core.NewSignal(false)
	s := core.NewSignal("same")
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{bound},
		Render: func() core.Node {
			if bound.Get() {
				return h.P(h.TextS(s))
			}
			return h.P(h.Text("same"))
		},
	}
	r := New()
	renderFx(r, scope)
	bound.Set(true) // static "same" → bound "same": nothing to send
	if muts := flushFx(r); len(muts) != 0 {
		t.Fatalf("static to bound with the same text: %+v", muts)
	}
	s.Set("changed") // now bound
	if muts := flushFx(r); len(muts) != 1 || muts[0].Value != "changed" {
		t.Fatalf("bound text change: %+v", muts)
	}
	bound.Set(false) // bound "changed" → static "same": send it, and unbind
	if muts := flushFx(r); len(muts) != 1 || muts[0].Value != "same" {
		t.Fatalf("bound to static: %+v", muts)
	}
	s.Set("ignored")
	if muts := flushFx(r); len(muts) != 0 {
		t.Fatalf("an unbound signal still drives the text: %+v", muts)
	}
}

// Duplicate attribute names: the last one wins, before and after a diff.
func TestDiffDuplicateAttrsLastWins(t *testing.T) {
	v := core.NewSignal(0)
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{v},
		Render: func() core.Node {
			if v.Get() == 0 {
				return &core.ElementNode{Tag: "div", Attrs: []core.Attr{{Name: "class", Value: "b"}}}
			}
			return &core.ElementNode{Tag: "div", Attrs: []core.Attr{{Name: "class", Value: "a"}, {Name: "class", Value: "b"}}}
		},
	}
	r := New()
	renderFx(r, scope)
	v.Set(1)
	for _, m := range flushFx(r) {
		if m.Key == "class" && m.Value != "b" {
			t.Fatalf("class set to %v; the last attribute (b) must win", m.Value)
		}
	}
}
