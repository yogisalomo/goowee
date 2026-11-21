//go:build !race

package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// #135: delivering a pointermove to one handler that reads its position and
// pointer type allocates nothing in Go (the bridge's copy of the payload text
// is the one allocation per event).
func TestAllocBudgetPointerMove(t *testing.T) {
	r := New()
	var x, y float64
	var kind string
	r.Render(h.Div(h.On("pointermove", func(e core.EventData) {
		x, y, kind = e.ClientX(), e.ClientY(), e.PointerType()
	})))
	r.Scheduler.Flush()
	id := 0
	for nid, n := range r.Registry.nodes {
		if _, ok := n.Events["pointermove"]; ok {
			id = nid
		}
	}
	payload := `{"clientX":120.5,"clientY":44,"offsetX":3,"offsetY":4,"button":0,"buttons":0,"pointerId":1,"pointerType":"mouse","pressure":0,"shiftKey":false,"ctrlKey":false,"altKey":false,"metaKey":false}`
	r.Registry.Dispatch(id, "pointermove", payload)
	if x != 120.5 || y != 44 || kind != "mouse" {
		t.Fatalf("got %v %v %q", x, y, kind)
	}
	if n := testing.AllocsPerRun(200, func() { r.Registry.Dispatch(id, "pointermove", payload) }); n > 0 {
		t.Fatalf("Dispatch of a pointermove: %v allocs, want 0", n)
	}
}

// A handler that keeps its EventData can't decide for a later event.
func TestStaleEventDataCantPreventALaterEvent(t *testing.T) {
	r := New()
	var kept core.EventData
	calls := 0
	r.Render(h.Div(h.On("click", func(e core.EventData) {
		calls++
		if calls == 1 {
			kept = e
		}
	})))
	id := 0
	for nid, n := range r.Registry.nodes {
		if _, ok := n.Events["click"]; ok {
			id = nid
		}
	}
	r.Registry.Dispatch(id, "click", `{}`)
	kept.PreventDefault() // after its event: ignored
	opts, _ := r.Registry.Dispatch(id, "click", `{}`)
	if opts.PreventDefault {
		t.Fatal("a stale EventData prevented the default of a later event")
	}
}
