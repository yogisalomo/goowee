package ssr_test

import (
	"errors"
	"fmt"
	"html"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/ssr"
)

type label string
type level int8
type point struct{ X, Y int }

func (p point) String() string { return fmt.Sprintf("(%d,%d)", p.X, p.Y) }

// paritySignal binds a signal of v's type as text and as an attribute,
// renders it on the server holding v, and on the client starting from the
// zero value and then set to v (the update path, which formats typed values
// without fmt, #126/#136). Both must write fmt's %v text.
func checkParity[T any](t *testing.T, v T) {
	t.Helper()
	want := fmt.Sprint(v)
	view := func(sig *core.Signal[T]) core.Node {
		return h.Span(h.ID("v"), h.BindAttr("data-v", sig), h.TextS(sig))
	}
	body, _ := ssr.New().Render(view(core.NewSignal(v)))
	esc := html.EscapeString(want)
	if !strings.Contains(body, `data-v="`+strings.ReplaceAll(esc, "&#39;", "'")+`"`) && !strings.Contains(body, `data-v="`+esc+`"`) {
		t.Errorf("%T %v: server attribute not %q in %s", v, v, want, body)
	}
	var zero T
	sig := core.NewSignal(zero)
	s := gooweetest.Render(t, view(sig))
	sig.Set(v)
	s.Flush()
	el := s.Find("#v")
	if got := el.Attr("data-v"); got != want {
		t.Errorf("%T %v: client attribute %q, want %q", v, v, got, want)
	}
	if got := el.Text(); got != want {
		t.Errorf("%T %v: client text %q, want %q", v, v, got, want)
	}
}

func TestBoundValueParity(t *testing.T) {
	checkParity(t, "héllo <b>")
	checkParity(t, true)
	checkParity(t, -42)
	checkParity(t, math.MaxInt64)
	checkParity(t, int8(-3))
	checkParity(t, int32(70000))
	checkParity(t, uint(7))
	checkParity(t, uint64(math.MaxUint64))
	checkParity(t, 0.1)
	checkParity(t, 1e21)
	checkParity(t, float32(0.1))
	checkParity(t, label("named"))
	checkParity(t, level(-5))
	checkParity(t, point{1, 2})
	checkParity(t, errors.New("boom"))
	checkParity(t, 1500*time.Millisecond)
	checkParity(t, []int{1, 2}) // composite: fmt (host builds always have it)
}
