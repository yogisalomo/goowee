package h_test

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
)

func card(label string) core.Node {
	return core.Component("Card", func() core.Node { return P(ID("card"), Text(label)) })
}

// #161 (FuzzDiffer): switching branches mounts the new branch's content even
// when both render a component of the same name.
func TestSwitchAndShowElseRemountSameNamedComponents(t *testing.T) {
	v := core.NewSignal(0)
	on := core.NewSignal(true)
	s := gooweetest.Render(t, Div(
		Div(ID("sw"), Switch(v, map[int]func() core.Node{0: func() core.Node { return card("zero") }}, func() core.Node { return card("other") })),
		Div(ID("se"), ShowElse(on, func() core.Node { return card("on") }, func() core.Node { return card("off") })),
	))
	v.Set(3)
	on.Set(false)
	s.Flush()
	if got := s.Find("#sw").Text(); got != "other" {
		t.Fatalf("Switch shows %q, want other", got)
	}
	if got := s.Find("#se").Text(); got != "off" {
		t.Fatalf("ShowElse shows %q, want off", got)
	}
}

type row struct{ K, V int }

// A plain component row remounts when its item changes, also in a For that
// a re-rendered parent row built afresh (no remount history).
func TestNestedForComponentRowsFollowItems(t *testing.T) {
	list := core.NewSignal([]row{{1, 1}, {2, 2}})
	s := gooweetest.Render(t, Ul(For(list, func(r row) int { return r.K }, func(r row) core.Node {
		return Li(Textf("%d", r.V), Ul(For(list, func(x row) int { return x.K }, func(x row) core.Node {
			return core.Component("Row", func() core.Node { return Li(Text(fmt.Sprintf("%d:%d", x.K, x.V))) })
		})))
	})))
	list.Set([]row{{1, 0}, {2, 2}})
	s.Flush()
	if got := s.Text(); got != "01:02:221:02:2" {
		t.Fatalf("got %q", got)
	}
}
