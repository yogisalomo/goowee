//go:build !race

package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// boundUpdates mounts n elements, each with a bound class (string), a bound
// numeric attribute (float64) and bound text (int), and returns a function
// that sets every signal once and flushes the frame.
func boundUpdates(r *DOMRenderer, n int) func(step int) []core.Mutation {
	cls := make([]*core.Signal[string], n)
	xs := make([]*core.Signal[float64], n)
	counts := make([]*core.Signal[int], n)
	labels := make([]string, 16)
	for i := range labels {
		labels[i] = fmt.Sprintf("c%d", i)
	}
	var items []core.Item
	for i := 0; i < n; i++ {
		cls[i], xs[i], counts[i] = core.NewSignal(""), core.NewSignal(0.0), core.NewSignal(0)
		items = append(items, h.Span(h.ClassS(cls[i]), h.BindAttr("data-x", xs[i]), h.TextS(counts[i])))
	}
	muts, _ := r.Render(h.Div(items...))
	_ = muts
	r.Scheduler.Flush()
	return func(step int) []core.Mutation {
		core.Batch(func() {
			for i := 0; i < n; i++ {
				cls[i].Set(labels[(i+step)%len(labels)])
				xs[i].Set(float64(step) + 0.5)
				counts[i].Set(step)
			}
		})
		return r.Scheduler.Flush()
	}
}

// #136: a bound update allocates nothing at steady state: no boxing of the
// signal's value, no formatting into a new string, no coalescing map. What a
// frame allocates (the batch handed to the caller, the Batch bookkeeping)
// doesn't grow with its updates. The frame is also encoded, as the bridge does.
func TestAllocBudgetBoundUpdates(t *testing.T) {
	const budget = 20 // per frame; 15 today
	for _, n := range []int{100, 2000} {
		r := New()
		frame := boundUpdates(r, n)
		var enc BatchEncoder
		step := 0
		run := func() {
			step++
			enc.Encode(frame(step))
		}
		run()
		run()
		avg := testing.AllocsPerRun(20, run)
		t.Logf("%d bound updates: %.0f allocs per frame", 3*n, avg)
		if avg > budget {
			t.Errorf("%d bound updates: %.0f allocs per frame, budget %d (it must not grow with the updates)", 3*n, avg, budget)
		}
	}
}
