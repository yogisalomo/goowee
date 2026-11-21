//go:build js && wasm

// Command graph is the app test/e2e/transport.mjs measures: the force-layout
// graph of BenchmarkLayoutTickFrame (internal/dom) — 500 circles and 1,000
// lines whose endpoints are bound to the circles' position signals — and a
// text label, rendered by the real bridge. The page drives it through three
// functions, each producing the next frame's DOM updates:
//
//	graphTick(step)   moves every node: 5,000 attribute updates
//	graphNudge(step)  moves node 0: 10 attribute updates
//	graphLabel(step)  changes the label's text: 1 property update
//
// graphGCs() reports how many garbage collections the Go runtime has run.
package main

import (
	"fmt"
	"runtime"
	"syscall/js"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

func main() {
	const nodes, edges = 500, 1000
	xs := make([]*core.Signal[float64], nodes)
	ys := make([]*core.Signal[float64], nodes)
	var shapes []core.Item
	for i := range xs {
		xs[i], ys[i] = core.NewSignal(float64(i)), core.NewSignal(float64(i))
		shapes = append(shapes, h.Circle(h.BindAttr("cx", xs[i]), h.BindAttr("cy", ys[i]), h.Attr("r", "4")))
	}
	for e := 0; e < edges; e++ {
		a, b := e%nodes, (e*7+1)%nodes
		shapes = append(shapes, h.Line(
			h.BindAttr("x1", xs[a]), h.BindAttr("y1", ys[a]),
			h.BindAttr("x2", xs[b]), h.BindAttr("y2", ys[b]),
			h.Attr("stroke", "#999"),
		))
	}
	label := core.NewSignal("step 0")
	js.Global().Set("graphGCs", js.FuncOf(func(this js.Value, args []js.Value) any {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.NumGC
	}))

	export := func(name string, fn func(step float64)) {
		js.Global().Set(name, js.FuncOf(func(this js.Value, args []js.Value) any {
			fn(args[0].Float())
			return nil
		}))
	}
	export("graphTick", func(step float64) {
		core.Batch(func() {
			for i := range xs {
				xs[i].Set(float64(i) + step*0.37)
				ys[i].Set(float64(i) - step*0.61)
			}
		})
	})
	export("graphNudge", func(step float64) {
		core.Batch(func() {
			xs[0].Set(step * 0.37)
			ys[0].Set(-step * 0.61)
		})
	})
	export("graphLabel", func(step float64) {
		label.Set(fmt.Sprintf("step %g", step))
	})

	bridge.Run(h.Div(
		h.P(h.Class("label"), h.TextS(label)),
		h.Svg(append([]core.Item{h.Attr("width", "1200"), h.Attr("height", "1200")}, shapes...)...),
	))
}
