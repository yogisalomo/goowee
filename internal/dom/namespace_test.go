package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// createdNS maps each element CreateElement in muts to the namespace it was
// created with, by tag (the last one wins).
func createdNS(muts []core.Mutation) map[string]string {
	ns := map[string]string{}
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			if tag, _ := m.Value.(string); tag != "#text" && tag != "#comment" {
				ns[tag] = m.NS
			}
		}
	}
	return ns
}

// A For inside an <svg>: rows added on a re-render are SVG elements too. The
// scope's root is a child of the svg, so the namespace must come from where
// the scope is mounted, not from the (empty) re-render context.
func TestSVGNamespaceForRowsAddedOnReRender(t *testing.T) {
	bars := core.NewSignal([]int{1})
	r, _ := mount(t, h.Div(h.Svg(h.For(bars, func(v int) int { return v }, func(v int) core.Node {
		return h.Rect(h.Attr("height", "1"))
	}))))
	bars.Set([]int{1, 2})
	if got := createdNS(r.Scheduler.Flush())["rect"]; got != core.SVGNamespace {
		t.Fatalf("a <rect> added by a re-render inside <svg> was created with namespace %q", got)
	}
}

// A scope rooted at an SVG child (<g>) keeps the namespace when it re-renders.
func TestSVGNamespaceScopeRootedAtSVGChild(t *testing.T) {
	big := core.NewSignal(false)
	r, _ := mount(t, h.Svg(h.G(hooks.UseScope(func() core.Node {
		if big.Get() {
			return h.Circle(h.Attr("r", "9"))
		}
		return h.Path()
	}, big))))
	big.Set(true)
	if got := createdNS(r.Scheduler.Flush())["circle"]; got != core.SVGNamespace {
		t.Fatalf("<circle> re-rendered under <g> was created with namespace %q", got)
	}
}

// A re-render that diffs a reused <svg> and adds children to it: the new
// children inherit the svg's namespace through the diff.
func TestSVGNamespaceChildAddedToReusedSVG(t *testing.T) {
	n := core.NewSignal(1)
	r, _ := mount(t, h.Div(hooks.UseScope(func() core.Node {
		var dots []core.Item
		for i := 0; i < n.Get(); i++ {
			dots = append(dots, h.G(h.Circle()))
		}
		return h.Svg(dots...)
	}, n)))
	n.Set(2)
	ns := createdNS(r.Scheduler.Flush())
	if _, recreated := ns["svg"]; recreated {
		t.Fatal("the <svg> should be reused, not recreated")
	}
	if ns["g"] != core.SVGNamespace || ns["circle"] != core.SVGNamespace {
		t.Fatalf("children added to a reused <svg>: g=%q circle=%q, want the SVG namespace", ns["g"], ns["circle"])
	}
}

// The namespace a scope inherits is recorded during hydration as well.
func TestSVGNamespaceScopeMountedByHydration(t *testing.T) {
	big := core.NewSignal(false)
	r := New()
	r.SetHydrating(true)
	renderFx(r, h.Svg(hooks.UseScope(func() core.Node {
		if big.Get() {
			return h.Circle()
		}
		return h.Path()
	}, big)))
	big.Set(true)
	if got := createdNS(r.Scheduler.Flush())["circle"]; got != core.SVGNamespace {
		t.Fatalf("<circle> re-rendered in a hydrated <svg> was created with namespace %q", got)
	}
}

// <foreignObject> holds HTML: its children are created without the SVG
// namespace, on the first render and on re-renders.
func TestSVGForeignObjectChildrenAreHTML(t *testing.T) {
	label := core.NewSignal("a")
	r := New()
	muts, _ := renderFx(r, h.Svg(h.SvgEl("foreignObject", hooks.UseScope(func() core.Node {
		if label.Get() == "a" {
			return h.Div(h.Text("a"))
		}
		return h.P(h.Text("b"))
	}, label))))
	ns := createdNS(muts)
	if ns["foreignObject"] != core.SVGNamespace || ns["div"] != "" {
		t.Fatalf("first render: foreignObject=%q div=%q, want SVG and HTML", ns["foreignObject"], ns["div"])
	}
	label.Set("b")
	if got, ok := createdNS(r.Scheduler.Flush())["p"]; !ok || got != "" {
		t.Fatalf("<p> re-rendered inside <foreignObject>: namespace %q (created=%v), want HTML", got, ok)
	}
}
