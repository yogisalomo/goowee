package canvas_test

import (
	"testing"
	"time"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/ssr"
)

// Each frame draws one picture; the first gets dt 0; unmounting stops it.
func TestAnimateDrawsOncePerFrame(t *testing.T) {
	frames := 0
	var dts []time.Duration
	s := gooweetest.Render(t, canvas.View(
		canvas.Size(50, 50),
		canvas.Animate(func(c *canvas.Ctx, dt time.Duration) {
			frames++
			dts = append(dts, dt)
			c.FillRect(float64(frames), 0, 1, 1)
		}),
	))
	cv := s.Find("canvas")
	if cv.DrawCount() != 0 {
		t.Fatalf("drew %d times before the first frame", cv.DrawCount())
	}
	s.Flush() // flushing alone never advances an animation
	for i := 0; i < 3; i++ {
		time.Sleep(time.Millisecond)
		s.Frame()
	}
	if cv.DrawCount() != 3 || frames != 3 {
		t.Fatalf("after 3 frames: DrawCount=%d frames=%d", cv.DrawCount(), frames)
	}
	if got := commands(cv.Canvas()); got != "FillRect(3, 0, 1, 1)" {
		t.Fatalf("last picture: %s", got)
	}
	if dts[0] != 0 || dts[1] <= 0 || dts[2] <= 0 {
		t.Fatalf("dt = %v, want 0 then positive", dts)
	}

	s.Unmount()
	s.Frame()
	if frames != 3 {
		t.Fatalf("drew after unmount (%d frames)", frames)
	}
}

// An animated Fill canvas waits for its size, and follows it.
func TestAnimateWithFill(t *testing.T) {
	s := gooweetest.Render(t, canvas.View(
		canvas.Fill(),
		canvas.Animate(func(c *canvas.Ctx, dt time.Duration) { c.FillRect(0, 0, c.Width(), c.Height()) }),
	))
	cv := s.Find("canvas")
	s.Frame()
	if cv.DrawCount() != 1 || len(cv.Canvas()) != 0 {
		t.Fatalf("before layout: DrawCount=%d cmds=%v; want only the empty follow buffer", cv.DrawCount(), cv.Canvas())
	}
	s.ResizeCanvas(cv, 80, 60)
	s.Frame()
	if got := commands(cv.Canvas()); got != "FillRect(0, 0, 80, 60)" {
		t.Fatalf("after layout: %s", got)
	}
}

// An animation can read signals; it doesn't subscribe to them, but every
// frame sees their latest value.
func TestAnimateReadsSignals(t *testing.T) {
	x := core.NewSignal(1.0)
	s := gooweetest.Render(t, canvas.View(
		canvas.Size(10, 10),
		canvas.Animate(func(c *canvas.Ctx, _ time.Duration) { c.FillRect(x.Get(), 0, 1, 1) }),
	))
	s.Frame()
	x.Set(5)
	s.Flush()
	if got := commands(s.Find("canvas").Canvas()); got != "FillRect(1, 0, 1, 1)" {
		t.Fatalf("a Set redrew an animation outside a frame: %s", got)
	}
	s.Frame()
	if got := commands(s.Find("canvas").Canvas()); got != "FillRect(5, 0, 1, 1)" {
		t.Fatalf("got %s", got)
	}
}

func TestAnimateOnTheServer(t *testing.T) {
	body, _ := ssr.New().Render(canvas.View(
		canvas.Size(10, 10),
		canvas.Attrs(h.ID("a")),
		canvas.Animate(func(*canvas.Ctx, time.Duration) { t.Fatal("animated on the server") }),
	))
	if body == "" {
		t.Fatal("no markup")
	}
}
