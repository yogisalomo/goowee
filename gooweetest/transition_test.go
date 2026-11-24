package gooweetest_test

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func TestShowLeaveTransition(t *testing.T) {
	s := gooweetest.Render(t, core.Component("ShowTransition", func() core.Node {
		show, setShow := hooks.UseState(true)
		return Div(
			Button(ID("toggle"), OnClick(func() { setShow(!show.Get()) }), Text("toggle")),
			Show(show, func() core.Node {
				return Div(Class("panel"), Transition("fade"), Text("panel"))
			}),
		)
	}))

	if got := s.Find(".panel").Attr("data-goowee-transition"); got != "fade" {
		t.Fatalf("panel transition attr = %q", got)
	}
	s.Click(s.Find("#toggle"))
	if s.Query(".panel") != nil {
		t.Fatalf("gooweetest should remove a leaving node at once: %s", s.HTML())
	}
	trs := s.Transitions()
	if len(trs) != 1 || trs[0].Name != "fade" || trs[0].Tag != "div" {
		t.Fatalf("transitions = %+v", trs)
	}
}

func TestLeaveDisposesStateImmediately(t *testing.T) {
	disposed := 0
	s := gooweetest.Render(t, core.Component("Dispose", func() core.Node {
		show, setShow := hooks.UseState(true)
		return Div(
			Button(ID("hide"), OnClick(func() { setShow(false) }), Text("hide")),
			Show(show, func() core.Node {
				return core.Component("Leaf", func() core.Node {
					hooks.OnMount(func() func() {
						return func() { disposed++ }
					})
					return Div(Transition("fade"), Text("leaf"))
				})
			}),
		)
	}))

	s.Click(s.Find("#hide"))
	if disposed != 1 {
		t.Fatalf("disposers run = %d, want 1", disposed)
	}
	if len(s.Transitions()) != 1 {
		t.Fatalf("transitions = %+v", s.Transitions())
	}
}

func TestForLeaveTransition(t *testing.T) {
	s := gooweetest.Render(t, core.Component("ListTransition", func() core.Node {
		items, setItems := hooks.UseState([]int{1, 2, 3})
		return Div(
			Button(ID("remove"), OnClick(func() { setItems([]int{1, 3}) }), Text("remove")),
			Ul(For(items, func(x int) int { return x }, func(x int) core.Node {
				return Li(Transition("row"), Textf("%d", x))
			})),
		)
	}))

	if got := len(s.FindAll("ul li")); got != 3 {
		t.Fatalf("initial rows = %d", got)
	}
	s.Click(s.Find("#remove"))
	if got := len(s.FindAll("ul li")); got != 2 {
		t.Fatalf("rows after removal = %d", got)
	}
	trs := s.Transitions()
	if len(trs) != 1 || trs[0].Name != "row" || trs[0].Tag != "li" {
		t.Fatalf("transitions = %+v", trs)
	}
}
