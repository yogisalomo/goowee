package canvas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/internal/canvasscene"
)

var update = flag.Bool("update", false, "rewrite testdata/scene.json")

func decode(t *testing.T, c *canvas.Ctx) []canvas.Command {
	t.Helper()
	_, _, _, cmds, err := canvas.Decode(c.Commands())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return cmds
}

func TestHeader(t *testing.T) {
	c := canvas.NewCtx(320, 180.5)
	w, h, flags, cmds, err := canvas.Decode(c.Commands())
	if err != nil || w != 320 || h != 180.5 || flags != canvas.FlagClear || len(cmds) != 0 {
		t.Fatalf("got w=%v h=%v flags=%v cmds=%v err=%v", w, h, flags, cmds, err)
	}
	if c.Width() != 320 || c.Height() != 180.5 {
		t.Fatalf("Width/Height = %v×%v", c.Width(), c.Height())
	}
	c.FillRect(1, 2, 3, 4)
	c.Reset(10, 20)
	if got := decode(t, c); len(got) != 0 || c.Width() != 10 {
		t.Fatalf("Reset kept %v, width %v", got, c.Width())
	}
}

// Every method records the command Decode reads back.
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		draw func(c *canvas.Ctx)
		want string
	}{
		{func(c *canvas.Ctx) { c.Save() }, "Save()"},
		{func(c *canvas.Ctx) { c.Restore() }, "Restore()"},
		{func(c *canvas.Ctx) { c.Translate(1.5, -2) }, "Translate(1.5, -2)"},
		{func(c *canvas.Ctx) { c.Scale(2, 3) }, "Scale(2, 3)"},
		{func(c *canvas.Ctx) { c.Rotate(0.5) }, "Rotate(0.5)"},
		{func(c *canvas.Ctx) { c.Transform(1, 2, 3, 4, 5, 6) }, "Transform(1, 2, 3, 4, 5, 6)"},
		{func(c *canvas.Ctx) { c.FillStyle("#888") }, `FillStyle("#888")`},
		{func(c *canvas.Ctx) { c.StrokeStyle("teal") }, `StrokeStyle("teal")`},
		{func(c *canvas.Ctx) { c.LineWidth(3) }, "LineWidth(3)"},
		{func(c *canvas.Ctx) { c.LineCap(canvas.CapSquare) }, "LineCap(2)"},
		{func(c *canvas.Ctx) { c.LineJoin(canvas.JoinRound) }, "LineJoin(1)"},
		{func(c *canvas.Ctx) { c.GlobalAlpha(0.25) }, "GlobalAlpha(0.25)"},
		{func(c *canvas.Ctx) { c.LineDash(5, 3, 1) }, "LineDash(5, 3, 1)"},
		{func(c *canvas.Ctx) { c.LineDash() }, "LineDash()"},
		{func(c *canvas.Ctx) { c.Font("12px serif") }, `Font("12px serif")`},
		{func(c *canvas.Ctx) { c.TextAlign(canvas.AlignRight) }, "TextAlign(3)"},
		{func(c *canvas.Ctx) { c.TextBaseline(canvas.BaselineBottom) }, "TextBaseline(5)"},
		{func(c *canvas.Ctx) { c.BeginPath() }, "BeginPath()"},
		{func(c *canvas.Ctx) { c.ClosePath() }, "ClosePath()"},
		{func(c *canvas.Ctx) { c.MoveTo(1, 2) }, "MoveTo(1, 2)"},
		{func(c *canvas.Ctx) { c.LineTo(3, 4) }, "LineTo(3, 4)"},
		{func(c *canvas.Ctx) { c.Arc(1, 2, 3, 0, 1.5) }, "Arc(1, 2, 3, 0, 1.5)"},
		{func(c *canvas.Ctx) { c.Rect(1, 2, 3, 4) }, "Rect(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.QuadTo(1, 2, 3, 4) }, "QuadTo(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.CubicTo(1, 2, 3, 4, 5, 6) }, "CubicTo(1, 2, 3, 4, 5, 6)"},
		{func(c *canvas.Ctx) { c.Fill() }, "Fill()"},
		{func(c *canvas.Ctx) { c.Stroke() }, "Stroke()"},
		{func(c *canvas.Ctx) { c.Clip() }, "Clip()"},
		{func(c *canvas.Ctx) { c.ClearRect(0, 0, 5, 5) }, "ClearRect(0, 0, 5, 5)"},
		{func(c *canvas.Ctx) { c.FillRect(1, 2, 3, 4) }, "FillRect(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.StrokeRect(1, 2, 3, 4) }, "StrokeRect(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.FillText(4, 12, "hi") }, `FillText(4, 12, "hi")`},
		{func(c *canvas.Ctx) { c.StrokeText(4, 12, "yo") }, `StrokeText(4, 12, "yo")`},
		{func(c *canvas.Ctx) { c.Segments([]float32{1, 2, 3, 4}) }, "Segments(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.Circles([]float32{1, 2, 3}) }, "Circles(1, 2, 3)"},
		{func(c *canvas.Ctx) { c.Rects([]float32{1, 2, 3, 4}) }, "Rects(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.Polyline([]float32{1, 2, 3, 4, 5, 6}) }, "Polyline(1, 2, 3, 4, 5, 6)"},
		{func(c *canvas.Ctx) { c.FillRects([]float32{1, 2, 3, 4}) }, "FillRects(1, 2, 3, 4)"},
		{func(c *canvas.Ctx) { c.Line(1, 2, 3, 4) }, "BeginPath() MoveTo(1, 2) LineTo(3, 4) Stroke()"},
		{func(c *canvas.Ctx) { c.Circle(5, 6, 7) }, "BeginPath() Arc(5, 6, 7, 0, 6.2831855) Fill()"},
	}
	for _, tc := range cases {
		c := canvas.NewCtx(10, 10)
		tc.draw(c)
		var got []string
		for _, cmd := range decode(t, c) {
			got = append(got, cmd.String())
		}
		if s := strings.Join(got, " "); s != tc.want {
			t.Errorf("got %s, want %s", s, tc.want)
		}
	}
}

func TestStrings(t *testing.T) {
	for _, s := range []string{"", "a", "abcd", "abcde", "日本語", "✓ ok", strings.Repeat("x", 1000)} {
		c := canvas.NewCtx(1, 1)
		c.FillStyle(s)
		c.FillText(1, 2, s)
		c.LineWidth(7) // a command after the padded string still decodes in place
		cmds := decode(t, c)
		if len(cmds) != 3 || cmds[0].Text != s || cmds[1].Text != s || cmds[2].Args[0] != 7 {
			t.Errorf("%q: got %v", s, cmds)
		}
	}
}

func TestUnknownOpIsSkipped(t *testing.T) {
	c := canvas.NewCtx(1, 1)
	c.Fill()
	buf := append([]uint32{}, c.Commands()...)
	buf = append(buf, 200|2<<8, 1, 2) // a future op with two operands
	buf = append(buf, uint32(canvas.OpStroke))
	_, _, _, cmds, err := canvas.Decode(buf)
	if err != nil || len(cmds) != 2 || cmds[0].Op != canvas.OpFill || cmds[1].Op != canvas.OpStroke {
		t.Fatalf("got %v, %v", cmds, err)
	}
	if canvas.Op(200).String() != "Op(200)" {
		t.Fatalf("Op(200).String() = %q", canvas.Op(200).String())
	}
}

func TestDecodeErrors(t *testing.T) {
	c := canvas.NewCtx(1, 1)
	c.FillRect(1, 2, 3, 4)
	full := c.Commands()
	wrongArity := append(append([]uint32{}, full[:4]...), uint32(canvas.OpFillRect)|3<<8, 0, 0, 0)
	badString := append(append([]uint32{}, full[:4]...), uint32(canvas.OpFont)|2<<8, 9, 0)
	for name, tc := range map[string]struct {
		buf  []uint32
		want error
	}{
		"short header": {full[:3], canvas.ErrTruncated},
		"version":      {append([]uint32{2}, full[1:]...), canvas.ErrVersion},
		"truncated":    {full[:len(full)-1], canvas.ErrTruncated},
		"arity":        {wrongArity, canvas.ErrTruncated},
		"string":       {badString, canvas.ErrTruncated},
	} {
		if _, _, _, _, err := canvas.Decode(tc.buf); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestBulkPanicsOnARaggedSlice(t *testing.T) {
	c := canvas.NewCtx(1, 1)
	for name, fn := range map[string]func(){
		"Segments":  func() { c.Segments(make([]float32, 7)) },
		"Circles":   func() { c.Circles(make([]float32, 4)) },
		"Rects":     func() { c.Rects(make([]float32, 5)) },
		"Polyline":  func() { c.Polyline(make([]float32, 3)) },
		"FillRects": func() { c.FillRects(make([]float32, 2)) },
	} {
		func() {
			defer func() {
				r := recover()
				if s, _ := r.(string); !strings.Contains(s, name+" needs a multiple of") {
					t.Errorf("%s: panic %v", name, r)
				}
			}()
			fn()
		}()
	}
}

// The golden scene uses every op once. testdata/scene.json is also what the
// canvas E2E test expects Chrome's canvas to receive.
func TestGoldenScene(t *testing.T) {
	c := canvas.NewCtx(canvasscene.Width, canvasscene.Height)
	canvasscene.Draw(c)
	cmds := decode(t, c)

	seen := map[canvas.Op]bool{}
	for _, cmd := range cmds {
		seen[cmd.Op] = true
	}
	for op := 0; op < 256; op++ {
		if name := canvas.Op(op).String(); !strings.HasPrefix(name, "Op(") && !seen[canvas.Op(op)] {
			t.Errorf("the scene doesn't use %s", name)
		}
	}

	var b bytes.Buffer
	b.WriteString("[\n")
	for i, cmd := range cmds {
		line, _ := json.Marshal(cmd)
		b.WriteString("  ")
		b.Write(line)
		if i < len(cmds)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
	if *update {
		if err := os.WriteFile("testdata/scene.json", b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/scene.json")
	if err != nil {
		t.Fatalf("%v (run go test ./canvas -update to create it)", err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("scene.json is out of date (go test ./canvas -update):\n%s", b.String())
	}
	var parsed []map[string]any
	if err := json.Unmarshal(want, &parsed); err != nil || len(parsed) != len(cmds) {
		t.Fatalf("scene.json doesn't parse: %v", err)
	}
}

func TestReuseKeepsCapacity(t *testing.T) {
	c := canvas.NewCtx(10, 10)
	c.Segments(make([]float32, 400))
	first := &c.Commands()[0]
	c.Reset(10, 10)
	c.Segments(make([]float32, 400))
	if &c.Commands()[0] != first {
		t.Fatal("Reset reallocated the buffer")
	}
	if !reflect.DeepEqual(decode(t, c)[0].Args, make([]float64, 400)) {
		t.Fatal("wrong payload")
	}
}

var segs, circs = scene(10000)

func scene(n int) (segs, circs []float32) {
	for i := 0; i < n; i++ {
		a := float64(i) * 0.61803
		x, y := float32(400+380*math.Cos(a)*math.Sin(a*0.37)), float32(300+280*math.Sin(a)*math.Cos(a*0.11))
		segs = append(segs, x, y, float32(400+380*math.Cos(a*1.3)), float32(300+280*math.Sin(a*0.7)))
		circs = append(circs, x, y, 3)
	}
	return
}

func BenchmarkRecordBulk(b *testing.B) {
	c := canvas.NewCtx(800, 600)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Reset(800, 600)
		c.StrokeStyle("#888")
		c.BeginPath()
		c.Segments(segs)
		c.Stroke()
		c.FillStyle("#00add8")
		c.BeginPath()
		c.Circles(circs)
		c.Fill()
	}
}

func BenchmarkRecordPerOp(b *testing.B) {
	c := canvas.NewCtx(800, 600)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Reset(800, 600)
		c.BeginPath()
		for j := 0; j < len(segs); j += 4 {
			c.MoveTo(float64(segs[j]), float64(segs[j+1]))
			c.LineTo(float64(segs[j+2]), float64(segs[j+3]))
		}
		c.Stroke()
		c.BeginPath()
		for j := 0; j < len(circs); j += 3 {
			c.Arc(float64(circs[j]), float64(circs[j+1]), float64(circs[j+2]), 0, 2*math.Pi)
		}
		c.Fill()
	}
}

func TestRecordingDoesNotAllocate(t *testing.T) {
	c := canvas.NewCtx(800, 600)
	draw := func() {
		c.Reset(800, 600)
		c.FillStyle("#00add8")
		c.BeginPath()
		c.Segments(segs)
		c.Circles(circs)
		c.Fill()
		c.FillText(1, 2, "label")
	}
	draw()
	if n := testing.AllocsPerRun(20, draw); n != 0 {
		t.Fatalf("recording into a warm Ctx allocates %v times", n)
	}
}
