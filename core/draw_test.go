package core

import "testing"

func TestCoalesceKeepsTheLastDrawPerCanvas(t *testing.T) {
	a, b, c := []uint32{1}, []uint32{2}, []uint32{3}
	out := coalesce([]Mutation{
		{Type: MutDraw, NodeID: 1, Value: a},
		{Type: MutSetAttribute, NodeID: 1, Key: "width", Value: "10"},
		{Type: MutDraw, NodeID: 2, Value: b},
		{Type: MutDraw, NodeID: 1, Value: c},
	})
	if len(out) != 3 {
		t.Fatalf("got %d mutations: %v", len(out), out)
	}
	if out[0].Type != MutSetAttribute || out[1].NodeID != 2 || out[2].NodeID != 1 {
		t.Fatalf("wrong order: %v", out)
	}
	if v := out[2].Value.([]uint32); &v[0] != &c[0] {
		t.Fatal("kept the earlier draw for node 1")
	}
}

func TestRefDrawEnqueues(t *testing.T) {
	s := NewScheduler()
	SetActiveScheduler(s)
	defer SetActiveScheduler(nil)

	(&Ref{}).Draw([]uint32{1}) // not rendered yet
	(&Ref{ID: 4}).Draw(nil)    // nothing to draw
	if s.Len() != 0 {
		t.Fatalf("queued %d mutations for no-op draws", s.Len())
	}
	buf := []uint32{1, 1, 0, 0}
	(&Ref{ID: 4}).Draw(buf)
	muts := s.Flush()
	if len(muts) != 1 || muts[0].Type != MutDraw || muts[0].NodeID != 4 || muts[0].Type.String() != "Draw" {
		t.Fatalf("got %v", muts)
	}
}
