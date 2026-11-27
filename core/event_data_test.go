package core

import (
	"math"
	"testing"
)

// The accessors for the event families added in #210 read the JSON text the
// runtime sends (buildPayload in runtime/goowee.js) and hand-built Data alike.
func TestMediaEventData(t *testing.T) {
	const payload = `{"currentTime":12.5,"duration":60,"paused":false,"ended":false,"seeking":false,` +
		`"muted":true,"volume":0.5,"playbackRate":1.5,"readyState":4,"buffered":[[0,20.25],[30,45]]}`
	for name, e := range map[string]EventData{
		"json": NewEventDataJSON("timeupdate", 1, payload, nil),
		"data": NewEventData("timeupdate", 1, map[string]any{
			"currentTime": 12.5, "duration": 60.0, "paused": false, "muted": true, "volume": 0.5,
			"playbackRate": 1.5, "readyState": 4.0, "buffered": []any{[]any{0.0, 20.25}, []any{30.0, 45.0}},
		}),
	} {
		t.Run(name, func(t *testing.T) {
			if e.CurrentTime() != 12.5 || e.Duration() != 60 || e.Paused() || e.Ended() || e.Seeking() {
				t.Errorf("position: %v / %v paused=%v ended=%v seeking=%v",
					e.CurrentTime(), e.Duration(), e.Paused(), e.Ended(), e.Seeking())
			}
			if !e.Muted() || e.Volume() != 0.5 || e.PlaybackRate() != 1.5 || e.ReadyState() != 4 {
				t.Errorf("muted=%v volume=%v rate=%v ready=%v", e.Muted(), e.Volume(), e.PlaybackRate(), e.ReadyState())
			}
			b := e.Buffered()
			if len(b) != 2 || b[0] != (TimeRange{0, 20.25}) || b[1] != (TimeRange{30, 45}) {
				t.Errorf("buffered = %v", b)
			}
			if code, msg := e.MediaError(); code != 0 || msg != "" {
				t.Errorf("MediaError = %d %q, want none", code, msg)
			}
		})
	}
}

func TestMediaDuration(t *testing.T) {
	for payload, want := range map[string]float64{
		`{"duration":null}`:       0, // NaN in the browser: not known yet
		`{}`:                      0,
		`{"duration":"Infinity"}`: math.Inf(1),
		`{"duration":3.25}`:       3.25,
	} {
		if got := NewEventDataJSON("durationchange", 1, payload, nil).Duration(); got != want {
			t.Errorf("%s: Duration = %v, want %v", payload, got, want)
		}
	}
}

func TestMediaErrorData(t *testing.T) {
	e := NewEventDataJSON("error", 1, `{"currentTime":0,"errorCode":4,"errorMessage":"MEDIA_ELEMENT_ERROR: Format error"}`, nil)
	if code, msg := e.MediaError(); code != 4 || msg != "MEDIA_ELEMENT_ERROR: Format error" {
		t.Errorf("MediaError = %d %q", code, msg)
	}
}

func TestOtherEventData(t *testing.T) {
	cases := []struct {
		typ, payload string
		check        func(EventData) bool
	}{
		{"toggle", `{"newState":"open","oldState":"closed"}`,
			func(e EventData) bool { return e.NewState() == "open" && e.OldState() == "closed" }},
		{"close", `{"returnValue":"confirm"}`,
			func(e EventData) bool { return e.ReturnValue() == "confirm" }},
		{"transitionend", `{"propertyName":"opacity","elapsedTime":0.3,"pseudoElement":""}`,
			func(e EventData) bool {
				return e.PropertyName() == "opacity" && e.ElapsedTime() == 0.3 && e.PseudoElement() == ""
			}},
		{"animationend", `{"animationName":"fade-in","elapsedTime":1,"pseudoElement":"::after"}`,
			func(e EventData) bool {
				return e.AnimationName() == "fade-in" && e.ElapsedTime() == 1 && e.PseudoElement() == "::after"
			}},
		{"load", `{"naturalWidth":640,"naturalHeight":480}`,
			func(e EventData) bool { return e.NaturalWidth() == 640 && e.NaturalHeight() == 480 }},
		{"scrollend", `{"scrollTop":120,"scrollLeft":0}`,
			func(e EventData) bool { return e.ScrollTop() == 120 }},
		{"beforeinput", `{"data":"é","inputType":"insertText","isComposing":false}`,
			func(e EventData) bool { return e.InputData() == "é" && e.InputType() == "insertText" }},
		{"compositionend", `{"data":"日本"}`,
			func(e EventData) bool { return e.InputData() == "日本" }},
		{"input", `{"value":"a","inputType":"deleteContentBackward","isComposing":false,"data":null}`,
			func(e EventData) bool { return e.InputData() == "" && e.Value() == "a" }},
	}
	for _, c := range cases {
		if !c.check(NewEventDataJSON(c.typ, 1, c.payload, nil)) {
			t.Errorf("%s %s: accessors don't match the payload", c.typ, c.payload)
		}
	}
}
