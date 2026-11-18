package canvas

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

// Stack draws a picture in layers: one canvas per layer, placed over each
// other in a box, first layer at the bottom. Each layer redraws on its own,
// so a part that changes rarely (a graph's edges) isn't redrawn when a cheap
// one changes (a hover highlight):
//
//	canvas.Stack(
//	    canvas.Options(canvas.Size(600, 400), canvas.Attrs(h.OnPointerMoveE(hover))),
//	    canvas.Draw([]core.SignalAccessor{graph}, drawEdges),
//	    canvas.Draw([]core.SignalAccessor{hovered}, drawHighlight),
//	)
//
// box sets the size (Size or Fill), shared by every layer. It may add Label
// and Attrs, which go on the box, a <div>: a pointer handler there sees events
// from every layer, with e.OffsetX and e.OffsetY in the layers' coordinates.
// A Fallback in box is rendered inside the bottom layer. Each layer is a Draw
// or an Animate, optionally combined with NoClear or Attrs through Options. Stack panics when the box has no size or
// draws, or when a layer has no drawing or sets its own size.
func Stack(box Option, layers ...Option) core.Node {
	var bc config
	box(&bc)
	bc.checkSize("Stack")
	if bc.draw != nil || bc.animate != nil {
		panic("canvas: Stack's box can't draw; pass each Draw or Animate as a layer")
	}
	if len(layers) == 0 {
		panic("canvas: Stack needs at least one layer")
	}
	style := "position:relative;width:" + px(bc.w) + ";height:" + px(bc.h)
	if bc.fill {
		style = "position:relative;width:100%;height:100%"
	}
	items := make([]core.Item, 0, 3+len(bc.attrs)+len(layers))
	items = append(items, h.Style(style))
	if bc.label != "" {
		items = append(items, h.Role("img"), h.AriaLabel(bc.label))
	}
	items = append(items, bc.attrs...)
	for i, layer := range layers {
		lc := config{w: bc.w, h: bc.h, position: "position:absolute;left:0;top:0;"}
		layer(&lc)
		if lc.sized || lc.fill {
			panic("canvas: a Stack layer can't set its own size; the box's Size or Fill applies to every layer")
		}
		lc.sized, lc.fill = bc.sized, bc.fill
		lc.checkDrawing("a Stack layer")
		if i == 0 {
			// Inside a canvas, the fallback shows only where canvas doesn't.
			lc.fallback = append(append([]core.Node{}, bc.fallback...), lc.fallback...)
		}
		if lc.label == "" && len(lc.fallback) == 0 {
			lc.attrs = append([]core.Item{h.AriaHidden(true)}, lc.attrs...)
		}
		items = append(items, view(lc))
	}
	return h.Div(items...)
}
