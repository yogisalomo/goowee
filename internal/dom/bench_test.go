package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
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

// Rendering a deep (not wide) tree — stresses the recursive walker and the
// per-level mutation emission depth, complementing the wide-list benchmarks
// (fan-out, list re-render). Roadmap 3.5.
func BenchmarkDeepTreeRender(b *testing.B) {
	const depth = 400
	var node core.Node = &core.TextNode{Value: "leaf"}
	for i := 0; i < depth; i++ {
		node = &core.ElementNode{Tag: "div", Children: []core.Node{node}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New().Render(node)
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
		r.diffNode(old, next, 0, 0, &muts)
		old = next
	}
}

// Appending one row to a 5,000-row h.For list: For reuses unchanged rows
// (render runs once) and the differ skips reused nodes, so the cost is the
// keyed bookkeeping, not re-rendering every row (#70).
func BenchmarkForAppendOne5k(b *testing.B) {
	xs := make([]int, 5000)
	for i := range xs {
		xs[i] = i
	}
	items := core.NewSignal(xs)
	r := New()
	r.Render(h.Ul(h.For(items, func(i int) int { return i }, func(i int) core.Node {
		return h.Li(h.Text("row"))
	})))
	next := len(xs)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur := items.Get()
		grown := make([]int, len(cur)+1)
		copy(grown, cur)
		grown[len(cur)] = next
		next++
		items.Set(grown)
		r.Scheduler.Flush()
	}
}
