package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// #154: several roots share one scheduler and id counter; each mounts into
// its own container and unmounts on its own.
func TestRootsShareSchedulerAndIDs(t *testing.T) {
	base := New()
	other := NewRoot(base, -1000)
	a := core.NewSignal("a")
	m1, _ := base.Render(h.P(h.TextS(a)))
	m2, _ := other.Render(h.Div(h.Span(h.Text("b"))))

	ids := map[int]bool{}
	for _, m := range append(append([]core.Mutation{}, m1...), m2...) {
		if m.Type == core.MutCreateElement {
			if ids[m.NodeID] {
				t.Fatalf("node id %d used by both roots", m.NodeID)
			}
			ids[m.NodeID] = true
		}
	}
	last := func(muts []core.Mutation) core.Mutation { return muts[len(muts)-1] }
	if m := last(m1); m.Type != core.MutAppendChild || m.NodeID != 0 {
		t.Fatalf("first root appends to %+v, want #root (0)", m)
	}
	if m := last(m2); m.Type != core.MutAppendChild || m.NodeID != -1000 {
		t.Fatalf("second root appends to %+v, want its container (-1000)", m)
	}
	if other.Scheduler != base.Scheduler || other.Registry != base.Registry || other.Bindings != base.Bindings {
		t.Fatal("roots don't share the scheduler and registries")
	}

	// A binding in the first root still updates through the shared scheduler.
	a.Set("a2")
	if muts := base.Scheduler.Flush(); len(muts) != 1 || muts[0].Val() != "a2" {
		t.Fatalf("binding update: %+v", muts)
	}

	// Unmounting the second root removes only its nodes.
	other.Unmount()
	muts := base.Scheduler.Flush()
	if len(muts) != 1 || muts[0].Type != core.MutRemoveNode || !ids[muts[0].NodeID] {
		t.Fatalf("unmount: %+v", muts)
	}
	a.Set("a3")
	if muts := base.Scheduler.Flush(); len(muts) != 1 || muts[0].Val() != "a3" {
		t.Fatalf("the first root stopped updating after the second unmounted: %+v", muts)
	}
}
