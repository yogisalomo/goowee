package dom

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// #143: in dev mode, a re-render cycle's log gives the component path the
// scope renders in (the path goowee.inspect() shows).
func TestRenderCycleLogNamesTheComponent(t *testing.T) {
	var cycles []core.LogEntry
	core.SetDevChecks(true)
	core.SetLogSink(func(e core.LogEntry) {
		if e.Kind == core.LogSignalCycle {
			cycles = append(cycles, e)
		}
	})
	t.Cleanup(func() {
		core.SetDevChecks(false)
		core.SetLogSink(nil)
	})
	n := core.NewSignal(0)
	r := New()
	renderFx(r, core.Component("Looper", func() core.Node {
		return &core.ScopeNode{
			Deps: []core.SignalAccessor{n},
			Render: func() core.Node {
				n.Set(n.Peek() + 1) // every render dirties the scope again
				return h.Text("x")
			},
		}
	}))
	n.Set(-1000)
	flushFx(r)
	if len(cycles) != 1 {
		t.Fatalf("%d cycle logs, want 1", len(cycles))
	}
	paths, _ := cycles[0].Fields["components"].([]string)
	if len(paths) != 1 || !strings.HasPrefix(paths[0], "/") {
		t.Fatalf("components %v", cycles[0].Fields["components"])
	}
	if r.Scheduler.HasWork() {
		t.Fatal("work left after the cycle was dropped")
	}
}
