package core

import "testing"

// Signal fan-out: one signal notifying many subscribers per Set. Guards the
// §2 subscription rework (token-based, copy-before-notify).
func BenchmarkSignalSetFanout(b *testing.B) {
	s := NewSignal(0)
	for i := 0; i < 1000; i++ {
		s.Subscribe(func() {})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Set(i)
	}
}

// Coalescing a queue full of redundant writes to the same nodes (the §4 flush
// path).
func BenchmarkCoalesce(b *testing.B) {
	muts := make([]Mutation, 0, 2000)
	for i := 0; i < 1000; i++ {
		muts = append(muts,
			Mutation{Type: MutSetProperty, NodeID: i % 50, Key: "textContent", Value: i},
			Mutation{Type: MutSetAttribute, NodeID: i % 50, Key: "class", Value: "x"},
		)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = coalesce(muts)
	}
}
