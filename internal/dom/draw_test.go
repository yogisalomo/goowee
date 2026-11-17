package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

func TestSplitDraws(t *testing.T) {
	muts := []core.Mutation{
		{Type: core.MutSetAttribute, NodeID: 1, Key: "a"},
		{Type: core.MutDraw, NodeID: 2, Value: []uint32{1}},
		{Type: core.MutAppendChild, NodeID: 0, ChildID: 1},
		{Type: core.MutDraw, NodeID: 3, Value: []uint32{2}},
	}
	rest, draws := SplitDraws(muts)
	if len(rest) != 2 || rest[0].Key != "a" || rest[1].Type != core.MutAppendChild {
		t.Fatalf("rest = %v", rest)
	}
	if len(draws) != 2 || draws[0].NodeID != 2 || draws[1].NodeID != 3 {
		t.Fatalf("draws = %v", draws)
	}

	plain := muts[:1]
	if n := testing.AllocsPerRun(100, func() {
		r, d := SplitDraws(plain)
		if len(r) != 1 || d != nil {
			t.Fatal("wrong split")
		}
	}); n != 0 {
		t.Fatalf("a batch without draws allocates %v times", n)
	}
}
