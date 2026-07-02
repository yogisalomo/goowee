package ssr

import (
	"fmt"
	"goowee/core"
	"goowee/dom"
	"goowee/hooks"
	"strings"
	"testing"
)

func TestSSRRender(t *testing.T) {
	n := &core.ElementNode{
		Tag: "div",
		Props: map[string]any{"class": "greeting"},
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
		Tag: "button",
		Props: map[string]any{
			"textContent": count,
			"class":       "btn",
		},
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
			&core.ElementNode{Tag: "span", Props: map[string]any{"class": "a"}},
			&core.ElementNode{Tag: "span", Props: map[string]any{"class": "b"}},
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
			Props: map[string]any{"class": "comp"},
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
				Props: map[string]any{"class": "scoped"},
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
	// Component with hooks that both SSR and DOM must render with matching IDs
	comp := core.Component("IDTest", func() core.Node {
		count, _ := hooks.UseState(0)
		return &core.ElementNode{
			Tag: "span",
			Props: map[string]any{
				"textContent": count,
				"class":       "test",
			},
		}
	})

	// SSR render
	ssrR := New()
	ssrHTML := ssrR.Render(comp)

	// DOM render
	domR := dom.New()
	muts, domID := domR.Render(comp)

	// Verify DOM root ID matches SSR data-node-id
	if !strings.Contains(ssrHTML, fmt.Sprintf(`data-node-id="%d"`, domID)) {
		t.Fatalf("DOM root ID %d not found in SSR HTML: %s", domID, ssrHTML)
	}

	// For each CreateElement mutation, verify data-node-id exists in SSR
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
