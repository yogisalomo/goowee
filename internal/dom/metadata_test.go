package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

// metadataTree is a <div> holding a Metadata block (title + meta) followed by a
// body paragraph — the shape appLayout produces.
func metadataTree() core.Node {
	return &core.ElementNode{Tag: "div", Children: []core.Node{
		&core.MetadataNode{Children: []core.Node{
			&core.ElementNode{Tag: "title", Children: []core.Node{&core.TextNode{Value: "Test"}}},
			&core.ElementNode{Tag: "meta", Attrs: []core.Attr{
				{Name: "name", Value: "description"}, {Name: "content", Value: "d"},
			}},
		}},
		&core.ElementNode{Tag: "p", Children: []core.Node{&core.TextNode{Value: "hello"}}},
	}}
}

func headAppends(muts []core.Mutation) int {
	n := 0
	for _, m := range muts {
		if m.Type == core.MutPortalAppend && m.Value == "head" {
			n++
		}
	}
	return n
}

// A fresh (pure-client, no SSR) render creates the head tags and appends them
// to <head>.
func TestMetadataFreshRenderAppendsHead(t *testing.T) {
	muts, _ := New().Render(metadataTree())
	if got := headAppends(muts); got != 2 {
		t.Fatalf("fresh render: expected 2 head appends (title, meta), got %d", got)
	}
}

// Regression: on hydration the server has already rendered <head>, so the
// client must NOT re-create or re-append the head tags — doing so duplicates
// every <title>/<meta>/… on the page. Body nodes must still hydrate.
func TestMetadataHydrationNoDuplicateHead(t *testing.T) {
	r := New()
	r.SetHydrating(true)
	muts, _ := r.Render(metadataTree())

	if got := headAppends(muts); got != 0 {
		t.Fatalf("hydration must not append to <head> (would duplicate server tags), got %d appends", got)
	}
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			if s, _ := m.Value.(string); s == "title" || s == "meta" {
				t.Fatalf("hydration must not re-create head element %q", s)
			}
		}
	}

	sawHydrate := false
	for _, m := range muts {
		if m.Type == core.MutHydrate {
			sawHydrate = true
			break
		}
	}
	if !sawHydrate {
		t.Fatal("hydration should still claim body nodes via MutHydrate")
	}
}

// Node-id allocation must be identical whether hydrating or not, so that body
// nodes after a Metadata block hydrate against the right server nodes. The <p>
// and its text sit after the head's title/text and meta, so they must land on
// the same ids in both modes.
func TestMetadataHydrationIDParity(t *testing.T) {
	fresh, _ := New().Render(metadataTree())

	rh := New()
	rh.SetHydrating(true)
	hyd, _ := rh.Render(metadataTree())

	pFresh := createID(fresh, "p")
	pHyd := hydrateID(hyd, "p")
	if pFresh == 0 || pHyd == 0 {
		t.Fatalf("could not locate <p> id (fresh=%d hyd=%d)", pFresh, pHyd)
	}
	if pFresh != pHyd {
		t.Fatalf("<p> id diverged between fresh (%d) and hydration (%d): head walk broke id parity", pFresh, pHyd)
	}
}

func createID(muts []core.Mutation, tag string) int {
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			if s, _ := m.Value.(string); s == tag {
				return m.NodeID
			}
		}
	}
	return 0
}

func hydrateID(muts []core.Mutation, tag string) int {
	for _, m := range muts {
		if m.Type == core.MutHydrate {
			if s, _ := m.Value.(string); s == tag {
				return m.NodeID
			}
		}
	}
	return 0
}
