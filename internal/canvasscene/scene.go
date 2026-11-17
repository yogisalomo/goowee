// Package canvasscene is a drawing that uses every goowee/canvas command once.
// The canvas package's golden test decodes it, and the canvas E2E app replays
// it in Chrome, so together they prove the Go recorder and the JavaScript
// replayer agree on every op.
package canvasscene

import "github.com/yogisalomo/goowee/canvas"

// Width and Height are the scene's canvas size in CSS pixels.
const Width, Height = 120, 90

// Draw records the scene. Every value is exactly representable as a float32,
// so the expected canvas calls are exact.
func Draw(c *canvas.Ctx) {
	c.Save()
	c.Translate(2, 3)
	c.Scale(1.5, 0.5)
	c.Rotate(0.25)
	c.Transform(1, 0, 0, 1, 4, 8)
	c.Restore()
	c.FillStyle("#00add8")
	c.StrokeStyle("rgb(10 20 30 / 50%)")
	c.LineWidth(2.5)
	c.LineCap(canvas.CapRound)
	c.LineJoin(canvas.JoinBevel)
	c.GlobalAlpha(0.75)
	c.LineDash(4, 2)
	c.Font("bold 12px monospace")
	c.TextAlign(canvas.AlignCenter)
	c.TextBaseline(canvas.BaselineMiddle)
	c.BeginPath()
	c.MoveTo(1, 2)
	c.LineTo(30, 40)
	c.Arc(50, 50, 10, 0, 3.5)
	c.Rect(5, 6, 7, 8)
	c.QuadTo(10, 0, 20, 10)
	c.CubicTo(1, 2, 3, 4, 5, 6)
	c.ClosePath()
	c.Fill()
	c.Stroke()
	c.Clip()
	c.ClearRect(0, 0, 10, 10)
	c.FillRect(10, 10, 20, 20)
	c.StrokeRect(15, 15, 5, 5)
	c.FillText(60, 45, "héllo ✓")
	c.StrokeText(60, 70, "")
	c.Segments([]float32{0, 0, 10, 10, 20, 20, 30, 0})
	c.Circles([]float32{40, 40, 3, 60, 40, 4})
	c.Rects([]float32{1, 1, 2, 2})
	c.Polyline([]float32{0, 80, 40, 60, 80, 80})
	c.FillRects([]float32{100, 10, 8, 8, 100, 30, 8, 8})
}
