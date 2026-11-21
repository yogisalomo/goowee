//go:build !race

package dom

import (
	"strconv"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

type listRow struct {
	ID   int
	Text string
}

// #140: appending to, and updating one row of, a h.ForList costs the same
// mutations and allocations at 1,000 rows as at 10,000.
func TestAllocBudgetListOps(t *testing.T) {
	measure := func(n int) (appendAllocs, updateAllocs float64, appendMuts, updateMuts int) {
		items := make([]listRow, n)
		for i := range items {
			items[i] = listRow{i, "row"}
		}
		list := core.NewList(func(r listRow) int { return r.ID }, items...)
		r := New()
		r.Render(h.Ul(h.ForList(list, func(it *core.Signal[listRow]) core.Node {
			return h.Li(h.TextS(core.Computed([]core.SignalAccessor{it}, func() string { return it.Get().Text })))
		})))
		r.Scheduler.Flush()
		next := n
		appendAllocs = testing.AllocsPerRun(50, func() {
			list.Append(listRow{next, "new"})
			next++
			appendMuts = len(r.Scheduler.Flush())
		})
		v := 0
		updateAllocs = testing.AllocsPerRun(50, func() {
			v++
			text := strconv.Itoa(v & 1)
			list.Update(n/2, func(r *listRow) { r.Text = text })
			updateMuts = len(r.Scheduler.Flush())
		})
		return
	}
	a1, u1, am1, um1 := measure(1000)
	a10, u10, am10, um10 := measure(10000)
	t.Logf("append: %.0f allocs, %d mutations at 1k; %.0f, %d at 10k", a1, am1, a10, am10)
	t.Logf("update: %.0f allocs, %d mutations at 1k; %.0f, %d at 10k", u1, um1, u10, um10)
	if am10 != am1 || um10 != um1 {
		t.Errorf("mutations grow with the list: append %d -> %d, update %d -> %d", am1, am10, um1, um10)
	}
	if a10 > a1+2 || u10 > u1+2 {
		t.Errorf("allocations grow with the list: append %.0f -> %.0f, update %.0f -> %.0f", a1, a10, u1, u10)
	}
}
