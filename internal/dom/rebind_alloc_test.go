//go:build !race

package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
)

// #137: a no-op re-render of 100 bound rows (nothing bound changed) costs
// building the new tree and diffing it, and no binding churn: no
// unsubscribe, subscribe or mutation per row. Before #137 it took 1,834
// allocations; today it is about 520, nearly all of it building the new rows.
func TestAllocBudgetNoOpReRenderOfBoundRows(t *testing.T) {
	tick := core.NewSignal(0)
	labels := make([]*core.Signal[string], 100)
	for i := range labels {
		labels[i] = core.NewSignal(fmt.Sprintf("row-%d", i))
	}
	r := New()
	renderFx(r, boundRows(100, tick, labels))
	n := 0
	avg := testing.AllocsPerRun(50, func() {
		n++
		tick.Set(n)
		if muts := flushFx(r); len(muts) != 0 {
			t.Fatalf("sent %d mutations", len(muts))
		}
	})
	t.Logf("%.0f allocs per no-op re-render of 100 bound rows", avg)
	if avg > noOpReRenderBudget {
		t.Fatalf("%.0f allocs per no-op re-render, budget %d", avg, noOpReRenderBudget)
	}
}

const noOpReRenderBudget = 650
