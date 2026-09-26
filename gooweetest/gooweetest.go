// Package gooweetest renders goowee components headlessly for unit tests —
// net/http/httptest for goowee. A Screen mounts a tree into an in-memory DOM
// that applies the renderer's real mutation stream (the same one the browser
// runtime applies), dispatches events through the real handler registry with
// DOM bubbling, runs effects after each frame, and wires core.Schedule, so
// signals, scopes, lists, effects, refs and async resources all behave as in
// the browser:
//
//	func TestCounter(t *testing.T) {
//	    s := gooweetest.Render(t, Counter())
//	    s.Click(s.FindByText("Increment"))
//	    if got := s.Find("p.count").Text(); got != "Count: 1" {
//	        t.Fatalf("got %q", got)
//	    }
//	}
//
// Every interaction flushes, so assertions see the updated DOM. For work that
// finishes on another goroutine (hooks.UseResource), use WaitFor/WaitForText.
// Layout doesn't exist here: ref reads of sizes and positions return nil.
package gooweetest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/dom"
)

// Screen is a mounted tree. Its methods fail the test (t.Fatal) when a
// lookup finds nothing.
type Screen struct {
	t   testing.TB
	r   *dom.DOMRenderer
	doc *document
}

// Render mounts n and flushes until the first frame and its effects are done.
// The mount is torn down (effects cleaned up) when the test ends.
func Render(t testing.TB, n core.Node) *Screen {
	t.Helper()
	s := &Screen{t: t, r: dom.New(), doc: newDocument()}
	core.SetActiveScheduler(s.r.Scheduler)
	muts, _ := s.r.Render(n)
	s.r.Scheduler.Enqueue(muts...)
	s.Flush()
	t.Cleanup(func() {
		s.Unmount()
		core.SetActiveScheduler(nil)
	})
	return s
}

// Flush runs frames — callbacks posted by goroutines, dirty re-renders, the
// resulting DOM mutations, then effects — until there is no work left.
func (s *Screen) Flush() {
	s.t.Helper()
	sched := s.r.Scheduler
	for i := 0; i < 1000; i++ {
		muts := sched.Flush()
		s.doc.apply(muts)
		replies := s.doc.reads
		s.doc.reads = nil
		for _, rp := range replies {
			sched.Resolve(rp.req, rp.value)
		}
		sched.RunEffects()
		if !sched.HasWork() {
			return
		}
	}
	s.t.Fatalf("gooweetest: rendering did not settle after 1000 frames (an update loop?)")
}

// WaitFor flushes until cond holds, for results that arrive from goroutines
// (hooks.UseResource, core.Schedule). It fails the test after timeout
// (default 2s).
func (s *Screen) WaitFor(cond func() bool, timeout ...time.Duration) {
	s.t.Helper()
	limit := 2 * time.Second
	if len(timeout) > 0 {
		limit = timeout[0]
	}
	deadline := time.Now().Add(limit)
	for {
		s.Flush()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("gooweetest: condition not met within %v; page text: %q", limit, s.Text())
		}
		time.Sleep(time.Millisecond)
	}
}

// WaitForText flushes until the page's text contains text.
func (s *Screen) WaitForText(text string, timeout ...time.Duration) {
	s.t.Helper()
	s.WaitFor(func() bool { return strings.Contains(s.Text(), text) }, timeout...)
}

// Unmount removes the tree, running every cleanup (effects, subscriptions).
// Render arranges it for the end of the test; call it earlier to test
// teardown.
func (s *Screen) Unmount() {
	if s.doc == nil {
		return
	}
	s.r.Unmount()
	s.Flush()
	s.doc.root.children = nil
}

// HTML is the rendered markup of the mount point's content (attributes sorted,
// scope anchors omitted).
func (s *Screen) HTML() string {
	var b strings.Builder
	for _, c := range s.doc.root.children {
		outerHTML(c, &b)
	}
	return b.String()
}

// Text is the text content of everything rendered (portal and head content
// excluded).
func (s *Screen) Text() string { return s.doc.root.textContent() }

// Head is the markup rendered into <head> by h.Metadata.
func (s *Screen) Head() string {
	var b strings.Builder
	for _, c := range s.doc.head.children {
		outerHTML(c, &b)
	}
	return b.String()
}

// Portal is the content rendered into a portal target (the selector passed to
// h.Portal) — nil if nothing was.
func (s *Screen) Portal(selector string) *Element {
	c := s.doc.container(selector)
	if len(c.children) == 0 {
		return nil
	}
	return &Element{s: s, n: c}
}

// Focused is the element that has focus (via ref.Focus or Focus), or nil.
func (s *Screen) Focused() *Element {
	if s.doc.active == nil {
		return nil
	}
	return &Element{s: s, n: s.doc.active}
}

// Find returns the first element matching selector (see package doc for the
// supported CSS subset), failing the test if there is none.
func (s *Screen) Find(selector string) *Element {
	s.t.Helper()
	if el := s.Query(selector); el != nil {
		return el
	}
	s.t.Fatalf("gooweetest: no element matches %q in:\n%s", selector, s.HTML())
	return nil
}

// Query is Find without failing: nil when nothing matches.
func (s *Screen) Query(selector string) *Element {
	s.t.Helper()
	all := s.FindAll(selector)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

// FindAll returns every element matching selector, in document order.
func (s *Screen) FindAll(selector string) []*Element {
	s.t.Helper()
	sel, err := parseSelector(selector)
	if err != nil {
		s.t.Fatalf("gooweetest: %v", err)
	}
	var out []*Element
	for _, root := range s.searchRoots() {
		for _, n := range selectAll(root, sel) {
			out = append(out, &Element{s: s, n: n})
		}
	}
	return out
}

func (s *Screen) searchRoots() []*node {
	roots := []*node{s.doc.root}
	for _, c := range s.doc.portal {
		roots = append(roots, c)
	}
	return roots
}

// FindByText returns the innermost element whose trimmed text is exactly
// text, failing the test if there is none.
func (s *Screen) FindByText(text string) *Element {
	s.t.Helper()
	var found *node
	var walk func(n *node) bool
	walk = func(n *node) bool {
		if n.kind != elementNode {
			return false
		}
		for _, c := range n.children {
			if walk(c) {
				return true
			}
		}
		if strings.TrimSpace(n.textContent()) == text {
			found = n
			return true
		}
		return false
	}
	for _, root := range s.searchRoots() {
		for _, c := range root.children {
			if walk(c) {
				return &Element{s: s, n: found}
			}
		}
	}
	s.t.Fatalf("gooweetest: no element with text %q; page text: %q", text, s.Text())
	return nil
}

// Dispatch fires event at el with the given payload (what the browser runtime
// would send: "value", "key", "clientX", …), bubbling through ancestors like
// the DOM (non-bubbling events reach el only), then flushes. It reports
// whether a handler prevented the default action.
func (s *Screen) Dispatch(el *Element, event string, data map[string]any) (prevented bool) {
	s.t.Helper()
	if data == nil {
		data = map[string]any{}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		s.t.Fatalf("gooweetest: event data: %v", err)
	}
	targetOnly := dom.EventCapture(event)
	for n := el.n; n != nil; n = n.parent {
		if n.id == 0 {
			if targetOnly {
				break
			}
			continue
		}
		opts, handled := s.r.Registry.Dispatch(n.id, event, string(payload))
		if handled {
			prevented = prevented || opts.PreventDefault
			if opts.StopPropagation {
				break
			}
		}
		if targetOnly {
			break
		}
	}
	s.Flush()
	return prevented
}

// Click clicks el (primary button, no modifiers).
func (s *Screen) Click(el *Element) {
	s.t.Helper()
	s.Dispatch(el, "click", map[string]any{"button": 0})
}

// Input sets el's value and fires input, as typing does.
func (s *Screen) Input(el *Element, value string) {
	s.t.Helper()
	el.n.props["value"] = value
	s.Dispatch(el, "input", map[string]any{"value": value})
}

// Change sets el's value and fires change (a committed edit, a <select>).
func (s *Screen) Change(el *Element, value string) {
	s.t.Helper()
	el.n.props["value"] = value
	s.Dispatch(el, "change", map[string]any{"value": value})
}

// Check sets a checkbox/radio's checked state and fires input and change.
func (s *Screen) Check(el *Element, checked bool) {
	s.t.Helper()
	el.n.props["checked"] = checked
	value, _ := el.n.attr("value")
	data := map[string]any{"value": value, "checked": checked}
	s.Dispatch(el, "input", data)
	s.Dispatch(el, "change", data)
}

// Submit fires submit on a <form>, carrying the values of its named fields
// (like the browser runtime's payload), and reports whether it was prevented.
func (s *Screen) Submit(form *Element) (prevented bool) {
	s.t.Helper()
	values := map[string]any{}
	var walk func(n *node)
	walk = func(n *node) {
		if name, ok := n.attr("name"); ok && name != "" && (n.tag == "input" || n.tag == "textarea" || n.tag == "select") {
			typ, _ := n.attr("type")
			if typ == "checkbox" || typ == "radio" {
				if c, _ := n.props["checked"].(bool); c {
					values[name] = "on"
				} else {
					values[name] = "off"
				}
			} else {
				values[name] = Element{n: n}.Value()
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(form.n)
	return s.Dispatch(form, "submit", map[string]any{"values": values})
}

// KeyDown fires keydown (then keyup) for key on el and reports whether the
// keydown was prevented.
func (s *Screen) KeyDown(el *Element, key string) (prevented bool) {
	s.t.Helper()
	prevented = s.Dispatch(el, "keydown", map[string]any{"key": key})
	s.Dispatch(el, "keyup", map[string]any{"key": key})
	return prevented
}

// Focus focuses el and fires focus.
func (s *Screen) Focus(el *Element) {
	s.t.Helper()
	s.doc.active = el.n
	s.Dispatch(el, "focus", nil)
}

// Element is a rendered element.
type Element struct {
	s *Screen
	n *node
}

// Tag is the element's tag name (lowercase).
func (e *Element) Tag() string { return e.n.tag }

// Attr is an attribute's value ("" if absent).
func (e *Element) Attr(name string) string { v, _ := e.n.attr(name); return v }

// HasAttr reports whether the attribute is present.
func (e *Element) HasAttr(name string) bool { _, ok := e.n.attr(name); return ok }

// Prop is a DOM property set by the renderer (value, checked, disabled, …).
func (e *Element) Prop(name string) any { return e.n.props[name] }

// Value is the element's value property (or value attribute).
func (e Element) Value() string {
	if v, ok := e.n.props["value"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	v, _ := e.n.attr("value")
	return v
}

// Checked is the checked property of a checkbox/radio.
func (e *Element) Checked() bool { c, _ := e.n.props["checked"].(bool); return c }

// Text is the element's text content.
func (e *Element) Text() string { return e.n.textContent() }

// HTML is the element's outer markup.
func (e *Element) HTML() string {
	var b strings.Builder
	outerHTML(e.n, &b)
	return b.String()
}

// Children are the element's child elements.
func (e *Element) Children() []*Element {
	var out []*Element
	for _, c := range e.n.children {
		if c.kind == elementNode {
			out = append(out, &Element{s: e.s, n: c})
		}
	}
	return out
}

// Find returns the first descendant matching selector, failing if none.
func (e *Element) Find(selector string) *Element {
	e.s.t.Helper()
	sel, err := parseSelector(selector)
	if err != nil {
		e.s.t.Fatalf("gooweetest: %v", err)
	}
	if all := selectAll(e.n, sel); len(all) > 0 {
		return &Element{s: e.s, n: all[0]}
	}
	e.s.t.Fatalf("gooweetest: no element matches %q in:\n%s", selector, e.HTML())
	return nil
}
