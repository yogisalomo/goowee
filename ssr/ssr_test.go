package ssr

import (
	"fmt"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/dom"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSSRRender(t *testing.T) {
	n := &core.ElementNode{
		Tag:   "div",
		Attrs: []core.Attr{{Name: "class", Value: "greeting"}},
		Children: []core.Node{
			&core.TextNode{Value: "hello"},
		},
	}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, "data-node-id") {
		t.Fatal("expected data-node-id")
	}
	if !strings.Contains(html, "greeting") {
		t.Fatal("expected class")
	}
	if !strings.Contains(html, "hello") {
		t.Fatal("expected text")
	}
}

func TestSSRWithMeta(t *testing.T) {
	count := core.NewSignal(0)
	n := &core.ElementNode{
		Tag:   "button",
		Attrs: []core.Attr{{Name: "class", Value: "btn"}},
		Binds: []core.Bind{{
			Target: core.BindToProp, Name: "textContent", Signal: count,
		}},
	}
	r := New()
	html, meta := r.RenderWithMeta(n, "root/0", []core.SignalAccessor{count})

	if !strings.Contains(html, "data-node-id") {
		t.Fatal("expected data-node-id")
	}
	if meta.NodeMap[1] != "root/0" {
		t.Fatalf("expected NodeMap[1]=root/0, got %q", meta.NodeMap[1])
	}
	deps, ok := meta.Deps[1]
	if !ok || len(deps) != 1 {
		t.Fatal("expected Deps[1]")
	}
	if deps[0].ComponentPath != "root/0" {
		t.Fatalf("expected path root/0, got %q", deps[0].ComponentPath)
	}
}

func TestSSRFragment(t *testing.T) {
	n := &core.FragmentNode{
		Children: []core.Node{
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "a"}}},
			&core.ElementNode{Tag: "span", Attrs: []core.Attr{{Name: "class", Value: "b"}}},
		},
	}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, "data-node-id") {
		t.Fatal("expected data-node-id")
	}
}

func TestSSRTextNodeSignal(t *testing.T) {
	label := core.NewSignal("hello")
	n := &core.ElementNode{
		Tag: "p",
		Children: []core.Node{
			&core.TextNode{Value: label},
		},
	}
	r := New()
	html, meta := r.RenderWithMeta(n, "root/greeting", []core.SignalAccessor{label})

	if !strings.Contains(html, "hello") {
		t.Fatal("expected text")
	}
	if len(meta.NodeMap) < 2 {
		t.Fatal("expected at least 2 nodes in NodeMap")
	}
}

func TestSSRComponentNode(t *testing.T) {
	comp := core.Component("MyComp", func() core.Node {
		return &core.ElementNode{
			Tag:   "p",
			Attrs: []core.Attr{{Name: "class", Value: "comp"}},
		}
	})
	r := New()
	html := r.Render(comp)
	if !strings.Contains(html, "data-node-id") {
		t.Fatal("expected data-node-id")
	}
	if !strings.Contains(html, "comp") {
		t.Fatal("expected class")
	}
}

func TestSSRScopeNode(t *testing.T) {
	comp := core.ScopeNode{
		Render: func() core.Node {
			return &core.ElementNode{
				Tag:   "p",
				Attrs: []core.Attr{{Name: "class", Value: "scoped"}},
			}
		},
	}
	r := New()
	html := r.Render(&comp)
	if !strings.Contains(html, "scoped") {
		t.Fatal("expected class from scope")
	}
	if !strings.Contains(html, "data-node-id") {
		t.Fatal("expected data-node-id")
	}
}

func TestSSRIDMatchesDOM(t *testing.T) {
	comp := core.Component("IDTest", func() core.Node {
		count, _ := hooks.UseState(0)
		return &core.ElementNode{
			Tag:   "span",
			Attrs: []core.Attr{{Name: "class", Value: "test"}},
			Binds: []core.Bind{{
				Target: core.BindToProp, Name: "textContent", Signal: count,
			}},
		}
	})

	ssrR := New()
	ssrHTML := ssrR.Render(comp)

	domR := dom.New()
	muts, domID := domR.Render(comp)

	if !strings.Contains(ssrHTML, fmt.Sprintf(`data-node-id="%d"`, domID)) {
		t.Fatalf("DOM root ID %d not found in SSR HTML: %s", domID, ssrHTML)
	}

	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			expectedAttr := fmt.Sprintf(`data-node-id="%d"`, m.NodeID)
			if !strings.Contains(ssrHTML, expectedAttr) {
				t.Fatalf("SSR missing %s for element %s", expectedAttr, m.Value)
			}
		}
	}
}

func TestSSREscapeHTML(t *testing.T) {
	n := &core.TextNode{Value: "a < b & c > d"}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, "&lt;") || !strings.Contains(html, "&amp;") || !strings.Contains(html, "&gt;") {
		t.Fatalf("expected escaped HTML, got %s", html)
	}
}

func TestAttrEscaping(t *testing.T) {
	n := &core.ElementNode{
		Tag:   "div",
		Attrs: []core.Attr{{Name: "title", Value: `he said "hello"`}},
	}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, "&quot;") {
		t.Fatalf("expected escaped attribute value, got %s", html)
	}
}

func TestVoidElements(t *testing.T) {
	n := &core.ElementNode{
		Tag: "br",
	}
	r := New()
	html := r.Render(n)
	if strings.Contains(html, "</br>") {
		t.Fatalf("expected no closing tag for void element, got %s", html)
	}
	if !strings.HasSuffix(strings.TrimSpace(html), ">") {
		t.Fatalf("expected void element to end with >, got %s", html)
	}

	input := &core.ElementNode{
		Tag:   "input",
		Attrs: []core.Attr{{Name: "type", Value: "text"}},
	}
	html2 := r.Render(input)
	if strings.Contains(html2, "</input>") {
		t.Fatalf("expected no closing tag for void input, got %s", html2)
	}
}

func TestBoolPropsAsAttributes(t *testing.T) {
	n := &core.ElementNode{
		Tag: "input",
		Props: []core.Prop{
			{Name: "disabled", Value: true},
		},
	}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, " disabled") {
		t.Fatalf("expected disabled attribute, got %s", html)
	}

	n2 := &core.ElementNode{
		Tag: "input",
		Props: []core.Prop{
			{Name: "disabled", Value: false},
		},
	}
	html2 := r.Render(n2)
	if strings.Contains(html2, "disabled") {
		t.Fatalf("expected no disabled attribute when false, got %s", html2)
	}

	n3 := &core.ElementNode{
		Tag: "input",
		Props: []core.Prop{
			{Name: "readOnly", Value: true},
		},
	}
	html3 := r.Render(n3)
	if !strings.Contains(html3, " readonly") {
		t.Fatalf("expected readonly attribute, got %s", html3)
	}
}

func TestBindSerialization(t *testing.T) {
	sig := core.NewSignal("hello")
	n := &core.ElementNode{
		Tag: "div",
		Binds: []core.Bind{{
			Target: core.BindToAttr, Name: "title", Signal: sig,
		}},
	}
	r := New()
	html := r.Render(n)
	if !strings.Contains(html, `title="hello"`) {
		t.Fatalf("expected title attribute with resolved value, got %s", html)
	}
}

// Concurrent server renders must not share the frame stack. With the old
// package-global renderStack this races (and can cross-wire frames); each
// render now runs under its own RenderContext. Run under -race.
func TestConcurrentServerRendersNoRace(t *testing.T) {
	build := func(n int) core.Node {
		return core.Component("Outer", func() core.Node {
			return &core.ElementNode{Tag: "div", Children: []core.Node{
				core.Component("Inner", func() core.Node {
					return &core.ElementNode{Tag: "span", Children: []core.Node{
						&core.TextNode{Value: fmt.Sprintf("n=%d", n)},
					}}
				}),
			}}
		})
	}

	const goroutines = 64
	var wg sync.WaitGroup
	errs := make(chan string, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := New().Render(build(i))
			if want := fmt.Sprintf("n=%d</span>", i); !strings.Contains(got, want) {
				errs <- fmt.Sprintf("render %d missing %q in %q", i, want, got)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// Effects/OnMount must not execute during a server render (they'd leak a
// goroutine or subscription per request, never cleaned up server-side).
func TestServerRenderSkipsEffects(t *testing.T) {
	var ran int32
	comp := core.Component("Effectful", func() core.Node {
		hooks.OnMount(func() func() {
			atomic.AddInt32(&ran, 1)
			return nil
		})
		return &core.ElementNode{Tag: "div", Children: []core.Node{&core.TextNode{Value: "x"}}}
	})

	html := New().Render(comp)
	if !strings.Contains(html, "x") {
		t.Fatalf("expected rendered content, got %q", html)
	}
	if atomic.LoadInt32(&ran) != 0 {
		t.Fatalf("OnMount ran during ssr.Render (ran=%d), should be skipped server-side", ran)
	}
}
