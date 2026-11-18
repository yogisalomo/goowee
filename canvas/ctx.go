// Package canvas draws on a <canvas> element from Go without one JavaScript
// call per drawing operation.
//
// A draw function records commands into a Ctx — a buffer of 32-bit words —
// and the runtime replays the whole buffer onto the element's 2D context in
// one go. For thousands of marks that is several times faster than calling
// the canvas API through syscall/js, and because recording is plain Go the
// package works the same in server rendering and in gooweetest.
//
// Most apps use View, which records and sends a frame whenever the signals it
// depends on change:
//
//	canvas.View(
//	    canvas.Size(300, 120),
//	    canvas.Label("Sales by month"),
//	    canvas.Draw([]core.SignalAccessor{sales}, func(c *canvas.Ctx) {
//	        c.FillStyle("#00add8")
//	        for i, v := range sales.Get() {
//	            c.FillRect(float64(i)*25, c.Height()-v, 20, v)
//	        }
//	    }),
//	)
//
// Coordinates are CSS pixels; the runtime scales for the screen's device
// pixel ratio. Group marks by style — one path and one Fill per color, or the
// bulk methods (Segments, Circles, Rects, Polyline, FillRects) — since once
// the crossing into JavaScript is gone, the canvas API itself is the cost.
package canvas

import (
	"math"
	"slices"
	"strconv"
)

// Version is the command buffer format this package writes. The runtime
// (goowee.js) refuses buffers of a version it doesn't know.
const Version = 1

// Header flags (word 1 of a buffer).
const (
	FlagClear      = 1 << 0 // clear the canvas before drawing
	FlagFollowSize = 1 << 1 // the canvas follows its element's CSS size (Fill); the header's size is ignored
)

const headerWords = 4

// maxOperands is the largest operand count one command word can hold (24 bits).
const maxOperands = 1<<24 - 1

// Ctx records drawing commands. Its methods mirror CanvasRenderingContext2D,
// with float64 arguments for single values and []float32 for the bulk
// methods. A Ctx is reused from frame to frame, so recording into one that
// has grown to fit a scene does not allocate.
//
// A draw function always starts from the context's default state (no
// transform, black fill and stroke, line width 1, …): what one frame sets does
// not leak into the next.
type Ctx struct {
	buf  []uint32
	w, h float64
}

// NewCtx returns an empty context for a w×h (CSS pixel) canvas. View creates
// its own; NewCtx is for recording a buffer by hand (tests, Ref.Draw).
func NewCtx(w, h float64) *Ctx {
	c := &Ctx{}
	c.begin(w, h, FlagClear)
	return c
}

// Width and Height are the canvas's size in CSS pixels.
func (c *Ctx) Width() float64  { return c.w }
func (c *Ctx) Height() float64 { return c.h }

// Commands returns the recorded buffer, header included, for Ref.Draw. It
// aliases the Ctx's storage: it stays valid until the next recording begins.
func (c *Ctx) Commands() []uint32 { return c.buf }

// Reset discards what was recorded and starts a new w×h frame that clears the
// canvas first.
func (c *Ctx) Reset(w, h float64) { c.begin(w, h, FlagClear) }

// begin resets the buffer (keeping its capacity) and writes the header.
func (c *Ctx) begin(w, h float64, flags uint32) {
	c.w, c.h = w, h
	c.buf = append(c.buf[:0], Version, flags, f32(w), f32(h))
}

func f32(v float64) uint32 { return math.Float32bits(float32(v)) }

func word(op Op, n int) uint32 { return uint32(op) | uint32(n)<<8 }

func (c *Ctx) op0(op Op) { c.buf = append(c.buf, word(op, 0)) }
func (c *Ctx) op1(op Op, a float64) {
	c.buf = append(c.buf, word(op, 1), f32(a))
}
func (c *Ctx) op2(op Op, a, b float64) {
	c.buf = append(c.buf, word(op, 2), f32(a), f32(b))
}
func (c *Ctx) op4(op Op, a, b, d, e float64) {
	c.buf = append(c.buf, word(op, 4), f32(a), f32(b), f32(d), f32(e))
}
func (c *Ctx) op5(op Op, a, b, d, e, g float64) {
	c.buf = append(c.buf, word(op, 5), f32(a), f32(b), f32(d), f32(e), f32(g))
}
func (c *Ctx) op6(op Op, a, b, d, e, g, k float64) {
	c.buf = append(c.buf, word(op, 6), f32(a), f32(b), f32(d), f32(e), f32(g), f32(k))
}
func (c *Ctx) opInt(op Op, v uint32) { c.buf = append(c.buf, word(op, 1), v) }

// stringWords is how many words s takes as an operand: its length, then its
// bytes padded with zeros to a multiple of four.
func stringWords(s string) int { return 1 + (len(s)+3)/4 }

// appendString writes s as a string operand. Bytes are packed little-endian,
// so the runtime reads them back as a plain byte view of the buffer.
func (c *Ctx) appendString(s string) {
	c.buf = append(c.buf, uint32(len(s)))
	for i := 0; i < len(s); i += 4 {
		var w uint32
		for j := 0; j < 4 && i+j < len(s); j++ {
			w |= uint32(s[i+j]) << (8 * j)
		}
		c.buf = append(c.buf, w)
	}
}

func (c *Ctx) opString(op Op, s string) {
	n := stringWords(s)
	if n > maxOperands {
		panic("canvas: " + op.String() + " string is too long (" + strconv.Itoa(len(s)) + " bytes)")
	}
	c.buf = append(c.buf, word(op, n))
	c.appendString(s)
}

func (c *Ctx) opXYString(op Op, x, y float64, s string) {
	n := 2 + stringWords(s)
	if n > maxOperands {
		panic("canvas: " + op.String() + " text is too long (" + strconv.Itoa(len(s)) + " bytes)")
	}
	c.buf = append(c.buf, word(op, n), f32(x), f32(y))
	c.appendString(s)
}

func (c *Ctx) appendFloats(v []float32) {
	n := len(c.buf)
	c.buf = slices.Grow(c.buf, len(v))[:n+len(v)]
	dst := c.buf[n:]
	for i, x := range v {
		dst[i] = math.Float32bits(x)
	}
}

// bulk writes v as commands of op, items of size floats each. Items are
// independent, so a slice too long for one command is split across several
// with the same meaning.
func (c *Ctx) bulk(op Op, size int, what string, v []float32) {
	if len(v)%size != 0 {
		panic("canvas: " + op.String() + " needs a multiple of " + strconv.Itoa(size) +
			" values (" + what + " per item), got " + strconv.Itoa(len(v)))
	}
	limit := maxOperands / size * size
	for len(v) > 0 {
		chunk := v
		if len(chunk) > limit {
			chunk = v[:limit]
		}
		c.buf = append(c.buf, word(op, len(chunk)))
		c.appendFloats(chunk)
		v = v[len(chunk):]
	}
}

// State.

// Save pushes the drawing state (styles, transform, clip). Restore pops it.
// Unbalanced calls are harmless: every frame ends by restoring the default.
func (c *Ctx) Save()    { c.op0(OpSave) }
func (c *Ctx) Restore() { c.op0(OpRestore) }

// Transforms.

func (c *Ctx) Translate(x, y float64) { c.op2(OpTranslate, x, y) }
func (c *Ctx) Scale(x, y float64)     { c.op2(OpScale, x, y) }

// Rotate rotates by angle radians, clockwise.
func (c *Ctx) Rotate(angle float64) { c.op1(OpRotate, angle) }

// Transform multiplies the current transform by the matrix [a c e; b d f; 0 0 1].
func (c *Ctx) Transform(a, b, cc, d, e, f float64) { c.op6(OpTransform, a, b, cc, d, e, f) }

// Styles.

// FillStyle and StrokeStyle take a CSS color: "#00add8", "rgb(0 0 0 / 50%)", "teal".
func (c *Ctx) FillStyle(color string)   { c.opString(OpFillStyle, color) }
func (c *Ctx) StrokeStyle(color string) { c.opString(OpStrokeStyle, color) }
func (c *Ctx) LineWidth(w float64)      { c.op1(OpLineWidth, w) }

// LineCap is how the ends of lines are drawn.
type LineCap uint32

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

func (c *Ctx) LineCap(v LineCap) { c.opInt(OpLineCap, uint32(v)) }

// LineJoin is how two connected segments are joined.
type LineJoin uint32

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

func (c *Ctx) LineJoin(v LineJoin) { c.opInt(OpLineJoin, uint32(v)) }

// GlobalAlpha sets the opacity (0–1) of everything drawn after it.
func (c *Ctx) GlobalAlpha(a float64) { c.op1(OpGlobalAlpha, a) }

// LineDash sets alternating dash and gap lengths; no arguments draws solid lines.
func (c *Ctx) LineDash(segments ...float64) {
	if len(segments) > maxOperands {
		panic("canvas: LineDash has too many segments")
	}
	c.buf = append(c.buf, word(OpLineDash, len(segments)))
	for _, s := range segments {
		c.buf = append(c.buf, f32(s))
	}
}

// Font takes a CSS font: "14px system-ui", "bold 12px monospace".
func (c *Ctx) Font(css string) { c.opString(OpFont, css) }

// TextAlign is the horizontal alignment of text relative to its x.
type TextAlign uint32

const (
	AlignStart TextAlign = iota
	AlignEnd
	AlignLeft
	AlignRight
	AlignCenter
)

func (c *Ctx) TextAlign(v TextAlign) { c.opInt(OpTextAlign, uint32(v)) }

// TextBaseline is the vertical alignment of text relative to its y.
type TextBaseline uint32

const (
	BaselineAlphabetic TextBaseline = iota
	BaselineTop
	BaselineHanging
	BaselineMiddle
	BaselineIdeographic
	BaselineBottom
)

func (c *Ctx) TextBaseline(v TextBaseline) { c.opInt(OpTextBaseline, uint32(v)) }

// Paths.

func (c *Ctx) BeginPath()              { c.op0(OpBeginPath) }
func (c *Ctx) ClosePath()              { c.op0(OpClosePath) }
func (c *Ctx) MoveTo(x, y float64)     { c.op2(OpMoveTo, x, y) }
func (c *Ctx) LineTo(x, y float64)     { c.op2(OpLineTo, x, y) }
func (c *Ctx) Rect(x, y, w, h float64) { c.op4(OpRect, x, y, w, h) }

// Arc adds a clockwise arc of radius r around (x, y) from startAngle to
// endAngle (radians, 0 pointing right).
func (c *Ctx) Arc(x, y, r, startAngle, endAngle float64) {
	c.op5(OpArc, x, y, r, startAngle, endAngle)
}

// QuadTo adds a quadratic Bézier curve through control point (cx, cy) to (x, y).
func (c *Ctx) QuadTo(cx, cy, x, y float64) { c.op4(OpQuadTo, cx, cy, x, y) }

// CubicTo adds a cubic Bézier curve through two control points to (x, y).
func (c *Ctx) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	c.op6(OpCubicTo, c1x, c1y, c2x, c2y, x, y)
}

func (c *Ctx) Fill()   { c.op0(OpFill) }
func (c *Ctx) Stroke() { c.op0(OpStroke) }

// Clip restricts later drawing to the current path, until Restore.
func (c *Ctx) Clip() { c.op0(OpClip) }

// Rectangles and text.

func (c *Ctx) ClearRect(x, y, w, h float64)  { c.op4(OpClearRect, x, y, w, h) }
func (c *Ctx) FillRect(x, y, w, h float64)   { c.op4(OpFillRect, x, y, w, h) }
func (c *Ctx) StrokeRect(x, y, w, h float64) { c.op4(OpStrokeRect, x, y, w, h) }
func (c *Ctx) FillText(x, y float64, text string) {
	c.opXYString(OpFillText, x, y, text)
}
func (c *Ctx) StrokeText(x, y float64, text string) {
	c.opXYString(OpStrokeText, x, y, text)
}

// Bulk methods: one command for many marks. Each adds to the current path
// (except FillRects), so draw a group with BeginPath, the bulk call, then one
// Fill or Stroke. They panic when len(v) is not a multiple of the item size.

// Segments adds a line per (x1, y1, x2, y2).
func (c *Ctx) Segments(v []float32) { c.bulk(OpSegments, 4, "x1, y1, x2, y2", v) }

// Circles adds a full circle per (x, y, r).
func (c *Ctx) Circles(v []float32) { c.bulk(OpCircles, 3, "x, y, r", v) }

// Rects adds a rectangle per (x, y, w, h).
func (c *Ctx) Rects(v []float32) { c.bulk(OpRects, 4, "x, y, w, h", v) }

// Polyline adds one connected line through the (x, y) points.
func (c *Ctx) Polyline(v []float32) {
	if len(v)%2 != 0 {
		panic("canvas: Polyline needs a multiple of 2 values (x, y per point), got " + strconv.Itoa(len(v)))
	}
	if len(v) > maxOperands {
		panic("canvas: Polyline has too many points; split it into several")
	}
	c.buf = append(c.buf, word(OpPolyline, len(v)))
	c.appendFloats(v)
}

// FillRects fills a rectangle per (x, y, w, h) with the fill style, without
// touching the current path.
func (c *Ctx) FillRects(v []float32) { c.bulk(OpFillRects, 4, "x, y, w, h", v) }

// Conveniences.

// Line strokes one line from (x1, y1) to (x2, y2).
func (c *Ctx) Line(x1, y1, x2, y2 float64) {
	c.BeginPath()
	c.MoveTo(x1, y1)
	c.LineTo(x2, y2)
	c.Stroke()
}

// Circle fills one circle.
func (c *Ctx) Circle(x, y, r float64) {
	c.BeginPath()
	c.Arc(x, y, r, 0, 2*math.Pi)
	c.Fill()
}
