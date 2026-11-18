//go:build js && wasm

// Command canvas is the app test/e2e/canvas.mjs drives: goowee/canvas buffers
// replayed by the real runtime onto real <canvas> elements.
//   - #pixels: a red square drawn with ref.Draw, checked pixel by pixel, at
//     device scale factors 1 and 2.
//   - #scene: internal/canvasscene, every op once; the page logs the canvas
//     calls it receives and the test compares them with canvas/testdata/scene.json.
//   - #view: a canvas.View redrawn when a signal changes (#grow).
//   - #fill: a canvas.Fill View in a 300×200 box (#fillbox) that the test
//     resizes; it paints its whole area red.
//   - #anim: a canvas.Animate View; the page counts its pictures.
package main

import (
	"time"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/canvasscene"
)

func main() {
	bridge.Run(core.Component("CanvasE2E", func() core.Node {
		pixels, scene := Ref(), Ref()
		hooks.OnMount(func() func() {
			c := canvas.NewCtx(100, 80)
			c.FillStyle("red")
			c.FillRect(10, 10, 20, 20)
			pixels.Draw(c.Commands())

			s := canvas.NewCtx(canvasscene.Width, canvasscene.Height)
			canvasscene.Draw(s)
			scene.Draw(s.Commands())
			return nil
		})
		bars, setBars := hooks.UseState(1)
		return Div(
			Canvas(ID("pixels"), RefTo(pixels), Style("width:100px;height:80px")),
			Canvas(ID("scene"), RefTo(scene), Style("width:120px;height:90px")),
			canvas.View(
				canvas.Size(60, 40),
				canvas.Label("Bars"),
				canvas.Attrs(ID("view")),
				canvas.Draw([]core.SignalAccessor{bars}, func(c *canvas.Ctx) {
					c.FillStyle("#00ff00")
					c.FillRect(0, 0, float64(bars.Get())*10, c.Height())
				}),
			),
			Button(ID("grow"), OnClick(func() { setBars(bars.Get() + 1) }), Text("grow")),
			Div(ID("fillbox"), Style("width:300px;height:200px"),
				canvas.View(
					canvas.Fill(),
					canvas.Attrs(ID("fill")),
					canvas.Draw(nil, func(c *canvas.Ctx) {
						c.FillStyle("red")
						c.FillRect(0, 0, c.Width(), c.Height())
					}),
				),
			),
			canvas.View(
				canvas.Size(40, 40),
				canvas.Attrs(ID("anim")),
				canvas.Animate(func(c *canvas.Ctx, dt time.Duration) {
					x := float64(time.Now().UnixMilli() % 40)
					c.FillStyle("blue")
					c.FillRect(x, 0, 4, 40)
				}),
			),
		)
	}))
}
