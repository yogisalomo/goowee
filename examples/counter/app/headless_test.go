package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/dom"
	"github.com/yogisalomo/goowee/router"
)

func testRouter() *router.Router {
	return router.New("/")
}

// ---------------------------------------------------------------------------
// Fake DOM — mirrors runtime/goowee.js applyMutations so these tests exercise
// the same render/diff/event/scheduler pipeline the browser runs, headlessly.
// ---------------------------------------------------------------------------

type fnode struct {
	id       int
	tag      string
	parent   int // -1 == detached (parentNode === null)
	children []int
	attrs    map[string]string
	props    map[string]any
}

type fakeDOM struct {
	nodes map[int]*fnode
}

func newFakeDOM() *fakeDOM {
	return &fakeDOM{nodes: map[int]*fnode{
		0: {id: 0, tag: "#root", parent: -1, attrs: map[string]string{}, props: map[string]any{}},
	}}
}

func (d *fakeDOM) detach(id int) {
	n := d.nodes[id]
	if n == nil || n.parent == -1 {
		return
	}
	p := d.nodes[n.parent]
	if p != nil {
		for i, c := range p.children {
			if c == id {
				p.children = append(p.children[:i], p.children[i+1:]...)
				break
			}
		}
	}
	n.parent = -1
}

func (d *fakeDOM) apply(muts []core.Mutation) {
	for _, m := range muts {
		switch m.Type {
		case core.MutCreateElement:
			d.nodes[m.NodeID] = &fnode{
				id: m.NodeID, tag: m.Value.(string), parent: -1,
				attrs: map[string]string{}, props: map[string]any{},
			}
		case core.MutHydrate:
			// Claim a server-rendered node: keep it if present, else create
			// (the JS-side fallback for a hydration miss).
			if _, ok := d.nodes[m.NodeID]; !ok {
				d.nodes[m.NodeID] = &fnode{
					id: m.NodeID, tag: m.Value.(string), parent: -1,
					attrs: map[string]string{}, props: map[string]any{},
				}
			}
		case core.MutRemoveNode:
			d.detach(m.NodeID)
			delete(d.nodes, m.NodeID)
		case core.MutSetAttribute:
			if n := d.nodes[m.NodeID]; n != nil {
				n.attrs[m.Key] = toStr(m.Value)
			}
		case core.MutRemoveAttribute:
			if n := d.nodes[m.NodeID]; n != nil {
				delete(n.attrs, m.Key)
			}
		case core.MutSetProperty:
			if n := d.nodes[m.NodeID]; n != nil {
				n.props[m.Key] = m.Value
			}
		case core.MutAppendChild:
			p, c := d.nodes[m.NodeID], d.nodes[m.ChildID]
			if p != nil && c != nil && c.parent == -1 { // mirror !child.parentNode
				p.children = append(p.children, c.id)
				c.parent = p.id
			}
		case core.MutInsertBefore:
			p, c := d.nodes[m.NodeID], d.nodes[m.ChildID]
			if p == nil || c == nil {
				break
			}
			d.detach(c.id)
			idx := len(p.children)
			if m.RefID != 0 {
				for i, cid := range p.children {
					if cid == m.RefID {
						idx = i
						break
					}
				}
			}
			p.children = append(p.children, 0)
			copy(p.children[idx+1:], p.children[idx:])
			p.children[idx] = c.id
			c.parent = p.id
		}
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// text returns the concatenated text content of a node's subtree.
func (d *fakeDOM) text(id int) string {
	n := d.nodes[id]
	if n == nil {
		return ""
	}
	if n.tag == "#text" {
		if s, ok := n.props["textContent"].(string); ok {
			return s
		}
		return ""
	}
	var b strings.Builder
	for _, c := range n.children {
		b.WriteString(d.text(c))
	}
	return b.String()
}

// find returns the id of the first node (DFS from root) matching pred.
func (d *fakeDOM) find(pred func(*fnode) bool) int {
	return d.findFrom(0, pred)
}

func (d *fakeDOM) findFrom(id int, pred func(*fnode) bool) int {
	n := d.nodes[id]
	if n == nil {
		return 0
	}
	if id != 0 && pred(n) {
		return id
	}
	for _, c := range n.children {
		if got := d.findFrom(c, pred); got != 0 {
			return got
		}
	}
	return 0
}

func (d *fakeDOM) findAll(pred func(*fnode) bool) []int {
	var out []int
	var walk func(int)
	walk = func(id int) {
		n := d.nodes[id]
		if n == nil {
			return
		}
		if id != 0 && pred(n) {
			out = append(out, id)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(0)
	return out
}

// buttonWithText finds a <button> whose subtree text contains substr.
func (d *fakeDOM) buttonWithText(substr string) int {
	for _, id := range d.findAll(func(n *fnode) bool { return n.tag == "button" }) {
		if strings.Contains(d.text(id), substr) {
			return id
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Harness — wires a DOMRenderer to a fake DOM and simulates events.
// ---------------------------------------------------------------------------

type harness struct {
	t   *testing.T
	r   *dom.DOMRenderer
	dom *fakeDOM
}

func mount(t *testing.T, n core.Node) *harness {
	t.Helper()
	r := dom.New()
	muts, _ := r.Render(n)
	fd := newFakeDOM()
	fd.apply(muts)
	return &harness{t: t, r: r, dom: fd}
}

// flush drains the scheduler (as the rAF loop does) into the fake DOM.
func (h *harness) flush() {
	h.dom.apply(h.r.Scheduler.Flush())
}

func (h *harness) dispatch(id int, event string, data map[string]any) {
	h.t.Helper()
	if id == 0 {
		h.t.Fatalf("dispatch: node not found for event %q", event)
	}
	payload, _ := json.Marshal(data)
	h.r.Registry.Dispatch(id, event, string(payload))
	h.flush()
}

func (h *harness) click(id int)            { h.dispatch(id, "click", map[string]any{}) }
func (h *harness) input(id int, v string)  { h.dispatch(id, "input", map[string]any{"value": v}) }
func (h *harness) change(id int, v string) { h.dispatch(id, "change", map[string]any{"value": v}) }
func (h *harness) check(id int, checked bool) {
	h.dispatch(id, "input", map[string]any{"checked": checked})
}
func (h *harness) submit(id int, vals map[string]any) {
	h.dispatch(id, "submit", map[string]any{"values": vals})
}

// ---------------------------------------------------------------------------
// Page tests
// ---------------------------------------------------------------------------

func TestCounterPageHeadless(t *testing.T) {
	h := mount(t, counterPage(testRouter()))

	incr := h.dom.buttonWithText("Count: 0")
	if incr == 0 {
		t.Fatalf("increment button not found; tree text = %q", h.dom.text(0))
	}
	h.click(incr)
	if got := h.dom.text(incr); !strings.Contains(got, "Count: 1") {
		t.Fatalf("after 1 click want Count: 1, got %q", got)
	}
	h.click(incr)
	h.click(incr)
	if got := h.dom.text(incr); !strings.Contains(got, "Count: 3") {
		t.Fatalf("after 3 clicks want Count: 3, got %q", got)
	}

	// Greeting shown initially (show == true).
	greeting := h.dom.find(func(n *fnode) bool { return n.attrs["class"] == "greeting" })
	if greeting == 0 || !strings.Contains(h.dom.text(greeting), "Hello!") {
		t.Fatalf("expected greeting shown initially, tree = %q", h.dom.text(0))
	}

	// Toggle hides greeting and shows the hidden branch (ShowElse).
	toggle := h.dom.buttonWithText("Toggle")
	h.click(toggle)
	if id := h.dom.find(func(n *fnode) bool { return n.attrs["class"] == "greeting" }); id != 0 {
		t.Fatalf("greeting should be hidden after toggle")
	}
	if id := h.dom.find(func(n *fnode) bool { return n.attrs["class"] == "hidden" }); id == 0 {
		t.Fatalf("expected hidden branch after toggle, tree = %q", h.dom.text(0))
	}

	// Toggle again brings the greeting back (hide -> show reattachment).
	h.click(toggle)
	if id := h.dom.find(func(n *fnode) bool { return n.attrs["class"] == "greeting" }); id == 0 {
		t.Fatalf("greeting should reappear after second toggle, tree = %q", h.dom.text(0))
	}
}

// findAllText returns nodes of tag whose text contains substr — used to target
// demo content and ignore lesson chrome (code panel, "how it works" list).
func (d *fakeDOM) findAllText(tag, substr string) []int {
	out := []int{}
	for _, id := range d.findAll(func(n *fnode) bool { return n.tag == tag }) {
		if strings.Contains(d.text(id), substr) {
			out = append(out, id)
		}
	}
	return out
}

// findAllInLi returns nodes of tag whose direct parent is an <li> — e.g. the
// todo-row spans, excluding decorative spans in the card/code-panel bars.
func (d *fakeDOM) findAllInLi(tag string) []int {
	out := []int{}
	for _, id := range d.findAll(func(n *fnode) bool { return n.tag == tag }) {
		if p, ok := d.nodes[d.nodes[id].parent]; ok && p.tag == "li" {
			out = append(out, id)
		}
	}
	return out
}

func TestFormPageHeadless(t *testing.T) {
	h := mount(t, formPage(testRouter()))

	// Two-way binding: typing updates the bound signal, which the preview reflects.
	nameInput := h.dom.find(func(n *fnode) bool { return n.tag == "input" && n.attrs["name"] == "name" })
	emailInput := h.dom.find(func(n *fnode) bool { return n.tag == "input" && n.attrs["name"] == "email" })
	if nameInput == 0 || emailInput == 0 {
		t.Fatal("name/email inputs not found")
	}
	h.input(nameInput, "Ada")
	h.input(emailInput, "ada@x.io")
	if !strings.Contains(h.dom.text(0), "Name: Ada, Email: ada@x.io") {
		t.Fatalf("preview did not reflect inputs; tree = %q", h.dom.text(0))
	}
	// The input's value property is bound and reflects the signal.
	if got := h.dom.nodes[nameInput].props["value"]; got != "Ada" {
		t.Fatalf("name input value prop = %v, want Ada", got)
	}

	// Submit adds an entry to the list.
	form := h.dom.find(func(n *fnode) bool { return n.tag == "form" })
	h.submit(form, map[string]any{"name": "Ada", "email": "ada@x.io", "agreed": "on"})
	lis := h.dom.findAllText("li", "subscribed:")
	if len(lis) != 1 {
		t.Fatalf("want 1 submission li, got %d", len(lis))
	}
	text := h.dom.text(lis[0])
	if !strings.Contains(text, "Ada") || !strings.Contains(text, "ada@x.io") || !strings.Contains(text, "subscribed: yes") {
		t.Fatalf("submission text wrong: %q", text)
	}
	// Inputs cleared after submit.
	if got := h.dom.nodes[nameInput].props["value"]; got != "" {
		t.Fatalf("name input not cleared after submit: %v", got)
	}

	// A second, distinct submission appends a keyed row (unique keys).
	h.submit(form, map[string]any{"name": "Bob", "email": "bob@x.io", "agreed": "off"})
	lis = h.dom.findAllText("li", "subscribed:")
	if len(lis) != 2 {
		t.Fatalf("want 2 submission lis, got %d", len(lis))
	}
	text = h.dom.text(lis[1])
	if !strings.Contains(text, "Bob") || !strings.Contains(text, "bob@x.io") || !strings.Contains(text, "subscribed: no") {
		t.Fatalf("second submission text wrong: %q", text)
	}
}

func TestTodosPageHeadless(t *testing.T) {
	h := mount(t, todosPage(testRouter()))

	// doneCount starts at 0/100.
	if !strings.Contains(h.dom.text(0), "0/100 completed") {
		t.Fatalf("want 0/100 completed initially, tree = %q", h.dom.text(0))
	}

	// VirtualList renders a window, not all 100 items.
	spans := h.dom.findAllInLi("span")
	if len(spans) == 0 {
		t.Fatal("expected some visible todo items")
	}
	if len(spans) >= 100 {
		t.Fatalf("expected virtualization, got %d visible items", len(spans))
	}
	// First visible item is "Item 1".
	if got := h.dom.text(spans[0]); got != "Item 1" {
		t.Fatalf("first visible item = %q, want Item 1", got)
	}

	// Toggle the first todo's checkbox -> doneCount updates to 1/100.
	firstCheckbox := h.dom.find(func(n *fnode) bool { return n.tag == "input" && n.attrs["type"] == "checkbox" })
	h.change(firstCheckbox, "")
	if !strings.Contains(h.dom.text(0), "1/100 completed") {
		t.Fatalf("want 1/100 completed after toggle, tree = %q", h.dom.text(0))
	}

	// Add a todo via the form -> it prepends and count becomes /101.
	form := h.dom.find(func(n *fnode) bool { return n.tag == "form" })
	h.submit(form, map[string]any{"todo": "Brand new"})
	if !strings.Contains(h.dom.text(0), "completed") || !strings.Contains(h.dom.text(0), "/101") {
		t.Fatalf("want /101 after add, tree = %q", h.dom.text(0))
	}
	spans = h.dom.findAllInLi("span")
	if got := h.dom.text(spans[0]); got != "Brand new" {
		t.Fatalf("new todo should be first; got %q", got)
	}

	// Remove the first (new) todo via its ✕ button -> back to /100.
	removeBtn := h.dom.buttonWithText("✕")
	h.click(removeBtn)
	if !strings.Contains(h.dom.text(0), "/100") {
		t.Fatalf("want /100 after remove, tree = %q", h.dom.text(0))
	}
	spans = h.dom.findAllInLi("span")
	if got := h.dom.text(spans[0]); got != "Item 1" {
		t.Fatalf("after removing new todo, first should be Item 1; got %q", got)
	}
}

func TestDashboardPageHeadless(t *testing.T) {
	h := mount(t, dashboardPage(testRouter()))

	if !strings.Contains(h.dom.text(0), "Total: 215") { // 42+17+88+5+63
		t.Fatalf("want Total: 215, tree = %q", h.dom.text(0))
	}
	if !strings.Contains(h.dom.text(0), "No row selected.") {
		t.Fatalf("want no-selection message, tree = %q", h.dom.text(0))
	}

	// Click the first data row -> selection message updates.
	rows := h.dom.findAll(func(n *fnode) bool { return n.tag == "tr" })
	// rows[0] is the header row (in thead); rows[1] is the first data row.
	if len(rows) < 2 {
		t.Fatalf("want header + data rows, got %d", len(rows))
	}
	h.click(rows[1])
	if !strings.Contains(h.dom.text(0), "Selected: Alpha = 42km") {
		t.Fatalf("want Alpha selected, tree = %q", h.dom.text(0))
	}

	// Randomize -> total recomputes to a new value.
	randomize := h.dom.buttonWithText("Randomize Values")
	h.click(randomize)
	if strings.Contains(h.dom.text(0), "Total: 215") {
		t.Fatalf("total should change after randomize, tree = %q", h.dom.text(0))
	}
}

// OnMount must run once on mount and its cleanup must run on unmount, so
// resources (the stopwatch's ticker goroutine) are released instead of
// leaking on every remount.
func TestOnMountCleanupHeadless(t *testing.T) {
	var mounted, cleaned int
	show := core.NewSignal(true)
	scope := hooks.UseScope(func() core.Node {
		if show.Get() {
			return core.Component("Mountee", func() core.Node {
				hooks.OnMount(func() func() {
					mounted++
					return func() { cleaned++ }
				})
				return &core.ElementNode{Tag: "div", Children: []core.Node{&core.TextNode{Value: "hi"}}}
			})
		}
		return &core.FragmentNode{}
	}, show)

	h := mount(t, &core.ElementNode{Tag: "main", Children: []core.Node{scope}})
	if mounted != 1 || cleaned != 0 {
		t.Fatalf("after mount want mounted=1 cleaned=0, got %d/%d", mounted, cleaned)
	}

	show.Set(false) // unmount
	h.flush()
	if mounted != 1 || cleaned != 1 {
		t.Fatalf("after unmount want mounted=1 cleaned=1, got %d/%d", mounted, cleaned)
	}

	show.Set(true) // remount: fresh mount, previous cleanup already ran
	h.flush()
	if mounted != 2 || cleaned != 1 {
		t.Fatalf("after remount want mounted=2 cleaned=1, got %d/%d", mounted, cleaned)
	}
}

// The stopwatch renders its initial state, and mounting/unmounting it drives
// the OnMount ticker's setup and teardown. We deliberately do not press Start:
// the ticker goroutine's concurrent signal writes are only safe on WASM's
// single thread, so exercising them under the native race detector would be
// unrepresentative. Mount/unmount here only reads `running` (false), so the
// goroutine never writes and the test stays race-free.
func TestStopwatchPageHeadless(t *testing.T) {
	show := core.NewSignal(true)
	scope := hooks.UseScope(func() core.Node {
		if show.Get() {
			return stopwatchPage(testRouter())
		}
		return &core.FragmentNode{}
	}, show)

	h := mount(t, &core.ElementNode{Tag: "main", Children: []core.Node{scope}})

	if !strings.Contains(h.dom.text(0), "off-loop timers") {
		t.Fatalf("stopwatch lesson heading missing; tree = %q", h.dom.text(0))
	}
	disp := h.dom.find(func(n *fnode) bool {
		s, ok := n.props["textContent"].(string)
		return ok && s == "00:00.0"
	})
	if disp == 0 {
		t.Fatalf("expected initial display 00:00.0; tree = %q", h.dom.text(0))
	}
	if h.dom.buttonWithText("Start") == 0 || h.dom.buttonWithText("Reset") == 0 {
		t.Fatalf("expected Start and Reset buttons; tree = %q", h.dom.text(0))
	}

	// Unmount: the OnMount cleanup runs (closes the ticker's stop channel).
	show.Set(false)
	h.flush()
	if h.dom.buttonWithText("Reset") != 0 {
		t.Fatalf("stopwatch subtree should be gone after unmount")
	}
}

// navLink finds the <a> whose text equals label.
func (d *fakeDOM) navLink(label string) int {
	for _, id := range d.findAll(func(n *fnode) bool { return n.tag == "a" }) {
		if d.text(id) == label {
			return id
		}
	}
	return 0
}

// navLinkContaining finds the <a> whose text contains sub (for composite link
// text like the tutorial steps).
func (d *fakeDOM) navLinkContaining(sub string) int {
	for _, id := range d.findAll(func(n *fnode) bool { return n.tag == "a" }) {
		if strings.Contains(d.text(id), sub) {
			return id
		}
	}
	return 0
}

func TestAppNavigationHeadless(t *testing.T) {
	r := router.New("/")
	h := mount(t, App(r))

	// Landing page renders, and its hero demo is a live goowee component.
	if !strings.Contains(h.dom.text(0), "Reactive web UIs, written in Go.") {
		t.Fatalf("expected landing page initially; tree = %q", h.dom.text(0))
	}
	incr := h.dom.buttonWithText("increment")
	if incr == 0 {
		t.Fatalf("expected live hero demo; tree = %q", h.dom.text(0))
	}

	// Header link → tutorial index → an example page.
	h.click(h.dom.navLink("Tutorial"))
	if !strings.Contains(h.dom.text(0), "Learn goowee by example") {
		t.Fatalf("expected tutorial index after nav; tree = %q", h.dom.text(0))
	}
	h.click(h.dom.navLinkContaining("Counter"))
	if h.dom.buttonWithText("Count: 0") == 0 {
		t.Fatalf("expected counter page from tutorial; tree = %q", h.dom.text(0))
	}

	// Back to the landing via the brand link.
	h.click(h.dom.navLink("goowee"))
	if !strings.Contains(h.dom.text(0), "Reactive web UIs, written in Go.") {
		t.Fatalf("expected to return to landing; tree = %q", h.dom.text(0))
	}
}

func TestStaticPagesHeadless(t *testing.T) {
	h := mount(t, homePage())
	if !strings.Contains(h.dom.text(0), "Welcome to Goowee") {
		t.Fatalf("home page text = %q", h.dom.text(0))
	}
	h2 := mount(t, aboutPage())
	if !strings.Contains(h2.dom.text(0), "minimal Go WASM") {
		t.Fatalf("about page text = %q", h2.dom.text(0))
	}
}
