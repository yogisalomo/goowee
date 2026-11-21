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

// layout is a mounted layoutGraph.
type layout struct {
	mount []core.Mutation // the mount's mutations
	tick  func(step int)  // moves every node: one tick of a force layout
	nudge func(step int)  // moves node 0 alone
}

// layoutGraph mounts an SVG graph — nodes circles, edges lines whose endpoints
// are bound to the same position signals. With 500 nodes and 1,000 edges a
// tick is 5,000 attribute updates, the mutation transport's heaviest realistic
// frame (roadmap 3.4), and a nudge is 10.
func layoutGraph(r *DOMRenderer, nodes, edges int) layout {
	xs := make([]*core.Signal[float64], nodes)
	ys := make([]*core.Signal[float64], nodes)
	var shapes []core.Item
	for i := range xs {
		xs[i], ys[i] = core.NewSignal(float64(i)), core.NewSignal(float64(i))
		shapes = append(shapes, h.Circle(h.BindAttr("cx", xs[i]), h.BindAttr("cy", ys[i]), h.Attr("r", "4")))
	}
	for e := 0; e < edges; e++ {
		a, b := e%nodes, (e*7+1)%nodes
		shapes = append(shapes, h.Line(
			h.BindAttr("x1", xs[a]), h.BindAttr("y1", ys[a]),
			h.BindAttr("x2", xs[b]), h.BindAttr("y2", ys[b]),
		))
	}
	mount, _ := r.Render(h.Svg(shapes...))
	return layout{
		mount: append(mount, r.Scheduler.Flush()...),
		tick: func(step int) {
			core.Batch(func() {
				for i := range xs {
					xs[i].Set(float64(i) + float64(step)*0.37)
					ys[i].Set(float64(i) - float64(step)*0.61)
				}
			})
		},
		nudge: func(step int) {
			core.Batch(func() {
				xs[0].Set(float64(step) * 0.37)
				ys[0].Set(-float64(step) * 0.61)
			})
		},
	}
}

// BenchmarkLayoutTickFrame produces one layout-tick frame: the signal writes,
// binding notifications and coalescing that turn into the frame's mutations.
func BenchmarkLayoutTickFrame(b *testing.B) {
	r := New()
	g := layoutGraph(r, 500, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.tick(i + 1)
		if muts := r.Scheduler.Flush(); len(muts) != 5000 {
			b.Fatalf("tick produced %d mutations, want 5000", len(muts))
		}
	}
}

// BenchmarkEncodeLayoutTick serializes one layout-tick frame as JSON, the
// fallback transport; bytes/op is the payload that crosses to JS.
func BenchmarkEncodeLayoutTick(b *testing.B) {
	r := New()
	g := layoutGraph(r, 500, 1000)
	g.tick(1)
	muts := r.Scheduler.Flush()
	data, _ := EncodeBatch(muts)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncodeBatch(muts)
	}
}

// BenchmarkBatchEncodeLayoutTick is BenchmarkEncodeLayoutTick in the binary
// format the bridge sends by default.
func BenchmarkBatchEncodeLayoutTick(b *testing.B) {
	r := New()
	g := layoutGraph(r, 500, 1000)
	g.tick(1)
	muts := r.Scheduler.Flush()
	var e BatchEncoder
	data, _ := e.Encode(muts)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Encode(muts)
	}
}

// smallBatch is a typical interaction's frame: a click that updates a label,
// a class and a button's disabled state.
var smallBatch = []core.Mutation{
	{Type: core.MutSetProperty, NodeID: 41, Key: "textContent", Value: "Count: 42"},
	{Type: core.MutSetAttribute, NodeID: 40, Key: "class", Value: "counter counter--even"},
	{Type: core.MutSetProperty, NodeID: 43, Key: "disabled", Value: false},
}

func BenchmarkEncodeSmall(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		EncodeBatch(smallBatch)
	}
}

func BenchmarkBatchEncodeSmall(b *testing.B) {
	var e BatchEncoder
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Encode(smallBatch)
	}
}
