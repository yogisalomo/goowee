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
