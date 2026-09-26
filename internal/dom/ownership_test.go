package dom

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// tree renders the fake DOM under id as tag(children…), skipping scope anchors.
func (d *fakeDOM) tree(id int) string {
	n := d.nodes[id]
	if n == nil {
		return "?"
	}
	var kids []string
	for _, c := range n.Children {
		if cn := d.nodes[c]; cn != nil && cn.Tag == "#comment" {
			continue
		}
		kids = append(kids, d.tree(c))
	}
	if len(kids) == 0 {
		return n.Tag
	}
	return n.Tag + "(" + strings.Join(kids, " ") + ")"
}

func mount(t *testing.T, n core.Node) (*DOMRenderer, *fakeDOM) {
	t.Helper()
	r := New()
	d := newFakeDOM()
	muts, _ := r.Render(n)
	d.apply(muts)
	r.Scheduler.RunEffects()
	return r, d
}

func flush(r *DOMRenderer, d *fakeDOM) {
	d.apply(r.Scheduler.Flush())
	r.Scheduler.RunEffects()
}

// #58: Textf inside a re-rendering scope used to leave its Computed subscribed
// to b forever — one more subscriber per re-render.
func TestTextfInReRenderingScopeDoesNotLeak(t *testing.T) {
	a, b := core.NewSignal(0), core.NewSignal(0)
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		_ = a.Get()
		return h.P(h.Textf("b=%d", b))
	}, a)))
	for i := 1; i <= 50; i++ {
		a.Set(i)
		flush(r, d)
	}
	if n := b.SubscriberCount(); n != 1 {
		t.Fatalf("b should have exactly the live Textf's subscription, got %d", n)
	}
}

// #58: hooks called directly in a scope's render belong to that render: a
// re-render disposes the previous render's Watch/OnMount, and removing the
// scope disposes the current one.
func TestHooksInScopeRenderAreOwnedByTheRender(t *testing.T) {
	branch := core.NewSignal(0)
	src := core.NewSignal(0)
	watchRuns, cleanups, active := 0, 0, 0
	show := core.NewSignal(true)
	r, d := mount(t, h.Div(h.Show(show, func() core.Node {
		return hooks.UseScope(func() core.Node {
			_ = branch.Get()
			hooks.Watch([]core.SignalAccessor{src}, func() { watchRuns++ })
			hooks.OnMount(func() func() { active++; return func() { active--; cleanups++ } })
			return h.Span()
		}, branch)
	})))
	for i := 1; i <= 5; i++ {
		branch.Set(i)
		flush(r, d)
	}
	src.Set(1)
	if watchRuns != 1 {
		t.Fatalf("only the live render's Watch may fire, got %d runs", watchRuns)
	}
	if active != 1 || cleanups != 5 {
		t.Fatalf("previous renders' OnMount cleanups must run: active=%d cleanups=%d", active, cleanups)
	}
	show.Set(false)
	flush(r, d)
	if active != 0 || src.SubscriberCount() != 0 {
		t.Fatalf("removing the scope disposes its render's hooks: active=%d subs=%d", active, src.SubscriberCount())
	}
}

// #59: a component inside a Show inside a keyed element row used to be
// remounted whenever the list changed (the nested scope was rebuilt), and the
// rebuilt scope remembered the wrong parent.
func TestNestedScopeIsAdoptedNotRebuilt(t *testing.T) {
	items := core.NewSignal([]int{1, 2})
	show := core.NewSignal(true)
	mounts := 0
	inner := func() core.Node {
		return core.Component("Inner", func() core.Node { mounts++; return h.Span(h.Text("in")) })
	}
	r, d := mount(t, h.Ul(h.For(items, func(i int) int { return i }, func(i int) core.Node {
		return h.Li(h.Show(show, inner))
	})))
	items.Set([]int{1, 2, 3})
	flush(r, d)
	if mounts != 3 {
		t.Fatalf("existing rows' components must survive; mounts=%d want 3", mounts)
	}
	show.Set(false)
	flush(r, d)
	show.Set(true)
	flush(r, d)
	if got, want := d.tree(0), "#root(ul(li(span(#text)) li(span(#text)) li(span(#text))))"; got != want {
		t.Fatalf("toggled content must stay inside its row\n got %s\nwant %s", got, want)
	}
}

// #83: content a scope renders as several roots is placed before the scope's
// following siblings.
func TestMultiRootScopeContentKeepsSiblingOrder(t *testing.T) {
	items := core.NewSignal([]string{"a"})
	r, d := mount(t, h.Ul(
		h.For(items, func(s string) string { return s }, func(s string) core.Node { return h.Li(h.Text(s)) }),
		h.P(h.Text("footer")),
	))
	items.Set([]string{"a", "b"})
	flush(r, d)
	if got, want := d.tree(0), "#root(ul(li(#text) li(#text) p(#text)))"; got != want {
		t.Fatalf("appended row must precede the footer\n got %s\nwant %s", got, want)
	}

	cond := core.NewSignal(false)
	r2, d2 := mount(t, h.Div(
		h.Show(cond, func() core.Node { return h.Fragment(h.Span(h.Text("1")), h.B(h.Text("2"))) }),
		h.P(h.Text("after")),
	))
	cond.Set(true)
	flush(r2, d2)
	if got, want := d2.tree(0), "#root(div(span(#text) b(#text) p(#text)))"; got != want {
		t.Fatalf("multi-root Show content must precede its sibling\n got %s\nwant %s", got, want)
	}
	cond.Set(false)
	flush(r2, d2)
	if got, want := d2.tree(0), "#root(div(p(#text)))"; got != want {
		t.Fatalf("hiding removes both roots\n got %s\nwant %s", got, want)
	}
}

// #83: a scope child replaced by an element (positional diff) used to render
// nothing at all.
func TestScopeReplacedByElementRenders(t *testing.T) {
	flip := core.NewSignal(false)
	inner := core.NewSignal(true)
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		if flip.Get() {
			return h.Section(h.B(h.Text("elem")))
		}
		return h.Section(h.Show(inner, func() core.Node { return h.I(h.Text("scoped")) }))
	}, flip)))
	flip.Set(true)
	flush(r, d)
	if got, want := d.tree(0), "#root(div(section(b(#text))))"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if inner.SubscriberCount() != 0 {
		t.Fatalf("the replaced scope must unsubscribe, got %d", inner.SubscriberCount())
	}
}

// #83: random keyed list edits inside a list that has a leading and a
// trailing sibling always leave exactly head, the rows in order, tail.
func TestKeyedListBetweenSiblingsProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	items := core.NewSignal([]int{})
	list := h.For(items, func(i int) int { return i }, func(i int) core.Node {
		return h.Li(h.Text(fmt.Sprint(i)))
	}).(*core.ScopeNode)
	head, tail := h.Li(h.Text("head")), h.Li(h.Text("tail"))
	r, d := mount(t, h.Ul(head, list, tail))
	ul := d.nodes[0].Children[0]
	for step := 0; step < 400; step++ {
		perm := rng.Perm(12)[:rng.Intn(9)]
		items.Set(perm)
		flush(r, d)

		want := []int{head.ID}
		if frag, ok := list.Prev.(*core.FragmentNode); ok {
			for i, row := range frag.Children {
				el := row.(*core.ElementNode)
				if el.Key != perm[i] {
					t.Fatalf("step %d: row %d has key %v, want %d", step, i, el.Key, perm[i])
				}
				want = append(want, el.ID)
			}
		}
		want = append(want, tail.ID)
		var got []int
		for _, c := range d.nodes[ul].Children {
			if d.nodes[c].Tag != "#comment" {
				got = append(got, c)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("step %d (perm %v): DOM order %v, want %v", step, perm, got, want)
		}
	}
}

// #61: an ErrorBoundary whose child panicked must release everything that
// mounted before the panic.
func TestErrorBoundaryReleasesPartialMount(t *testing.T) {
	g := core.NewSignal("x")
	mounted, cleaned := 0, 0
	sib := core.Component("Sib", func() core.Node {
		hooks.OnMount(func() func() { mounted++; return func() { cleaned++ } })
		return h.Span(h.TextS(g), h.Show(core.NewSignal(true), func() core.Node { return h.I() }))
	})
	bad := core.Component("Bad", func() core.Node { panic("boom") })
	show := core.NewSignal(true)
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		if show.Get() {
			return h.ErrorBoundary(func(any) core.Node { return h.P(h.Text("fallback")) }, h.Div(sib, bad))
		}
		return h.Span()
	}, show)))
	if mounted != cleaned {
		t.Fatalf("the abandoned sibling's effect must not stay live: mounted=%d cleaned=%d", mounted, cleaned)
	}
	if g.SubscriberCount() != 0 {
		t.Fatalf("the abandoned sibling's binding must be released, got %d subscribers", g.SubscriberCount())
	}
	if got := d.tree(0); !strings.Contains(got, "p(#text)") {
		t.Fatalf("fallback not rendered: %s", got)
	}
	show.Set(false)
	flush(r, d)
	if mounted != cleaned || cleaned > 1 {
		t.Fatalf("no double cleanup: mounted=%d cleaned=%d", mounted, cleaned)
	}
}

// #61: a panic while a boundary's child re-renders swaps to the fallback and
// releases the partial work (it used to leave a half-diffed subtree).
func TestErrorBoundaryUpdatePanicShowsFallback(t *testing.T) {
	mode := core.NewSignal(0)
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		m := mode.Get()
		return h.ErrorBoundary(func(any) core.Node { return h.P(h.Text("fallback")) },
			h.Section(core.Component(fmt.Sprintf("C%d", m), func() core.Node {
				if m == 1 {
					panic("update boom")
				}
				return h.B(h.Text("ok"))
			})))
	}, mode)))
	mode.Set(1)
	flush(r, d)
	if got, want := d.tree(0), "#root(div(p(#text)))"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	mode.Set(2)
	flush(r, d)
	if got, want := d.tree(0), "#root(div(section(b(#text))))"; got != want {
		t.Fatalf("a later successful render restores the child: got %s want %s", got, want)
	}
}

// A raw node is server-rendered inside a <div>, so hydration must claim a div.
func TestRawHydratesAsDiv(t *testing.T) {
	r := New()
	r.SetHydrating(true)
	muts, _ := r.Render(h.Section(h.Raw("<b>hi</b>")))
	for _, m := range muts {
		if m.Type == core.MutHydrate && m.NodeID == 2 && m.Value != "div" {
			t.Fatalf("raw node claimed as %v, want div", m.Value)
		}
	}
}

// #71: removing a subtree sends only its DOM roots; the JS runtime forgets the
// descendants itself.
func TestClearingAListRemovesOnlyRowRoots(t *testing.T) {
	xs := make([]int, 100)
	for i := range xs {
		xs[i] = i
	}
	items := core.NewSignal(xs)
	r, d := mount(t, h.Ul(h.For(items, func(i int) int { return i }, func(i int) core.Node {
		return h.Li(h.Span(h.Text("a")), h.Span(h.Text("b")))
	})))
	items.Set(nil)
	muts := r.Scheduler.Flush()
	removes := 0
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			removes++
		}
	}
	if removes != 100 {
		t.Fatalf("want one RemoveNode per row root (100), got %d", removes)
	}
	d.apply(muts)
	if len(d.nodes) != 3 { // #root, ul, the For's anchor
		t.Fatalf("the fake DOM should have forgotten every row node, %d left", len(d.nodes))
	}
}

// #71: portal content lives outside the removed subtree's DOM, so it is
// removed explicitly.
func TestRemovingASubtreeRemovesItsPortalContent(t *testing.T) {
	show := core.NewSignal(true)
	var modal *core.ElementNode
	r, _ := mount(t, h.Div(h.Show(show, func() core.Node {
		modal = h.Div(h.Text("modal"))
		return h.Section(h.Portal("#modal-root", modal))
	})))
	show.Set(false)
	var removed []int
	for _, m := range r.Scheduler.Flush() {
		if m.Type == core.MutRemoveNode {
			removed = append(removed, m.NodeID)
		}
	}
	found := false
	for _, id := range removed {
		found = found || id == modal.ID
	}
	if !found || len(removed) != 2 {
		t.Fatalf("want the section and the portal's root removed, got %v (modal=%d)", removed, modal.ID)
	}
}
