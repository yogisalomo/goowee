package app

import (
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

// eventsPage demonstrates the event model: bubbling and StopPropagation,
// events that don't bubble (mouseenter, invalid), a handler that decides to
// prevent the default action, and event data (wheel deltas).
func eventsPage(r *router.Router) core.Node {
	return core.Component("EventsPage", func() core.Node {
		cardClicks, setCardClicks := hooks.UseState(0)
		innerClicks, setInnerClicks := hooks.UseState(0)
		stopped, setStopped := hooks.UseState(0)
		hovering, setHovering := hooks.UseState(false)
		invalids, setInvalids := hooks.UseState(0)
		draft, setDraft := hooks.UseState("")
		sent, setSent := hooks.UseState("")
		wheel, setWheel := hooks.UseState(0.0)

		hoverText := core.Computed([]core.SignalAccessor{hovering}, func() string {
			if hovering.Get() {
				return "Pointer inside: yes"
			}
			return "Pointer inside: no"
		})

		demo := Div(
			Div(Class("demo-row event-card"),
				OnClick(func() { setCardClicks(cardClicks.Get() + 1) }),
				Button(Class("inner"), OnClick(func() { setInnerClicks(innerClicks.Get() + 1) }), Text("Inner button")),
				Button(Class("stopper"), OnClickE(func(e core.EventData) {
					e.StopPropagation()
					setStopped(stopped.Get() + 1)
				}), Text("Stops propagation")),
			),
			P(Class("event-counts"), Textf("Card clicks: %d · inner: %d · stopped: %d", cardClicks, innerClicks, stopped)),

			Div(Class("demo-row hover-zone"),
				OnMouseEnter(func() { setHovering(true) }),
				OnMouseLeave(func() { setHovering(false) }),
				TextS(hoverText)),

			Form(Class("demo-row"), OnSubmit(func(map[string]string) {}),
				Input(Class("needs-email"), Name("email"), Type("email"), Placeholder("email (required)"), Required(true),
					OnInvalid(func() { setInvalids(invalids.Get() + 1) })),
				Button(Type("submit"), Text("Submit")),
			),
			P(Class("invalid-count"), Textf("Invalid events: %d", invalids)),

			Textarea(Class("enter-send"), Placeholder("Type, then Enter to send (Shift+Enter: newline)"),
				BindValue(draft),
				OnKeyDownE(func(e core.EventData) {
					if e.Key() == "Enter" && !e.ShiftKey() {
						e.PreventDefault() // decided per keystroke, not per handler
						setSent(draft.Get())
						setDraft("")
					}
				})),
			P(Class("sent"), Textf("Sent: %s", sent)),

			Div(Class("demo-row wheel-zone"),
				OnWheel(func(e core.EventData) { setWheel(e.DeltaY()) }),
				Textf("Wheel deltaY: %v", wheel)),
		)
		return lessonLayout(r, "Events",
			"Handlers bubble from the element you interact with up through its ancestors, like the DOM. A handler can stop that, or cancel the browser's default action, while it runs.",
			demo, "events.go", eventsCode,
			howItWorks(
				"Clicking the inner button runs its handler, then the card's: handlers bubble innermost-first.",
				"e.StopPropagation() inside a handler keeps the event from reaching ancestors — the card doesn't count that click.",
				"mouseenter/mouseleave and invalid don't bubble; goowee listens for them in the capture phase and delivers them to the element itself.",
				"e.PreventDefault() is decided per event: Enter sends (no newline), Shift+Enter still inserts one.",
				"Event data comes typed on core.EventData: e.DeltaY(), e.ClientX(), e.PointerType(), e.Key(), e.CtrlKey(), …",
			),
			tutorialStepNav(r, "/events"),
		)
	})
}

const eventsCode = `Div(OnClick(func() { cardClicks++ }),          // runs second (bubbling)
    Button(OnClick(func() { innerClicks++ })),  // runs first
    Button(OnClickE(func(e core.EventData) {
        e.StopPropagation()                     // the card never sees it
    })),
)

Div(OnMouseEnter(enter), OnMouseLeave(leave))   // non-bubbling: target only

Input(Required(true), OnInvalid(func() { invalids++ }))

Textarea(OnKeyDownE(func(e core.EventData) {
    if e.Key() == "Enter" && !e.ShiftKey() {
        e.PreventDefault()                      // decided while handling
        send()
    }
}))

Div(OnWheel(func(e core.EventData) { setWheel(e.DeltaY()) }))`
