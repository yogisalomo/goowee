package core

import (
	"math"

	"github.com/yogisalomo/goowee/internal/jsontext"
)

// EventData is what a handler receives: the event type, the node id of the
// element whose handler is running, and the event's payload. The typed
// accessors (Key, ClientX, Value, …) return zero values for fields the event
// type doesn't carry; Field and Fields read any other.
//
// An event from the browser carries its payload as the JSON text the runtime
// sent, and each accessor reads its field from it without decoding the rest
// or allocating (#135). Data is set only on EventData built with
// NewEventData (tests, custom dispatchers); read the payload through the
// accessors or Fields, which handle both.
type EventData struct {
	Type   string
	Target int
	Data   map[string]any // the payload, when built from a map; see Fields

	raw string        // the payload as JSON text, from the browser
	ctl *eventControl // set by the dispatcher; nil in hand-built EventData
	gen uint64        // ctl's generation for this event: a reused ctl ignores stale calls
}

// eventControl carries the decisions a handler makes while it runs.
type eventControl struct {
	preventDefault  bool
	stopPropagation bool
	gen             uint64 // bumped by Reset
}

// NewEventData builds the EventData a dispatcher hands to a handler, with
// PreventDefault/StopPropagation wired up; read the decisions back with
// Decisions after the handler returns.
func NewEventData(typ string, target int, data map[string]any) EventData {
	return EventData{Type: typ, Target: target, Data: data, ctl: &eventControl{}}
}

// NewEventDataJSON is NewEventData for a payload that is JSON text (an
// object), as the browser runtime sends it: nothing is decoded until a
// handler reads a field. ctl, if not nil, is where the handler's decisions go;
// a dispatcher can reuse one from event to event (ResetControl).
func NewEventDataJSON(typ string, target int, payload string, ctl *EventControl) EventData {
	if ctl == nil {
		ctl = &eventControl{}
	}
	return EventData{Type: typ, Target: target, raw: payload, ctl: ctl, gen: ctl.gen}
}

// EventControl holds a handler's decisions (PreventDefault,
// StopPropagation); a dispatcher may reuse one across events.
type EventControl = eventControl

// Reset clears the decisions for the next event. An EventData of an earlier
// event (kept by a handler past its return) no longer reaches this control.
func (c *EventControl) Reset() { *c = eventControl{gen: c.gen + 1} }

// Field returns the payload's member k as encoding/json would decode it into
// an any (a string, float64, bool, []any, map[string]any or nil), and
// whether the payload has it.
func (e EventData) Field(k string) (any, bool) {
	if e.Data != nil || e.raw == "" {
		v, ok := e.Data[k]
		return v, ok
	}
	text, ok := jsontext.Field(e.raw, k)
	if !ok {
		return nil, false
	}
	v, err := jsontext.Decode(text)
	return v, err == nil
}

// Number, String and Bool read a payload member of that type by name, for
// fields without an accessor of their own (a custom event's detail, say):
// zero when the payload doesn't have it, or it is of another type.
func (e EventData) Number(k string) float64 { return e.num(k) }
func (e EventData) String(k string) string  { return e.str(k) }
func (e EventData) Bool(k string) bool      { return e.flag(k) }

// Fields returns the whole payload as a map, decoding it if it came as JSON
// text. Prefer the accessors and Field for a field or two: they don't decode
// the rest.
func (e EventData) Fields() map[string]any {
	if e.Data != nil || e.raw == "" {
		return e.Data
	}
	m, _ := jsontext.DecodeObject(e.raw)
	return m
}

// PreventDefault cancels the browser's default action for this event — a link
// navigation, a form submission, a key's text input — decided while the
// handler runs:
//
//	OnKeyDownE(func(e core.EventData) {
//	    if e.Key() == "Enter" && !e.ShiftKey() {
//	        e.PreventDefault() // send instead of inserting a newline
//	        send()
//	    }
//	})
//
// Coalesced high-frequency events (scroll, pointermove) are delivered after
// the fact and can't be prevented.
func (e EventData) PreventDefault() {
	if e.ctl != nil && e.ctl.gen == e.gen {
		e.ctl.preventDefault = true
	}
}

// StopPropagation stops the event from reaching handlers on ancestor elements.
func (e EventData) StopPropagation() {
	if e.ctl != nil && e.ctl.gen == e.gen {
		e.ctl.stopPropagation = true
	}
}

// Decisions reports what the handler decided (for dispatchers).
func (e EventData) Decisions() (preventDefault, stopPropagation bool) {
	if e.ctl == nil {
		return false, false
	}
	return e.ctl.preventDefault, e.ctl.stopPropagation
}

func (e EventData) str(k string) string {
	if e.raw != "" && e.Data == nil {
		text, _ := jsontext.Field(e.raw, k)
		s, _ := jsontext.String(text)
		return s
	}
	return mapStr(e.Data, k)
}

func (e EventData) num(k string) float64 {
	if e.raw != "" && e.Data == nil {
		text, _ := jsontext.Field(e.raw, k)
		f, _ := jsontext.Number(text)
		return f
	}
	return mapNum(e.Data, k)
}

func (e EventData) flag(k string) bool {
	if e.raw != "" && e.Data == nil {
		text, _ := jsontext.Field(e.raw, k)
		return text == "true"
	}
	v, _ := e.Data[k].(bool)
	return v
}

// list is the payload's array member k.
func (e EventData) list(k string) []any {
	v, _ := e.Field(k)
	a, _ := v.([]any)
	return a
}

func mapStr(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func mapNum(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

// Form and input values.
func (e EventData) Value() string     { return e.str("value") }
func (e EventData) Checked() bool     { return e.flag("checked") }
func (e EventData) InputType() string { return e.str("inputType") }

// ValueAsNumber is an input's valueAsNumber (a number input, range slider or
// similar). An empty or non-numeric field, and other event types, report 0.
// Use ValueAsNumberOK to distinguish that from a real zero.
func (e EventData) ValueAsNumber() float64 {
	n, _ := e.ValueAsNumberOK()
	return n
}

// ValueAsNumberOK reports an input's valueAsNumber and whether the field held
// a valid number. In the browser an empty or partial number input ("" , "-",
// "1e") has valueAsNumber NaN; JSON turns that into null, so the field is
// indistinguishable from a missing payload without this check. Bindings use
// ok == false to keep the last valid signal value and leave the user's text
// alone while they are still typing.
func (e EventData) ValueAsNumberOK() (float64, bool) {
	v, _ := e.Field("valueAsNumber")
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n)
	case float32:
		f := float64(n)
		return f, !math.IsNaN(f)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}

// SelectedValues lists the selected option values of a <select>. For a single
// select it has at most one item; for a multiple select, all selected options.
// Other event types return nil.
func (e EventData) SelectedValues() []string {
	raw := e.list("selected")
	if raw == nil {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Clipboard and drag and drop: the event's DataTransfer (clipboardData for
// paste, cut and copy; dataTransfer for drag events). Files (see File)
// carries the files a paste or drop brings.

// DataText is the plain text a clipboard or drop event carries: the text a
// paste or drop inserts, or the selection a cut or copy takes. "" for other
// events.
func (e EventData) DataText() string { return e.str("text") }

// DataTypes lists the formats a paste or drag event carries, as the browser
// names them: "text/plain", "text/html", and "Files" when files are attached.
// During dragenter and dragover the files themselves aren't readable yet, so
// a drop zone checks for "Files" here to decide whether to accept the drop
// (PreventDefault on dragover):
//
//	On("dragover", func(e core.EventData) {
//	    if slices.Contains(e.DataTypes(), "Files") {
//	        e.PreventDefault() // allow dropping files here
//	    }
//	})
func (e EventData) DataTypes() []string {
	raw := e.list("types")
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		if s, ok := t.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Keyboard.
func (e EventData) Key() string       { return e.str("key") }
func (e EventData) Code() string      { return e.str("code") }
func (e EventData) Repeat() bool      { return e.flag("repeat") }
func (e EventData) IsComposing() bool { return e.flag("isComposing") }

// Modifier keys (keyboard, mouse, pointer, wheel, touch events).
func (e EventData) CtrlKey() bool  { return e.flag("ctrlKey") }
func (e EventData) ShiftKey() bool { return e.flag("shiftKey") }
func (e EventData) AltKey() bool   { return e.flag("altKey") }
func (e EventData) MetaKey() bool  { return e.flag("metaKey") }

// Mouse and pointer position and buttons. Button is the button that changed
// (0 primary, 1 middle, 2 secondary); Buttons is the bitmask held down.
func (e EventData) ClientX() float64 { return e.num("clientX") }
func (e EventData) ClientY() float64 { return e.num("clientY") }
func (e EventData) OffsetX() float64 { return e.num("offsetX") }
func (e EventData) OffsetY() float64 { return e.num("offsetY") }
func (e EventData) Button() int      { return int(e.num("button")) }
func (e EventData) Buttons() int     { return int(e.num("buttons")) }

// Pointer events.
func (e EventData) PointerID() int      { return int(e.num("pointerId")) }
func (e EventData) PointerType() string { return e.str("pointerType") } // "mouse", "pen", "touch"
func (e EventData) Pressure() float64   { return e.num("pressure") }

// Wheel events. DeltaMode: 0 pixels, 1 lines, 2 pages.
func (e EventData) DeltaX() float64 { return e.num("deltaX") }
func (e EventData) DeltaY() float64 { return e.num("deltaY") }
func (e EventData) DeltaZ() float64 { return e.num("deltaZ") }
func (e EventData) DeltaMode() int  { return int(e.num("deltaMode")) }

// Scroll position of the scrolled element.
func (e EventData) ScrollTop() float64  { return e.num("scrollTop") }
func (e EventData) ScrollLeft() float64 { return e.num("scrollLeft") }

// Media events (play, pause, timeupdate, ended, seeked, volumechange,
// progress, loadedmetadata, …, and load/error on <video> and <audio>) carry
// the element's playback state at the moment the event fired, so a handler
// needn't read it back through a Ref:
//
//	Video(Src("/intro.mp4"), OnTimeUpdate(func(e core.EventData) {
//	    setAt(e.CurrentTime())
//	}))
func (e EventData) CurrentTime() float64  { return e.num("currentTime") }
func (e EventData) Paused() bool          { return e.flag("paused") }
func (e EventData) Ended() bool           { return e.flag("ended") }
func (e EventData) Seeking() bool         { return e.flag("seeking") }
func (e EventData) Muted() bool           { return e.flag("muted") }
func (e EventData) Volume() float64       { return e.num("volume") }
func (e EventData) PlaybackRate() float64 { return e.num("playbackRate") }

// ReadyState is the media element's readyState: 0 (nothing loaded) to 4
// (enough to play through).
func (e EventData) ReadyState() int { return int(e.num("readyState")) }

// Duration is the media's length in seconds: 0 while it isn't known yet
// (before loadedmetadata), and +Inf for a live stream.
func (e EventData) Duration() float64 {
	v, _ := e.Field("duration")
	switch d := v.(type) {
	case float64:
		return d
	case string:
		if d == "Infinity" {
			return math.Inf(1)
		}
	}
	return 0
}

// TimeRange is one buffered span of a media element, in seconds.
type TimeRange struct{ Start, End float64 }

// Buffered is the media's buffered ranges, for a progress bar's loaded part
// (the progress event fires as it grows).
func (e EventData) Buffered() []TimeRange {
	raw := e.list("buffered")
	out := make([]TimeRange, 0, len(raw))
	for _, item := range raw {
		if r, ok := item.([]any); ok && len(r) == 2 {
			out = append(out, TimeRange{toFloat(r[0]), toFloat(r[1])})
		}
	}
	return out
}

// MediaError is the media element's error on an error event: the
// MediaError code (1 aborted, 2 network, 3 decode, 4 source not supported)
// and the browser's message. code is 0 when there is no media error.
func (e EventData) MediaError() (code int, message string) {
	return int(e.num("errorCode")), e.str("errorMessage")
}

// NewState and OldState are a toggle or beforetoggle event's states, "open"
// or "closed" (a <details> element or a popover).
func (e EventData) NewState() string { return e.str("newState") }
func (e EventData) OldState() string { return e.str("oldState") }

// ReturnValue is a <dialog>'s returnValue on close or cancel: the value of
// the button that closed a method="dialog" form, or what close(v) was given.
func (e EventData) ReturnValue() string { return e.str("returnValue") }

// PropertyName is the CSS property a transition event is about, and
// AnimationName the @keyframes name of an animation event. ElapsedTime is
// the seconds the transition or animation had run, and PseudoElement is
// "::before"/"::after" when it ran on one, else "".
func (e EventData) PropertyName() string  { return e.str("propertyName") }
func (e EventData) AnimationName() string { return e.str("animationName") }
func (e EventData) ElapsedTime() float64  { return e.num("elapsedTime") }
func (e EventData) PseudoElement() string { return e.str("pseudoElement") }

// NaturalWidth and NaturalHeight are an <img>'s intrinsic size, on load.
func (e EventData) NaturalWidth() float64  { return e.num("naturalWidth") }
func (e EventData) NaturalHeight() float64 { return e.num("naturalHeight") }

// InputData is the text an input or beforeinput event inserts, or the text
// being composed on compositionstart/update/end (an IME). It is "" for
// deletions and formatting (see InputType).
func (e EventData) InputData() string { return e.str("data") }

// Measurement is one row size reported by the runtime's ResizeObserver in a
// goowee:rowmeasure event. Index is the row's position in the list at the time
// it was measured.
type Measurement struct {
	Index  int
	Width  float64
	Height float64
}

// Measurements returns the batched row sizes a goowee:rowmeasure event carries.
func (e EventData) Measurements() []Measurement {
	// A browser event carries its payload as JSON text (#200); Field reads the
	// one member we need in place, while hand-built test events use Data.
	rows, _ := e.Field("rows")
	raw, _ := rows.([]any)
	out := make([]Measurement, 0, len(raw))
	for _, item := range raw {
		vals, ok := item.([]any)
		if !ok || len(vals) < 3 {
			continue
		}
		out = append(out, Measurement{
			Index:  int(toFloat(vals[0])),
			Width:  toFloat(vals[1]),
			Height: toFloat(vals[2]),
		})
	}
	return out
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// Touch is one point of contact in a touch event.
type Touch struct {
	ID               int
	ClientX, ClientY float64
}

// Touches lists the points currently touching the surface (touch events).
func (e EventData) Touches() []Touch { return e.touchList("touches") }

// ChangedTouches lists the points that changed in this touch event.
func (e EventData) ChangedTouches() []Touch { return e.touchList("changedTouches") }

func (e EventData) touchList(k string) []Touch {
	raw := e.list(k)
	if raw == nil {
		return nil
	}
	out := make([]Touch, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, Touch{ID: int(mapNum(m, "identifier")), ClientX: mapNum(m, "clientX"), ClientY: mapNum(m, "clientY")})
		}
	}
	return out
}

// FormValues returns a submitted form's named fields.
func (e EventData) FormValues() map[string]string {
	out := map[string]string{}
	if v, _ := e.Field("values"); v != nil {
		m, _ := v.(map[string]any)
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
