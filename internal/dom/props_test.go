package dom

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// texts collects the current textContent of every text node, sorted.
type textDOM struct {
	*fakeDOM
	text map[int]string
}

func newTextDOM() *textDOM { return &textDOM{fakeDOM: newFakeDOM(), text: map[int]string{}} }

func (d *textDOM) apply(muts []core.Mutation) {
	d.fakeDOM.apply(muts)
	for _, m := range muts {
		switch {
		case m.Type == core.MutSetProperty && m.Key == "textContent":
			d.text[m.NodeID] = fmt.Sprint(m.Value)
		case m.Type == core.MutRemoveNode:
			delete(d.text, m.NodeID)
		}
	}
}

func (d *textDOM) texts() string {
	var out []string
	for id, s := range d.text {
		if _, ok := d.nodes[id]; ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "|")
}

func mountText(t *testing.T, n core.Node) (*DOMRenderer, *textDOM) {
	t.Helper()
	r := New()
	d := newTextDOM()
	muts, _ := r.Render(n)
	d.apply(muts)
	r.Scheduler.RunEffects()
	return r, d
}

func flushText(r *DOMRenderer, d *textDOM) {
	d.apply(r.Scheduler.Flush())
	r.Scheduler.RunEffects()
}

// #57: a ComponentWithProps kept across a parent re-render receives the new
// props through its signal — new data, same state.
func TestComponentWithPropsUpdatesInPlace(t *testing.T) {
	sel := core.NewSignal("alice")
	setups := 0
	var bump func(int)
	card := func(name string) core.Node {
		return core.ComponentWithProps("Card", name, func(p *core.Signal[string]) core.Node {
			setups++
			clicks, setClicks := hooks.UseState(0)
			bump = setClicks
			return h.P(h.Textf("Hello %s (%d)", p, clicks))
		})
	}
	r, d := mountText(t, h.Div(hooks.UseScope(func() core.Node { return card(sel.Get()) }, sel)))
	bump(3)
	flushText(r, d)
	sel.Set("bob")
	flushText(r, d)
	if got := d.texts(); got != "Hello bob (3)" {
		t.Fatalf("want new props with kept state, got %q", got)
	}
	if setups != 1 {
		t.Fatalf("a props component must not remount, setups=%d", setups)
	}
}

// #57: a different key is a different component, even in a positional diff —
// the React key idiom for "remount when this value changes".
func TestKeyedComponentRemountsOnKeyChange(t *testing.T) {
	sel := core.NewSignal("alice")
	card := func(name string) core.Node {
		c := core.Component("Card", func() core.Node { return h.P(h.Text("Hello " + name)) })
		core.SetKey(c, name)
		return c
	}
	r, d := mountText(t, h.Div(hooks.UseScope(func() core.Node { return card(sel.Get()) }, sel)))
	sel.Set("bob")
	flushText(r, d)
	if got := d.texts(); got != "Hello bob" {
		t.Fatalf("keyed component must remount with new data, got %q", got)
	}
}

type todo struct {
	ID    int
	Title string
}

// #57: For with plain component rows used to keep showing an edited item's old
// data; the changed row is now remounted, the others are untouched.
func TestForPlainComponentRowShowsEditedItem(t *testing.T) {
	todos := core.NewSignal([]todo{{1, "buy milk"}, {2, "walk dog"}})
	setups := map[int]int{}
	r, d := mountText(t, h.Ul(h.For(todos, func(t todo) int { return t.ID }, func(t todo) core.Node {
		return core.Component("Row", func() core.Node {
			setups[t.ID]++
			return h.Li(h.Text(t.Title))
		})
	})))
	todos.Set([]todo{{1, "buy oat milk"}, {2, "walk dog"}})
	flushText(r, d)
	if got := d.texts(); got != "buy oat milk|walk dog" {
		t.Fatalf("edited row must show its new data, got %q", got)
	}
	if setups[1] != 2 || setups[2] != 1 {
		t.Fatalf("only the edited row remounts: setups=%v", setups)
	}
	if got, want := d.tree(0), "#root(ul(li(#text) li(#text)))"; got != want {
		t.Fatalf("remounted row must stay in place: got %s want %s", got, want)
	}
}

// #57: a ComponentWithProps row keeps its state and gets the edited item.
func TestForPropsRowKeepsStateAndGetsNewItem(t *testing.T) {
	todos := core.NewSignal([]todo{{1, "buy milk"}})
	setups := 0
	var toggle func(bool)
	r, d := mountText(t, h.Ul(h.For(todos, func(t todo) int { return t.ID }, func(t todo) core.Node {
		return core.ComponentWithProps("Row", t, func(p *core.Signal[todo]) core.Node {
			setups++
			open, setOpen := hooks.UseState(false)
			toggle = setOpen
			title := core.Computed([]core.SignalAccessor{p, open}, func() string {
				return fmt.Sprintf("%s open=%v", p.Get().Title, open.Get())
			})
			return h.Li(h.TextS(title))
		})
	})))
	toggle(true)
	flushText(r, d)
	todos.Set([]todo{{1, "buy oat milk"}})
	flushText(r, d)
	if got := d.texts(); got != "buy oat milk open=true" || setups != 1 {
		t.Fatalf("want new item with kept state and no remount, got %q setups=%d", got, setups)
	}
}

// #70: For renders only new or changed items; unchanged rows are reused as-is.
func TestForRendersOnlyChangedRows(t *testing.T) {
	items := core.NewSignal([]todo{{1, "a"}, {2, "b"}, {3, "c"}})
	renders := 0
	r, d := mountText(t, h.Ul(h.For(items, func(t todo) int { return t.ID }, func(t todo) core.Node {
		renders++
		return h.Li(h.Text(t.Title))
	})))
	renders = 0
	items.Set([]todo{{1, "a"}, {2, "b"}, {3, "c"}, {4, "d"}})
	flushText(r, d)
	if renders != 1 {
		t.Fatalf("append should render one row, rendered %d", renders)
	}
	renders = 0
	items.Set([]todo{{4, "d"}, {3, "c"}, {2, "B"}, {1, "a"}})
	flushText(r, d)
	if renders != 1 {
		t.Fatalf("reorder + one edit should render one row, rendered %d", renders)
	}
	if got := d.texts(); got != "B|a|c|d" {
		t.Fatalf("got %q", got)
	}
}

// #57: in dev mode, a plain unkeyed component kept across a parent re-render
// is flagged once.
func TestPreservedPlainComponentWarnsOnceInDevMode(t *testing.T) {
	prevDev := devMode
	devMode = func() bool { return true }
	defer func() { devMode = prevDev }()
	delete(warnedPreserved, "StaleCard")

	var warns []core.LogEntry
	core.SetLogSink(func(e core.LogEntry) {
		if e.Kind == core.LogWarn {
			warns = append(warns, e)
		}
	})
	defer core.SetLogSink(nil)

	sel := core.NewSignal(0)
	r, d := mountText(t, h.Div(hooks.UseScope(func() core.Node {
		v := sel.Get()
		return core.Component("StaleCard", func() core.Node { return h.P(h.Text(fmt.Sprint(v))) })
	}, sel)))
	sel.Set(1)
	flushText(r, d)
	sel.Set(2)
	flushText(r, d)
	if len(warns) != 1 || warns[0].Fields["component"] != "StaleCard" {
		t.Fatalf("want one warning naming StaleCard, got %+v", warns)
	}
}

// #70: the O(n log n) LIS picks a maximum-length strictly increasing
// subsequence (checked against the O(n²) DP on random inputs).
func TestLISIsMaximalAndIncreasing(t *testing.T) {
	dpLen := func(a []int) int {
		best := 0
		dp := make([]int, len(a))
		for i := range a {
			dp[i] = 1
			for j := 0; j < i; j++ {
				if a[j] < a[i] && dp[j]+1 > dp[i] {
					dp[i] = dp[j] + 1
				}
			}
			if dp[i] > best {
				best = dp[i]
			}
		}
		return best
	}
	seed := uint64(1)
	rnd := func(n int) int { seed = seed*6364136223846793005 + 1442695040888963407; return int(seed>>33) % n }
	for trial := 0; trial < 500; trial++ {
		n := rnd(40)
		perm := make([]int, n)
		for i := range perm {
			perm[i] = i
		}
		for i := n - 1; i > 0; i-- {
			j := rnd(i + 1)
			perm[i], perm[j] = perm[j], perm[i]
		}
		in := lis(perm)
		last, count := -1, 0
		for i, ok := range in {
			if ok {
				if perm[i] <= last {
					t.Fatalf("trial %d: not increasing: %v %v", trial, perm, in)
				}
				last = perm[i]
				count++
			}
		}
		if count != dpLen(perm) {
			t.Fatalf("trial %d: length %d, want %d for %v", trial, count, dpLen(perm), perm)
		}
	}
}

// ShowResource shows exactly one of loading / error / data.
func TestShowResourceShowsOneStateAtATime(t *testing.T) {
	res := &hooks.Resource[string]{
		Data:    core.NewSignal(""),
		Loading: core.NewSignal(true),
		Err:     core.NewSignal[error](nil),
	}
	r, d := mountText(t, h.Div(h.ShowResource(res,
		func() core.Node { return h.P(h.Text("loading")) },
		func(err error) core.Node { return h.P(h.Text("error: " + err.Error())) },
		func(data *core.Signal[string]) core.Node { return h.P(h.TextS(data)) },
	)))
	if got := d.texts(); got != "loading" {
		t.Fatalf("while loading want only the loading view, got %q", got)
	}
	core.Batch(func() { res.Data.Set("hi"); res.Loading.Set(false) })
	flushText(r, d)
	if got := d.texts(); got != "hi" {
		t.Fatalf("want only the data view, got %q", got)
	}
	core.Batch(func() { res.Err.Set(fmt.Errorf("boom")) })
	flushText(r, d)
	if got := d.texts(); got != "error: boom" {
		t.Fatalf("want only the error view, got %q", got)
	}
}
