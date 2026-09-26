package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// #67: a handler decides preventDefault/stopPropagation per event.
func TestHandlerDecidesPreventDefaultPerEvent(t *testing.T) {
	r := New()
	r.Render(h.Textarea(h.OnKeyDownE(func(e core.EventData) {
		if e.Key() == "Enter" && !e.ShiftKey() {
			e.PreventDefault()
			e.StopPropagation()
		}
	})))
	opts, _ := r.Registry.Dispatch(1, "keydown", `{"key":"Enter"}`)
	if !opts.PreventDefault || !opts.StopPropagation {
		t.Fatalf("Enter should prevent and stop, got %+v", opts)
	}
	opts, _ = r.Registry.Dispatch(1, "keydown", `{"key":"Enter","shiftKey":true}`)
	if opts.PreventDefault || opts.StopPropagation {
		t.Fatalf("Shift+Enter should not, got %+v", opts)
	}
	// Static options still apply, and a panicking handler keeps them.
	r2 := New()
	r2.Render(h.Button(h.OnClick(func() { panic("boom") }, h.PreventDefault())))
	if opts, handled := r2.Registry.Dispatch(1, "click", `{}`); !handled || !opts.PreventDefault {
		t.Fatalf("static PreventDefault lost: %+v", opts)
	}
}

// #66: events that don't bubble are announced for capture-phase listening.
func TestNonBubblingEventsAreCaptured(t *testing.T) {
	got := map[string]bool{}
	reg := NewNodeRegistry()
	reg.OnNewEventType = func(ev string, capture bool) { got[ev] = capture }
	for _, ev := range []string{"click", "invalid", "mouseenter", "load", "focus", "input", "scroll"} {
		reg.RegisterHandler(1, ev, func(core.EventData) {}, core.HandlerOptions{})
	}
	want := map[string]bool{"click": false, "input": false, "invalid": true, "mouseenter": true, "load": true, "focus": true, "scroll": true}
	for ev, capture := range want {
		if got[ev] != capture {
			t.Fatalf("%s: capture=%v, want %v", ev, got[ev], capture)
		}
	}
}

// #68: pointer, wheel, touch and keyboard data decode into typed accessors.
func TestEventDataAccessors(t *testing.T) {
	r := New()
	var got core.EventData
	r.Render(h.Div(h.On("any", func(e core.EventData) { got = e })))
	r.Registry.Dispatch(1, "any", `{
		"clientX": 10, "clientY": 20, "offsetX": 3, "offsetY": 4, "button": 2, "buttons": 3,
		"pointerId": 7, "pointerType": "pen", "pressure": 0.5,
		"deltaX": 1, "deltaY": -120, "deltaZ": 0, "deltaMode": 1,
		"key": "a", "code": "KeyA", "repeat": true, "isComposing": false,
		"ctrlKey": true, "shiftKey": false, "altKey": true, "metaKey": true,
		"touches": [{"identifier": 5, "clientX": 1.5, "clientY": 2.5}],
		"changedTouches": [{"identifier": 6, "clientX": 9, "clientY": 8}]
	}`)
	switch {
	case got.ClientX() != 10 || got.ClientY() != 20 || got.OffsetX() != 3 || got.OffsetY() != 4:
		t.Fatalf("position: %+v", got.Data)
	case got.Button() != 2 || got.Buttons() != 3:
		t.Fatal("buttons")
	case got.PointerID() != 7 || got.PointerType() != "pen" || got.Pressure() != 0.5:
		t.Fatal("pointer")
	case got.DeltaY() != -120 || got.DeltaMode() != 1 || got.DeltaX() != 1:
		t.Fatal("wheel")
	case got.Key() != "a" || got.Code() != "KeyA" || !got.Repeat() || got.IsComposing():
		t.Fatal("keyboard")
	case !got.CtrlKey() || got.ShiftKey() || !got.AltKey() || !got.MetaKey():
		t.Fatal("modifiers")
	}
	if ts := got.Touches(); len(ts) != 1 || ts[0].ID != 5 || ts[0].ClientX != 1.5 {
		t.Fatalf("touches: %+v", ts)
	}
	if ts := got.ChangedTouches(); len(ts) != 1 || ts[0].ID != 6 {
		t.Fatalf("changedTouches: %+v", ts)
	}
	// A hand-built EventData (tests, custom dispatch) tolerates control calls.
	core.EventData{}.PreventDefault()
	core.EventData{}.StopPropagation()
}
