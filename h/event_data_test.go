package h_test

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	h "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// The helpers added in #210 hand their handlers the event's data, and the
// non-bubbling ones (toggle, close, media) reach the element itself only.
func TestEventDataHelpers(t *testing.T) {
	var parentSaw []string
	var open bool
	var closedWith string
	var at, volume float64
	var ended bool
	s := gooweetest.Render(t, core.Component("App", func() core.Node {
		n, setN := hooks.UseState(0)
		return h.Div(
			h.On("toggle", func(core.EventData) { parentSaw = append(parentSaw, "toggle") }),
			h.On("timeupdate", func(core.EventData) { parentSaw = append(parentSaw, "timeupdate") }),
			h.Details(h.ID("more"), h.OnToggle(func(o bool) { open = o; setN(n.Get() + 1) }),
				h.Summary(h.Text("More"))),
			h.Dialog(h.ID("dlg"), h.OnClose(func(v string) { closedWith = v })),
			h.Video(h.ID("vid"),
				h.OnTimeUpdate(func(e core.EventData) { at = e.CurrentTime() }),
				h.OnVolumeChange(func(e core.EventData) { volume = e.Volume() }),
				h.OnEnded(func(e core.EventData) { ended = e.Ended() })),
			h.P(h.Textf("toggles: %d", n)),
		)
	}))

	s.Dispatch(s.Find("#more"), "toggle", map[string]any{"newState": "open", "oldState": "closed"})
	if !open || !strings.Contains(s.Text(), "toggles: 1") {
		t.Errorf("OnToggle: open=%v, text %q", open, s.Text())
	}
	s.Dispatch(s.Find("#more"), "toggle", map[string]any{"newState": "closed", "oldState": "open"})
	if open {
		t.Error("OnToggle didn't see the close")
	}
	s.Dispatch(s.Find("#dlg"), "close", map[string]any{"returnValue": "confirm"})
	if closedWith != "confirm" {
		t.Errorf("OnClose got %q", closedWith)
	}
	vid := s.Find("#vid")
	s.Dispatch(vid, "timeupdate", map[string]any{"currentTime": 3.5})
	s.Dispatch(vid, "volumechange", map[string]any{"volume": 0.25})
	s.Dispatch(vid, "ended", map[string]any{"ended": true})
	if at != 3.5 || volume != 0.25 || !ended {
		t.Errorf("media: at=%v volume=%v ended=%v", at, volume, ended)
	}
	if len(parentSaw) != 0 {
		t.Errorf("non-bubbling events reached the parent: %v", parentSaw)
	}
}
