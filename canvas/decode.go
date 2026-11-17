package canvas

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Op is a command's opcode. The numbers are part of the buffer format and
// never change; new commands get new numbers.
type Op uint8

const (
	OpSave         Op = 1
	OpRestore      Op = 2
	OpTranslate    Op = 3
	OpScale        Op = 4
	OpRotate       Op = 5
	OpTransform    Op = 6
	OpFillStyle    Op = 10
	OpStrokeStyle  Op = 11
	OpLineWidth    Op = 12
	OpLineCap      Op = 13
	OpLineJoin     Op = 14
	OpGlobalAlpha  Op = 15
	OpLineDash     Op = 16
	OpFont         Op = 17
	OpTextAlign    Op = 18
	OpTextBaseline Op = 19
	OpBeginPath    Op = 30
	OpClosePath    Op = 31
	OpMoveTo       Op = 32
	OpLineTo       Op = 33
	OpArc          Op = 34
	OpRect         Op = 35
	OpQuadTo       Op = 36
	OpCubicTo      Op = 37
	OpFill         Op = 38
	OpStroke       Op = 39
	OpClip         Op = 40
	OpClearRect    Op = 50
	OpFillRect     Op = 51
	OpStrokeRect   Op = 52
	OpFillText     Op = 53
	OpStrokeText   Op = 54
	OpSegments     Op = 60
	OpCircles      Op = 61
	OpRects        Op = 62
	OpPolyline     Op = 63
	OpFillRects    Op = 64
)

type operands uint8

const (
	fixedFloats operands = iota // n float operands, n fixed
	oneInt                      // one integer (an enum)
	str                         // one string
	xyStr                       // two floats, then a string
	floats                      // any number of floats, a multiple of size
)

type opSpec struct {
	name string
	kind operands
	n    int // fixedFloats: the count; floats: the item size
}

var specs = [256]opSpec{
	OpSave:         {"Save", fixedFloats, 0},
	OpRestore:      {"Restore", fixedFloats, 0},
	OpTranslate:    {"Translate", fixedFloats, 2},
	OpScale:        {"Scale", fixedFloats, 2},
	OpRotate:       {"Rotate", fixedFloats, 1},
	OpTransform:    {"Transform", fixedFloats, 6},
	OpFillStyle:    {"FillStyle", str, 0},
	OpStrokeStyle:  {"StrokeStyle", str, 0},
	OpLineWidth:    {"LineWidth", fixedFloats, 1},
	OpLineCap:      {"LineCap", oneInt, 0},
	OpLineJoin:     {"LineJoin", oneInt, 0},
	OpGlobalAlpha:  {"GlobalAlpha", fixedFloats, 1},
	OpLineDash:     {"LineDash", floats, 1},
	OpFont:         {"Font", str, 0},
	OpTextAlign:    {"TextAlign", oneInt, 0},
	OpTextBaseline: {"TextBaseline", oneInt, 0},
	OpBeginPath:    {"BeginPath", fixedFloats, 0},
	OpClosePath:    {"ClosePath", fixedFloats, 0},
	OpMoveTo:       {"MoveTo", fixedFloats, 2},
	OpLineTo:       {"LineTo", fixedFloats, 2},
	OpArc:          {"Arc", fixedFloats, 5},
	OpRect:         {"Rect", fixedFloats, 4},
	OpQuadTo:       {"QuadTo", fixedFloats, 4},
	OpCubicTo:      {"CubicTo", fixedFloats, 6},
	OpFill:         {"Fill", fixedFloats, 0},
	OpStroke:       {"Stroke", fixedFloats, 0},
	OpClip:         {"Clip", fixedFloats, 0},
	OpClearRect:    {"ClearRect", fixedFloats, 4},
	OpFillRect:     {"FillRect", fixedFloats, 4},
	OpStrokeRect:   {"StrokeRect", fixedFloats, 4},
	OpFillText:     {"FillText", xyStr, 0},
	OpStrokeText:   {"StrokeText", xyStr, 0},
	OpSegments:     {"Segments", floats, 4},
	OpCircles:      {"Circles", floats, 3},
	OpRects:        {"Rects", floats, 4},
	OpPolyline:     {"Polyline", floats, 2},
	OpFillRects:    {"FillRects", floats, 4},
}

// String is the op's name, as its Ctx method is called ("FillRect"), or
// "Op(n)" for a number this version doesn't know.
func (o Op) String() string {
	if s := specs[o].name; s != "" {
		return s
	}
	return "Op(" + strconv.Itoa(int(o)) + ")"
}

// Command is one decoded drawing command.
type Command struct {
	Op   Op
	Args []float64 // float and integer operands, in order
	Text string    // the string operand, if the op has one
}

// String formats the command like the Ctx call that recorded it:
// FillRect(0, 0, 10, 10), FillStyle("#888"), FillText(4, 12, "hi").
func (c Command) String() string {
	var b strings.Builder
	b.WriteString(c.Op.String())
	b.WriteByte('(')
	for i, a := range c.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(formatFloat(a))
	}
	if k := specs[c.Op].kind; k == str || k == xyStr {
		if len(c.Args) > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(c.Text))
	}
	b.WriteByte(')')
	return b.String()
}

// MarshalJSON writes {"op": "FillRect", "args": [0, 0, 10, 10]}, with
// "text" for ops that carry a string. Numbers are written at float32
// precision, the precision the buffer stores.
func (c Command) MarshalJSON() ([]byte, error) {
	b := []byte(`{"op":`)
	b = strconv.AppendQuote(b, c.Op.String())
	if len(c.Args) > 0 {
		b = append(b, `,"args":[`...)
		for i, a := range c.Args {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, formatFloat(a)...)
		}
		b = append(b, ']')
	}
	if k := specs[c.Op].kind; k == str || k == xyStr {
		b = append(b, `,"text":`...)
		b = strconv.AppendQuote(b, c.Text)
	}
	return append(b, '}'), nil
}

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 32) }

// Decoding errors.
var (
	ErrVersion   = errors.New("canvas: unknown buffer format version")
	ErrTruncated = errors.New("canvas: truncated or malformed buffer")
)

// Decode reads a buffer written by Ctx: the canvas size in CSS pixels, the
// header flags, and the commands. Ops this version doesn't know are skipped,
// as the runtime skips them. It returns ErrVersion for another format version
// and ErrTruncated (wrapped, with the position) for a command that runs past
// the end or doesn't match its op's operands.
func Decode(buf []uint32) (w, h float64, flags uint32, cmds []Command, err error) {
	if len(buf) < headerWords {
		return 0, 0, 0, nil, wrap(ErrTruncated, "the header needs 4 words, got "+strconv.Itoa(len(buf)))
	}
	if buf[0] != Version {
		return 0, 0, 0, nil, wrap(ErrVersion, "got "+strconv.Itoa(int(buf[0]))+", want "+strconv.Itoa(Version))
	}
	flags = buf[1]
	w, h = fl(buf[2]), fl(buf[3])
	for i := headerWords; i < len(buf); {
		at := i
		op, n := Op(buf[i]&0xff), int(buf[i]>>8)
		i++
		if i+n > len(buf) {
			return w, h, flags, cmds, bad(at, "a command with "+strconv.Itoa(n)+" operands runs past the end")
		}
		ops := buf[i : i+n]
		i += n
		spec := specs[op]
		if spec.name == "" {
			continue // a newer op: skip it, like the runtime does
		}
		cmd := Command{Op: op}
		switch spec.kind {
		case fixedFloats:
			if n != spec.n {
				return w, h, flags, cmds, bad(at, op.String()+" takes "+strconv.Itoa(spec.n)+" operands, got "+strconv.Itoa(n))
			}
			cmd.Args = floatsOf(ops)
		case oneInt:
			if n != 1 {
				return w, h, flags, cmds, bad(at, op.String()+" takes 1 operand, got "+strconv.Itoa(n))
			}
			cmd.Args = []float64{float64(ops[0])}
		case str:
			s, ok := stringOf(ops)
			if !ok {
				return w, h, flags, cmds, bad(at, op.String()+" has a malformed string")
			}
			cmd.Text = s
		case xyStr:
			if n < 3 {
				return w, h, flags, cmds, bad(at, op.String()+" needs x, y and a string")
			}
			s, ok := stringOf(ops[2:])
			if !ok {
				return w, h, flags, cmds, bad(at, op.String()+" has a malformed string")
			}
			cmd.Args = floatsOf(ops[:2])
			cmd.Text = s
		case floats:
			if n%spec.n != 0 {
				return w, h, flags, cmds, bad(at, op.String()+" needs a multiple of "+strconv.Itoa(spec.n)+" operands, got "+strconv.Itoa(n))
			}
			cmd.Args = floatsOf(ops)
		}
		cmds = append(cmds, cmd)
	}
	return w, h, flags, cmds, nil
}

func fl(u uint32) float64 { return float64(math.Float32frombits(u)) }

func floatsOf(ws []uint32) []float64 {
	if len(ws) == 0 {
		return nil
	}
	out := make([]float64, len(ws))
	for i, u := range ws {
		out[i] = fl(u)
	}
	return out
}

// stringOf reads a string operand that must fill ws exactly.
func stringOf(ws []uint32) (string, bool) {
	if len(ws) < 1 {
		return "", false
	}
	n := int(ws[0])
	if 1+(n+3)/4 != len(ws) {
		return "", false
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(ws[1+i/4] >> (8 * (i % 4)))
	}
	return string(b), true
}

type decodeError struct {
	base   error
	detail string
}

func (e *decodeError) Error() string { return e.base.Error() + ": " + e.detail }
func (e *decodeError) Unwrap() error { return e.base }

func wrap(base error, detail string) error { return &decodeError{base, detail} }

func bad(at int, detail string) error {
	return wrap(ErrTruncated, "word "+strconv.Itoa(at)+": "+detail)
}
