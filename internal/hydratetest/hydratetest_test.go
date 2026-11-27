package hydratetest_test

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/internal/dom"
	"github.com/yogisalomo/goowee/internal/hydratetest"
	"github.com/yogisalomo/goowee/ssr"
)

func hydrate(n func() core.Node) []core.Mutation {
	r := dom.New()
	r.SetHydrating(true)
	muts, _ := r.Render(n())
	return muts
}

func claims(muts []core.Mutation) int {
	c := 0
	for _, m := range muts {
		if m.Type == core.MutHydrate {
			c++
		}
	}
	return c
}

// The cases positional hydration has to get right (ADR-033): text nodes the
// parser would merge, empty text, content only the server writes, scopes,
// raw-text elements and the newline the parser drops after <pre>.
func TestPositionalClaims(t *testing.T) {
	count := core.NewSignal(3)
	empty := core.NewSignal("")
	show := core.NewSignal(true)
	cases := map[string]func() core.Node{
		"adjacent text": func() core.Node {
			return h.P(h.Text("Count: "), h.TextS(count), h.Text(" items"))
		},
		"empty text": func() core.Node {
			return h.P(h.TextS(empty), h.Text("after"), h.TextS(empty))
		},
		"raw html": func() core.Node {
			return h.Div(h.Raw("<b>bold</b><i>x</i>"), h.Span(h.Text("next")))
		},
		"text content": func() core.Node {
			return h.Div(h.Button(h.Prop("textContent", "7")), h.Span(h.Text("next")))
		},
		"scope": func() core.Node {
			return h.Div(h.Text("a"), h.Show(show, func() core.Node { return h.Text("b") }), h.Text("c"))
		},
		"textarea and pre": func() core.Node {
			return h.Div(h.Textarea(h.Text("\nline")), h.Pre(h.Text("\ncode")), h.P(h.Text("end")))
		},
	}
	for name, tree := range cases {
		t.Run(name, func(t *testing.T) {
			html, _ := ssr.New().Render(tree())
			muts := hydrate(tree)
			claimed, mismatches := hydratetest.Claim(html, muts)
			if len(mismatches) > 0 || len(claimed) != claims(muts) {
				t.Fatalf("%d of %d claims; %v\nhtml: %s", len(claimed), claims(muts), mismatches, html)
			}
		})
	}
}

func TestAdjacentTextKeepsBoundaries(t *testing.T) {
	count := core.NewSignal(3)
	tree := func() core.Node { return h.P(h.Text("Count: "), h.TextS(count)) }
	html, _ := ssr.New().Render(tree())
	if !strings.Contains(html, "Count: <!---->3") {
		t.Fatalf("no separator between adjacent texts: %s", html)
	}
	claimed, _ := hydratetest.Claim(html, hydrate(tree))
	var texts []string
	for _, n := range claimed {
		if n.Kind == hydratetest.Text {
			texts = append(texts, n.Tag)
		}
	}
	if len(texts) != 2 {
		t.Fatalf("claimed texts %q, want two nodes", texts)
	}
}

// A different tree on the client is reported, with the node id.
func TestMismatchIsReported(t *testing.T) {
	html, _ := ssr.New().Render(h.Div(h.P(h.Text("server"))))
	_, mismatches := hydratetest.Claim(html, hydrate(func() core.Node { return h.Div(h.Span(h.Text("client"))) }))
	if len(mismatches) == 0 || !strings.Contains(mismatches[0], "expected <span>, server rendered <p>") {
		t.Fatalf("mismatches = %v", mismatches)
	}
}
