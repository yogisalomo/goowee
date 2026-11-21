package h_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
)

type todo struct {
	ID   int
	Text string
}

func todoRow(t *core.Signal[todo]) core.Node {
	text := core.Computed([]core.SignalAccessor{t}, func() string { return strconv.Itoa(t.Get().ID) + ":" + t.Get().Text })
	return Li(TextS(text))
}

func items(s *gooweetest.Screen) string {
	var out []string
	for _, li := range s.FindAll("li") {
		out = append(out, li.Text())
	}
	return strings.Join(out, " ")
}

// Every operation, then a full replace, against what a fresh render shows.
func TestForListOperations(t *testing.T) {
	list := core.NewList(func(t todo) int { return t.ID }, todo{1, "a"}, todo{2, "b"})
	s := gooweetest.Render(t, Ul(ForList(list, todoRow)))
	step := func(what, want string, op func()) {
		t.Helper()
		op()
		s.Flush()
		if got := items(s); got != want {
			t.Fatalf("%s: %q, want %q", what, got, want)
		}
		fresh := gooweetest.Render(t, Ul(ForList(core.NewList(list.Key, list.Items()...), todoRow)))
		if s.HTML() != fresh.HTML() {
			t.Fatalf("%s: the document differs from a fresh render:\n got %s\nwant %s", what, s.HTML(), fresh.HTML())
		}
	}
	step("append", "1:a 2:b 3:c", func() { list.Append(todo{3, "c"}) })
	step("insert", "0:z 1:a 2:b 3:c", func() { list.Insert(0, todo{0, "z"}) })
	step("update", "0:z 1:A 2:b 3:c", func() { list.Update(1, func(t *todo) { t.Text = "A" }) })
	step("remove", "0:z 1:A 3:c", func() { list.Remove(2) })
	step("move down", "1:A 3:c 0:z", func() { list.Move(0, 2) })
	step("moves", "0:z 3:c 1:A", func() { list.Move(0, 0); list.Move(3, 2); list.Move(3, 1) })
	step("batched", "9:x 0:z 3:C", func() {
		core.Batch(func() {
			list.Remove(1)
			list.Update(3, func(t *todo) { t.Text = "C" })
			list.Insert(0, todo{9, "x"})
		})
	})
	step("set", "3:C 4:d", func() { list.Set([]todo{{3, "C"}, {4, "d"}}) })
	step("after set", "3:C 4:d 5:e", func() { list.Append(todo{5, "e"}) })
}

// Rows render inside <svg> and portals as their namespace and place say.
func TestForListInSVGAndPortal(t *testing.T) {
	list := core.NewList(func(t todo) int { return t.ID }, todo{1, "a"})
	circle := func(t *core.Signal[todo]) core.Node {
		return El("circle", BindAttr("data-id", core.Computed([]core.SignalAccessor{t}, func() string { return t.Get().Text })))
	}
	s := gooweetest.Render(t, Div(Svg(ForList(list, circle)), Portal("#over", Ul(ForList(list, todoRow)))))
	list.Append(todo{2, "b"})
	list.Update(1, func(t *todo) { t.Text = "A" })
	s.Flush()
	if n := len(s.FindAll("circle")); n != 2 {
		t.Fatalf("%d circles", n)
	}
	if got := s.FindAll("circle")[0].Attr("data-id"); got != "A" {
		t.Fatalf("updated circle: %q", got)
	}
	if got := s.Portal("#over").Text(); got != "1:A2:b" {
		t.Fatalf("portal rows: %q", got)
	}
}
