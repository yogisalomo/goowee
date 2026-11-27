package app

import (
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

// eventsPage demonstrates the event model: bubbling and StopPropagation,
// events that don't bubble (mouseenter, invalid), a handler that decides to
// prevent the default action, and event data (wheel deltas, a toggle's state).
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
		detailsOpen, setDetailsOpen := hooks.UseState(false)

		// Enter sends; Shift+Enter (and Enter that confirms an IME
		// composition) still types a newline.
		enter := KeyIs("Enter").Without(core.ModShift)

		hoverText := core.Derive(hovering, func(in bool) string {
			if in {
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
					if enter.Match(e) {
						setSent(draft.Get())
						setDraft("")
					}
				}, PreventDefaultWhen(enter))), // the browser checks it, per keystroke
			P(Class("sent"), Textf("Sent: %s", sent)),

			Div(Class("demo-row wheel-zone"),
				OnWheel(func(e core.EventData) { setWheel(e.DeltaY()) }),
				Textf("Wheel deltaY: %v", wheel)),

			Details(Class("demo-row event-details"), OnToggle(setDetailsOpen),
				Summary(Text("Open me")),
				P(Text("toggle doesn't bubble, and its event says which way it went."))),
			P(Class("details-state"), Textf("Details open: %v", detailsOpen)),
		)
		return lessonLayout(r, "Events",
			"Handlers bubble from the element you interact with up through its ancestors, like the DOM. A handler can stop that, or cancel the browser's default action, while it runs.",
			demo, "events.go", eventsCode,
			howItWorks(
				"Clicking the inner button runs its handler, then the card's: handlers bubble innermost-first.",
				"`e.StopPropagation()` inside a handler keeps the event from reaching ancestors — the card doesn't count that click.",
				"`mouseenter`/`mouseleave` and `invalid` don't bubble; goowee listens for them in the capture phase and delivers them to the element itself.",
				"`PreventDefaultWhen(KeyIs(\"Enter\").Without(core.ModShift))` cancels the default per event, decided by the browser before Go runs: Enter sends (no newline), Shift+Enter still inserts one. `e.PreventDefault()` inside a handler works too.",
				"Event data comes typed on `core.EventData`: `e.DeltaY()`, `e.ClientX()`, `e.PointerType()`, `e.Key()`, `e.CtrlKey()`, …",
				"Events beyond the mouse and keyboard carry their data too: a toggle its new state, a dialog's close its returnValue, a video's timeupdate `e.CurrentTime()` and `e.Duration()`.",
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

enter := KeyIs("Enter").Without(core.ModShift)
Textarea(OnKeyDownE(func(e core.EventData) {
    if enter.Match(e) {
        send()
    }
}, PreventDefaultWhen(enter)))                  // decided by the browser, per keystroke

Div(OnWheel(func(e core.EventData) { setWheel(e.DeltaY()) }))

Details(OnToggle(setDetailsOpen), Summary(Text("Open me")))

Video(Src("/intro.mp4"), OnTimeUpdate(func(e core.EventData) {
    setAt(e.CurrentTime())                      // the position when it fired
}))`
