package canvas_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/ssr"
)

// Each layer redraws on its own deps only.
func TestStackLayersRedrawIndependently(t *testing.T) {
	edges, hover := core.NewSignal(1.0), core.NewSignal(0.0)
	edgeRuns, hoverRuns := 0, 0
	s := gooweetest.Render(t, canvas.Stack(
		canvas.Options(canvas.Size(100, 50), canvas.Label("Graph"), canvas.Attrs(h.Class("box"))),
		canvas.Draw([]core.SignalAccessor{edges}, func(c *canvas.Ctx) { edgeRuns++; c.LineWidth(edges.Get()) }),
		canvas.Options(canvas.Draw([]core.SignalAccessor{hover}, func(c *canvas.Ctx) { hoverRuns++; c.FillRect(hover.Get(), 0, 1, 1) }),
			canvas.Attrs(h.Class("top"))),
	))
	hover.Set(7)
	s.Flush()
	if edgeRuns != 1 || hoverRuns != 2 {
		t.Fatalf("edges drawn %d times, hover %d; want 1 and 2", edgeRuns, hoverRuns)
	}
	if got := commands(s.Find("canvas.top").Canvas()); got != "FillRect(7, 0, 1, 1)" {
		t.Fatalf("top layer: %s", got)
	}
	box := s.Find("div.box")
	if box.Attr("style") != "position:relative;width:100px;height:50px" || box.Attr("aria-label") != "Graph" || box.Attr("role") != "img" {
		t.Fatalf("box: %s", box.HTML())
	}
	layers := s.FindAll("canvas")
	if len(layers) != 2 || layers[0].Attr("style") != "position:absolute;left:0;top:0;width:100px;height:50px" || layers[0].Attr("aria-hidden") != "true" {
		t.Fatalf("layers: %s", box.HTML())
	}
}

func TestStackWithFillOnTheServer(t *testing.T) {
	body, _ := ssr.New().Render(canvas.Stack(
		canvas.Options(canvas.Fill(), canvas.Fallback(h.P(h.Text("the data")))),
		canvas.Draw(nil, func(*canvas.Ctx) {}),
		canvas.Animate(func(*canvas.Ctx, time.Duration) {}),
	))
	body = regexp.MustCompile(` data-node-id="\d+"|<!--g\d+-->`).ReplaceAllString(body, "")
	for _, want := range []string{
		`<div style="position:relative;width:100%;height:100%">`,
		`<canvas style="position:absolute;left:0;top:0;width:100%;height:100%;display:block"><p>the data</p></canvas>`,
		`<canvas style="position:absolute;left:0;top:0;width:100%;height:100%;display:block" aria-hidden="true"></canvas>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in\n%s", want, body)
		}
	}
}

func TestStackPanics(t *testing.T) {
	draw := canvas.Draw(nil, func(*canvas.Ctx) {})
	for want, f := range map[string][]func(){
		"needs a Size":           {func() { canvas.Stack(canvas.Label("x"), draw) }},
		"at least one layer":     {func() { canvas.Stack(canvas.Size(1, 1)) }},
		"box can't draw":         {func() { canvas.Stack(canvas.Options(canvas.Size(1, 1), draw), draw) }},
		"can't set its own size": {func() { canvas.Stack(canvas.Size(1, 1), canvas.Options(canvas.Size(2, 2), draw)) }},
		"layer needs a Draw":     {func() { canvas.Stack(canvas.Size(1, 1), canvas.NoClear()) }},
	} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, want) {
					t.Errorf("want a panic with %q, got %q", want, r)
				}
			}()
			f[0]()
		}()
	}
}
