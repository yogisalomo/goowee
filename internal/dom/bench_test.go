package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
)

// Re-rendering + keyed-diffing an M-row list on every dep change (§4 batching
// feeds one Flush per frame; this measures the render+diff inside it).
func BenchmarkScopeReRenderList(b *testing.B) {
	const n = 200
	asc := make([]int, n)
	desc := make([]int, n)
	for i := 0; i < n; i++ {
		asc[i] = i
		desc[i] = n - 1 - i
	}
	items := core.NewSignal(asc)
	scope := &core.ScopeNode{
		Deps: []core.SignalAccessor{items},
		Render: func() core.Node {
			xs := items.Get()
			kids := make([]core.Node, len(xs))
			for i, x := range xs {
				kids[i] = &core.ElementNode{
					Tag: "li", Key: x,
					Children: []core.Node{&core.TextNode{Value: fmt.Sprintf("row %d", x)}},
				}
			}
			return &core.FragmentNode{Children: kids}
		},
	}
	r := New()
	r.Render(&core.ElementNode{Tag: "ul", Children: []core.Node{scope}})
	r.Scheduler.Flush()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			items.Set(desc)
		} else {
			items.Set(asc)
		}
		r.Scheduler.Flush()
	}
}

// Diffing two K-node element trees (the reconciliation hot path).
func BenchmarkDiffTree(b *testing.B) {
	build := func(cls string) *core.ElementNode {
		kids := make([]core.Node, 100)
		for i := range kids {
			kids[i] = &core.ElementNode{
				Tag:      "span",
				Attrs:    []core.Attr{{Name: "class", Value: cls}},
				Children: []core.Node{&core.TextNode{Value: cls}},
			}
		}
		return &core.ElementNode{Tag: "div", Children: kids}
	}
	r := New()
	old := build("a")
	r.Render(old)
	r.Scheduler.Flush()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cls := "a"
		if i%2 == 0 {
			cls = "b"
		}
		next := build(cls)
		var muts []core.Mutation
		r.diffNode(old, next, &muts)
		old = next
	}
}
