package graph

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/gooweetest"
)

func TestGraphDrawsEveryNodeAndEdgeEachFrame(t *testing.T) {
	s := gooweetest.Render(t, View())
	cv := s.Find("#graph")
	s.ResizeCanvas(cv, 800, 600)
	for i := 0; i < 30; i++ {
		s.Frame()
	}
	circles, segments, fills := 0, 0, 0
	for _, c := range cv.Canvas() {
		switch c.Op.String() {
		case "Circles":
			circles += len(c.Args) / 3
		case "Segments":
			segments += len(c.Args) / 4
		case "Fill":
			fills++
		}
	}
	if circles != NodeCount || segments != EdgeCount || fills != len(colors) {
		t.Fatalf("%d circles, %d segments, %d fills; want %d, %d, %d", circles, segments, fills, NodeCount, EdgeCount, len(colors))
	}
	if cv.DrawCount() != 31 { // the follow buffer, then one per frame
		t.Fatalf("DrawCount = %d", cv.DrawCount())
	}
	if got := s.Find("#graph-stats").Text(); !strings.HasPrefix(got, "10,000 nodes, 10,000 edges") {
		t.Fatalf("stats %q", got)
	}
}

// Pointing at a node highlights its edges, found through canvas.Grid.
func TestGraphHover(t *testing.T) {
	s := gooweetest.Render(t, View())
	cv := s.Find("#graph")
	s.ResizeCanvas(cv, 800, 600)
	s.Frame()
	// Node 0's position in the last picture: the first Circles item.
	var x, y float64
	for _, c := range cv.Canvas() {
		if c.Op.String() == "Circles" {
			x, y = c.Args[0], c.Args[1]
			break
		}
	}
	s.Dispatch(cv, "pointermove", map[string]any{"offsetX": x, "offsetY": y})
	if got := s.Find(".graph-info").Text(); !strings.HasPrefix(got, "Node ") {
		t.Fatalf("info after pointing at a node: %q", got)
	}
	before := len(cv.Canvas())
	s.Frame()
	if len(cv.Canvas()) <= before {
		t.Fatal("the hovered node's edges weren't drawn")
	}
	s.Dispatch(cv, "pointerleave", nil)
	if got := s.Find(".graph-info").Text(); !strings.HasPrefix(got, "Point at") {
		t.Fatalf("info after leaving: %q", got)
	}
}

func BenchmarkGraphFrame(b *testing.B) {
	s := gooweetest.Render(b, View())
	s.ResizeCanvas(s.Find("#graph"), 1200, 800)
	s.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Frame()
	}
}
