package dom

import (
	"strconv"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

func mountRows(n int) core.Node {
	items := make([]core.Item, n)
	for i := range items {
		items[i] = h.Li(h.Class("row"), h.Attr("data-id", strconv.Itoa(i)),
			h.Span(h.Class("label"), h.Text("Row number "+strconv.Itoa(i))),
			h.A(h.Href("#"), h.Text("remove")))
	}
	return h.Ul(items...)
}

// BenchmarkMount1000 is the Go side of mounting 1,000 rows (#139): building
// the tree, rendering it to mutations, and encoding the batch, each timed.
func BenchmarkMount1000Build(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mountRows(1000)
	}
}

func BenchmarkMount1000Render(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tree := mountRows(1000)
		r := New()
		b.StartTimer()
		r.Render(tree)
	}
}

func BenchmarkMount1000Encode(b *testing.B) {
	r := New()
	muts, _ := r.Render(mountRows(1000))
	var e BatchEncoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Encode(muts)
	}
}

// Render sizes its mutation buffer from the tree up front, and a component
// reserves its content when the walk reaches it, so neither regrows the
// buffer by doubling (#203).
func TestRenderSizesMutationBuffer(t *testing.T) {
	muts, _ := New().Render(mountRows(100))
	if len(muts) != 1602 || cap(muts)-len(muts) > 1 {
		t.Errorf("plain tree: len %d cap %d, want 1602 with no spare beyond the estimate", len(muts), cap(muts))
	}
	inner := mountRows(100)
	muts, _ = New().Render(core.Component("Rows", func() core.Node { return inner }))
	if cap(muts) > len(muts)+len(muts)/8 {
		t.Errorf("component: len %d cap %d, the buffer grew by doubling", len(muts), cap(muts))
	}
	if got := mutationEstimate(mountRows(10), true); got != 51 {
		t.Errorf("hydrating estimate = %d, want one claim per node (51)", got)
	}
}
