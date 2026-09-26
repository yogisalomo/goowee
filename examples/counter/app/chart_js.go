//go:build js && wasm

package app

import (
	"strconv"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
)

// drawBars paints values (0–100) as a bar chart on the <canvas> ref points at,
// through the canvas 2D API — the same move as handing an element to a
// JavaScript library, via bridge.Element.
func drawBars(ref *core.Ref, values []int) {
	el := bridge.Element(ref)
	if el.IsUndefined() || el.IsNull() {
		return
	}
	ctx := el.Call("getContext", "2d")
	w, h := el.Get("width").Float(), el.Get("height").Float()
	ctx.Call("clearRect", 0, 0, w, h)
	ctx.Set("fillStyle", "#00728f")
	sum := 0
	for i, v := range values {
		barW := w / float64(len(values))
		barH := h * float64(v) / 100
		ctx.Call("fillRect", float64(i)*barW+2, h-barH, barW-4, barH)
		sum += v
	}
	el.Get("dataset").Set("sum", strconv.Itoa(sum)) // lets the E2E test see it drew
}
