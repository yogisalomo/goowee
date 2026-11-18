package app

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/examples/graph/graph"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

var dotColors = [...]string{"#00add8", "#ce3262", "#fddd00"}

func canvasPage(r *router.Router) core.Node {
	return core.Component("CanvasPage", func() core.Node {
		// A bar chart: the example from docs/guides/concepts.md.
		sales, setSales := hooks.UseState([]float64{40, 72, 55, 90, 64, 30, 81})
		chart := canvas.View(
			canvas.Size(280, 120),
			canvas.Label("Sales per day, as bars"),
			canvas.Attrs(Class("bars-canvas")),
			canvas.Draw([]core.SignalAccessor{sales}, func(c *canvas.Ctx) {
				c.FillStyle("#00728f")
				for i, v := range sales.Get() {
					c.FillRect(float64(i)*40+4, c.Height()-v, 32, v)
				}
			}),
		)

		// Thousands of dots, recorded into one buffer per frame.
		count, setCount := hooks.UseState(10000)
		phase, setPhase := hooks.UseState(0.0)
		playing, setPlaying := hooks.UseState(false)
		stats, setStats := hooks.UseState("")
		var groups [len(dotColors)][]float32 // reused every frame: recording doesn't allocate
		field := canvas.View(
			canvas.Size(560, 280),
			canvas.Label("An animated field of dots"),
			canvas.Attrs(Class("field-canvas")),
			canvas.Draw([]core.SignalAccessor{count, phase}, func(c *canvas.Ctx) {
				start := time.Now()
				n, t := count.Get(), phase.Get()
				for g := range groups {
					groups[g] = groups[g][:0]
				}
				cx, cy := c.Width()/2, c.Height()/2
				dot := float32(max(0.6, 1.5*math.Sqrt(10000/float64(n)))) // smaller when denser
				for i := 0; i < n; i++ {
					rr := math.Sqrt(float64(i) / float64(n)) // a sunflower: even density
					a := float64(i)*2.399963 + t*(1.6-rr)    // the inside turns faster
					g := i % len(groups)
					groups[g] = append(groups[g],
						float32(cx+rr*(cx-6)*math.Cos(a)), float32(cy+rr*(cy-6)*math.Sin(a)), dot)
				}
				c.FillStyle("#0b1020")
				c.FillRect(0, 0, c.Width(), c.Height())
				for g, color := range dotColors {
					c.FillStyle(color) // one path and one Fill per color
					c.BeginPath()
					c.Circles(groups[g])
					c.Fill()
				}
				setStats(fmt.Sprintf("%s dots · %d KB per frame · recorded in %.1f ms",
					thousands(n), len(c.Commands())*4/1024, float64(time.Since(start).Microseconds())/1000))
			}),
		)

		// Animation: while playing, each frame moves the phase by the time
		// since the last one. core.NextFrame runs on the display's frames,
		// and pauses while the tab is hidden.
		hooks.UseEffect([]core.SignalAccessor{playing}, func() func() {
			if !playing.Get() {
				return nil
			}
			stopped := false
			last := time.Now()
			var tick func()
			tick = func() {
				if stopped {
					return
				}
				now := time.Now()
				setPhase(phase.Get() + min(now.Sub(last), 100*time.Millisecond).Seconds())
				last = now
				core.NextFrame(tick)
			}
			core.NextFrame(tick)
			return func() { stopped = true }
		})
		playLabel := core.Computed([]core.SignalAccessor{playing}, func() string {
			if playing.Get() {
				return "Pause"
			}
			return "Animate"
		})

		demo := Div(Class("canvas-demo"),
			chart,
			Div(Class("demo-row"),
				Button(OnClick(func() {
					cur := sales.Get()
					next := make([]float64, len(cur))
					for i, v := range cur {
						next[i] = math.Mod(v*7+13, 110) + 5
					}
					setSales(next)
				}), Text("Shuffle")),
			),
			field,
			Div(Class("demo-row"),
				Button(Class("play"), OnClick(func() { setPlaying(!playing.Get()) }), TextS(playLabel)),
				Label(Class("dots"), Text("Dots "),
					Input(Type("range"), Min("1000"), Max("50000"), Step("1000"), Value("10000"),
						OnInput(func(v string) {
							if n, err := strconv.Atoi(v); err == nil {
								setCount(n)
							}
						}))),
			),
			P(Class("field-stats"), TextS(stats)),
			P(Class("graph-link"), Text("For a bigger scene, "), r.Link("/graph", "a graph of 10,000 nodes and 10,000 edges"),
				Text(" redraws every frame with "), Code(Text("canvas.Animate")), Text(".")),
		)
		return lessonLayout(r, "Canvas",
			"`goowee/canvas` draws on a `<canvas>` from Go. A draw function records commands into one buffer, and the runtime replays it in a single call, so thousands of marks per frame stay cheap. Shuffle the bars, then animate the dots and drag the slider.",
			demo, "canvas.go", canvasCode,
			howItWorks(
				"`canvas.View(Size, Draw(deps, fn))` renders a `<canvas>` and runs `fn` once mounted and whenever a dep changes. `fn` records into a `canvas.Ctx`; nothing reaches JavaScript until the frame is sent.",
				"The whole picture crosses into JavaScript as one buffer, copied once, instead of one `syscall/js` call per drawing operation, which costs several times more for big scenes.",
				"Coordinates are CSS pixels; the runtime scales the backing store for the screen's device pixel ratio, so the drawing is sharp on high-density screens.",
				"Group marks by style: one path and one `Fill` per color, using bulk methods like `Circles` and `Segments`. Once the crossing is cheap, the canvas API itself is the cost.",
				"`core.NextFrame(fn)` runs `fn` at the start of the next frame; calling it again from `fn` animates at the display's refresh rate, and stops while the tab is hidden. `canvas.Animate(fn)` does that for a View: one picture per frame.",
				"`canvas.Fill()` makes a canvas follow its parent's size instead of a fixed `Size`, redrawing when it changes. `canvas.Stack` layers canvases, and `canvas.Grid` finds the mark under the pointer.",
				"The draw function never runs on the server; SSR renders the empty `<canvas>` with its label. In tests, gooweetest's `Element.Canvas()` returns the recorded commands.",
			),
			tutorialStepNav(r, "/canvas"),
		)
	})
}

// thousands formats 12000 as "12,000".
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// graphPage shows examples/graph: 10,000 nodes and 10,000 edges, every frame.
func graphPage(r *router.Router) core.Node {
	return Div(Class("page lesson graph-page"),
		r.Link("/canvas", "← Back to Canvas", Class("backlink")),
		H1(Text("10,000 nodes, every frame")),
		P(Class("lead"), rich("Each node circles its cluster's center, so the whole picture changes every frame. `canvas.Animate` records it into one buffer, nodes grouped by color into one path each, and `canvas.Grid` finds the node under the pointer.")),
		Div(Class("graph-box"), graph.View()),
	)
}
