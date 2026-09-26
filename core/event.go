package core

// EventData is what a handler receives: the event type, the node id of the
// element whose handler is running, and the event's payload (Data) as decoded
// from the browser. The typed accessors return zero values for fields the
// event type doesn't carry.
type EventData struct {
	Type   string
	Target int
	Data   map[string]any

	ctl *eventControl // set by the dispatcher; nil in hand-built EventData
}

// eventControl carries the decisions a handler makes while it runs.
type eventControl struct {
	preventDefault  bool
	stopPropagation bool
}

// NewEventData builds the EventData a dispatcher hands to a handler, with
// PreventDefault/StopPropagation wired up; read the decisions back with
// Decisions after the handler returns.
func NewEventData(typ string, target int, data map[string]any) EventData {
	return EventData{Type: typ, Target: target, Data: data, ctl: &eventControl{}}
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
	if e.ctl != nil {
		e.ctl.preventDefault = true
	}
}

// StopPropagation stops the event from reaching handlers on ancestor elements.
func (e EventData) StopPropagation() {
	if e.ctl != nil {
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

func (e EventData) str(k string) string  { return mapStr(e.Data, k) }
func (e EventData) num(k string) float64 { return mapNum(e.Data, k) }
func (e EventData) flag(k string) bool {
	v, _ := e.Data[k].(bool)
	return v
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
	raw, ok := e.Data[k].([]any)
	if !ok {
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
	if m, ok := e.Data["values"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
