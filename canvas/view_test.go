package canvas_test

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/ssr"
)

func commands(cmds []canvas.Command) string {
	var s []string
	for _, c := range cmds {
		s = append(s, c.String())
	}
	return strings.Join(s, " ")
}

func bars(values *core.Signal[[]float64], runs *int) core.Node {
	return canvas.View(
		canvas.Size(60, 40),
		canvas.Label("Bars"),
		canvas.Attrs(h.Class("chart")),
		canvas.Fallback(h.P(h.Text("no canvas"))),
		canvas.Draw([]core.SignalAccessor{values}, func(c *canvas.Ctx) {
			*runs++
			c.FillStyle("#00add8")
			for i, v := range values.Get() {
				c.FillRect(float64(i)*20, c.Height()-v, 16, v)
			}
		}),
	)
}

func TestViewRedrawsWhenADepChanges(t *testing.T) {
	values := core.NewSignal([]float64{10, 30})
	runs := 0
	s := gooweetest.Render(t, bars(values, &runs))
	cv := s.Find("canvas.chart")

	if got, want := commands(cv.Canvas()), `FillStyle("#00add8") FillRect(0, 30, 16, 10) FillRect(20, 10, 16, 30)`; got != want {
		t.Fatalf("first draw:\n got %s\nwant %s", got, want)
	}
	values.Set([]float64{40}) // the View records the new picture into the same buffer now…
	if got := commands(cv.Canvas()); got != `FillStyle("#00add8") FillRect(0, 30, 16, 10) FillRect(20, 10, 16, 30)` {
		t.Fatalf("the screen shows a picture before its frame: %s", got) // …so the screen must hold a copy
	}
	s.Flush()
	if got, want := commands(cv.Canvas()), `FillStyle("#00add8") FillRect(0, 0, 16, 40)`; got != want {
		t.Fatalf("after Set:\n got %s\nwant %s", got, want)
	}
	if cv.DrawCount() != 2 || runs != 2 {
		t.Fatalf("DrawCount = %d, runs = %d, want 2 and 2", cv.DrawCount(), runs)
	}
	if cv.Attr("style") != "width:60px;height:40px" || cv.Attr("role") != "img" || cv.Attr("aria-label") != "Bars" {
		t.Fatalf("canvas markup: %s", cv.HTML())
	}

	s.Unmount()
	values.Set([]float64{1})
	if runs != 2 {
		t.Fatalf("drew after unmount (%d runs)", runs)
	}
}

// Two writes before a frame draw once: the later buffer replaces the earlier.
func TestViewSendsOneDrawPerFrame(t *testing.T) {
	values := core.NewSignal([]float64{1})
	runs := 0
	s := gooweetest.Render(t, bars(values, &runs))
	values.Set([]float64{2})
	values.Set([]float64{3})
	s.Flush()
	cv := s.Find("canvas")
	if cv.DrawCount() != 2 || !strings.Contains(commands(cv.Canvas()), "FillRect(0, 37, 16, 3)") {
		t.Fatalf("DrawCount = %d, last = %s", cv.DrawCount(), commands(cv.Canvas()))
	}
}

func TestViewHeaderAndNoClear(t *testing.T) {
	for _, tc := range []struct {
		opts  []canvas.Option
		flags uint32
	}{
		{nil, canvas.FlagClear},
		{[]canvas.Option{canvas.NoClear()}, 0},
	} {
		var buf []uint32
		opts := append([]canvas.Option{
			canvas.Size(12.5, 7),
			canvas.Draw(nil, func(c *canvas.Ctx) { c.Fill(); buf = c.Commands() }),
		}, tc.opts...)
		gooweetest.Render(t, canvas.View(opts...))
		w, h, flags, cmds, err := canvas.Decode(buf)
		if err != nil || w != 12.5 || h != 7 || flags != tc.flags || len(cmds) != 1 {
			t.Fatalf("w=%v h=%v flags=%v cmds=%v err=%v", w, h, flags, cmds, err)
		}
	}
}

func TestViewPanicsWithoutSizeOrDraw(t *testing.T) {
	for name, opts := range map[string][]canvas.Option{
		"Size": {canvas.Draw(nil, func(*canvas.Ctx) {})},
		"Draw": {canvas.Size(1, 1)},
	} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, "needs a "+name) {
					t.Errorf("without %s: panic %q", name, r)
				}
			}()
			canvas.View(opts...)
		}()
	}
}

// The server renders the element, label and fallback; the draw function is
// client-only.
func TestViewOnTheServer(t *testing.T) {
	values := core.NewSignal([]float64{1})
	runs := 0
	body, _ := ssr.New().Render(bars(values, &runs))
	for _, want := range []string{
		`<canvas `, `style="width:60px;height:40px"`, `role="img"`, `aria-label="Bars"`, `class="chart"`,
		`no canvas</p></canvas>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	if runs != 0 {
		t.Fatalf("the draw function ran on the server")
	}
}

func TestVersionMatchesCore(t *testing.T) {
	if canvas.Version != core.DrawVersion {
		t.Fatalf("canvas.Version %d != core.DrawVersion %d", canvas.Version, core.DrawVersion)
	}
}

// ref.Draw by hand reaches gooweetest.
func TestRefDrawInGooweetest(t *testing.T) {
	ref := h.Ref()
	c := canvas.NewCtx(10, 10)
	s := gooweetest.Render(t, core.Component("C", func() core.Node {
		return h.Div(h.Canvas(h.RefTo(ref)), h.Button(h.OnClick(func() {
			c.Reset(10, 10)
			c.FillRect(1, 2, 3, 4)
			ref.Draw(c.Commands())
		}), h.Text("draw")))
	}))
	cv := s.Find("canvas")
	if cv.Canvas() != nil || cv.DrawCount() != 0 {
		t.Fatal("drawn before the click")
	}
	s.Click(s.FindByText("draw"))
	if got := commands(cv.Canvas()); got != "FillRect(1, 2, 3, 4)" {
		t.Fatalf("got %q", got)
	}
}
