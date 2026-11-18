package canvas_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/ssr"
)

func fillView(color *core.Signal[string], runs *int) core.Node {
	return canvas.View(
		canvas.Fill(),
		canvas.Attrs(h.Class("fill")),
		canvas.Draw([]core.SignalAccessor{color}, func(c *canvas.Ctx) {
			*runs++
			c.FillStyle(color.Get())
			c.FillRect(0, 0, c.Width(), c.Height())
		}),
	)
}

// A Fill canvas draws nothing until it has a size; then it draws at that
// size, and again when the size or a dep changes.
func TestFillDrawsAtTheElementsSize(t *testing.T) {
	color := core.NewSignal("red")
	runs := 0
	s := gooweetest.Render(t, fillView(color, &runs))
	cv := s.Find("canvas.fill")
	if runs != 0 || cv.DrawCount() != 1 || len(cv.Canvas()) != 0 {
		t.Fatalf("before layout: runs=%d DrawCount=%d cmds=%v; want 0 runs and one empty buffer", runs, cv.DrawCount(), cv.Canvas())
	}
	if cv.Attr("style") != "width:100%;height:100%;display:block" {
		t.Fatalf("style %q", cv.Attr("style"))
	}

	s.ResizeCanvas(cv, 300, 200)
	if got := commands(cv.Canvas()); got != `FillStyle("red") FillRect(0, 0, 300, 200)` {
		t.Fatalf("after layout: %s", got)
	}
	s.ResizeCanvas(cv, 400, 200)
	if got := commands(cv.Canvas()); got != `FillStyle("red") FillRect(0, 0, 400, 200)` {
		t.Fatalf("after a resize: %s", got)
	}
	s.ResizeCanvas(cv, 400, 200) // the same size: no redraw
	color.Set("blue")
	s.Flush()
	if got := commands(cv.Canvas()); got != `FillStyle("blue") FillRect(0, 0, 400, 200)` {
		t.Fatalf("after a dep changed: %s", got)
	}
	if runs != 3 {
		t.Fatalf("draw function ran %d times, want 3", runs)
	}
}

// The buffers a Fill canvas sends carry the follow-size flag, which tells
// the runtime to watch the element.
func TestFillSetsTheFollowFlag(t *testing.T) {
	var bufs [][]uint32
	s := gooweetest.Render(t, canvas.View(canvas.Fill(), canvas.NoClear(), canvas.Draw(nil, func(c *canvas.Ctx) {
		bufs = append(bufs, append([]uint32(nil), c.Commands()...))
	})))
	s.ResizeCanvas(s.Find("canvas"), 10, 20)
	if len(bufs) != 1 {
		t.Fatalf("%d draws, want 1", len(bufs))
	}
	w, hh, flags, _, err := canvas.Decode(bufs[0])
	if err != nil || w != 10 || hh != 20 || flags != canvas.FlagFollowSize {
		t.Fatalf("w=%v h=%v flags=%b err=%v", w, hh, flags, err)
	}
}

func TestFillOnTheServer(t *testing.T) {
	runs := 0
	body, _ := ssr.New().Render(fillView(core.NewSignal("red"), &runs))
	if !strings.Contains(body, `style="width:100%;height:100%;display:block"`) || strings.Contains(body, "width=") || runs != 0 {
		t.Fatalf("runs=%d body %s", runs, body)
	}
}

func TestViewPanicsOnConflictingOptions(t *testing.T) {
	draw := canvas.Draw(nil, func(*canvas.Ctx) {})
	for want, opts := range map[string][]canvas.Option{
		"Size or Fill, not both":    {canvas.Size(1, 1), canvas.Fill(), draw},
		"Draw or Animate, not both": {canvas.Size(1, 1), draw, canvas.Animate(func(*canvas.Ctx, time.Duration) {})},
		"needs a Size":              {draw},
	} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, want) {
					t.Errorf("want a panic with %q, got %q", want, r)
				}
			}()
			canvas.View(opts...)
		}()
	}
}
