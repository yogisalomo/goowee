package dom

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/storage"
)

// UsePersist waits for the mount: the hydrating render claims the server's
// markup with the value the server rendered (it has no browser storage), and
// the stored value arrives as an ordinary update afterwards.
func TestUsePersistLoadsAfterHydration(t *testing.T) {
	storage.Local.Set("n", "5")
	defer storage.Local.Clear()
	n := core.NewSignal(0)
	r := New()
	r.SetHydrating(true)
	muts, _ := r.Render(core.Component("C", func() core.Node {
		hooks.UsePersist(n, "n")
		return h.P(h.Textf("%d", n))
	}))
	for _, m := range muts {
		if m.Type != core.MutHydrate || fmt.Sprint(m.Value) == "5" {
			t.Fatalf("hydration must only claim the server's nodes: %v %+v", m.Type, m)
		}
	}
	if n.Get() != 0 {
		t.Fatalf("loaded during the render: %d", n.Get())
	}
	r.Scheduler.RunEffects()
	got := r.Scheduler.Flush()
	updated := false
	for _, m := range got {
		updated = updated || (m.Type == core.MutSetProperty && fmt.Sprint(m.Value) == "5")
	}
	if n.Get() != 5 || !updated {
		t.Fatalf("after mount n=%d, mutations %+v", n.Get(), got)
	}
}
