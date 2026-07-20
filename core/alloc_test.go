//go:build !race

// Allocation-budget tests for hot paths. Tagged !race because the race detector
// perturbs allocation counts; CI runs these in a dedicated non-race step
// (`go test -run AllocBudget`). They guard against a regression that adds
// per-item allocation to a per-frame path, not against absolute numbers.
package core

import "testing"

// Notifying subscribers must allocate O(1), not O(number of subscribers) — the
// property the token-based subscription rework (fable review §2) guarantees.
// A regression to per-subscriber allocation shows up as this scaling with the
// subscriber count.
func TestAllocBudgetSignalNotifyConstant(t *testing.T) {
	const budget = 3
	for _, subs := range []int{100, 2000} {
		s := NewSignal(0)
		for i := 0; i < subs; i++ {
			s.Subscribe(func() {})
		}
		n := 0
		avg := testing.AllocsPerRun(200, func() { n++; s.Set(n) }) // vary value so each Set notifies
		if avg > budget {
			t.Errorf("Set with %d subscribers: %.1f allocs/op, budget %d — notify should be O(1) in allocations", subs, avg, budget)
		}
	}
}

// coalesce collapses a full frame's redundant writes; its allocation cost must
// stay bounded (a map + one output slice), not grow per mutation.
func TestAllocBudgetCoalesce(t *testing.T) {
	muts := make([]Mutation, 0, 2000)
	for i := 0; i < 1000; i++ {
		muts = append(muts,
			Mutation{Type: MutSetProperty, NodeID: i % 50, Key: "textContent", Value: i},
			Mutation{Type: MutSetAttribute, NodeID: i % 50, Key: "class", Value: "x"},
		)
	}
	const budget = 16 // ~10 today
	avg := testing.AllocsPerRun(200, func() { _ = coalesce(muts) })
	if avg > budget {
		t.Errorf("coalesce(2000 muts): %.1f allocs/op, budget %d", avg, budget)
	}
}
