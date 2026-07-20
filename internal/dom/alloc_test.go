//go:build !race

// Allocation-budget tests for the reconciliation hot path. See core/alloc_test.go
// for why these are !race and run in a dedicated non-race CI step.
package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

// A build+diff cycle of a 100-node tree must not regress into excessive
// allocation. The budget is generous headroom over today's ~720 (which
// includes rebuilding the new tree, matching BenchmarkDiffTree); it catches a
// ~1.4x blowup, e.g. an accidental per-node allocation added to the differ.
func TestAllocBudgetDiffTree(t *testing.T) {
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

	flip := 0
	avg := testing.AllocsPerRun(100, func() {
		flip++
		cls := "a"
		if flip%2 == 0 {
			cls = "b"
		}
		next := build(cls)
		var muts []core.Mutation
		r.diffNode(old, next, &muts)
		old = next
	})
	const budget = 1000
	if avg > budget {
		t.Errorf("diffNode build+diff cycle (100-span tree): %.0f allocs/op, budget %d", avg, budget)
	}
}
