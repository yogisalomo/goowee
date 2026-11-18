package canvas_test

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/canvas"
)

func TestGridAt(t *testing.T) {
	g := canvas.NewGrid(10)
	g.Insert(1, 5, 5, 4)
	g.Insert(2, 25, 5, 8)    // spans cells 1 to 3 horizontally
	g.Insert(3, 28, 5, 3)    // inside 2: nearer to its own center
	g.Insert(4, -15, -15, 2) // negative coordinates
	g.Insert(5, 5, 5, 4)     // same as 1: drawn on top
	for _, tc := range []struct {
		x, y float64
		id   int
		ok   bool
	}{
		{5, 5, 5, true},
		{8.9, 5, 5, true},
		{9.5, 5, 0, false}, // in 1's cell, outside its circle
		{18, 5, 2, true},   // in cell 1, from 2's left edge
		{29, 5, 3, true},
		{32.5, 5, 2, true}, // cell 3
		{-15, -14, 4, true},
		{100, 100, 0, false},
		{math.NaN(), 0, 0, false},
	} {
		id, ok := g.At(tc.x, tc.y)
		if id != tc.id || ok != tc.ok {
			t.Errorf("At(%v, %v) = %d, %v; want %d, %v", tc.x, tc.y, id, ok, tc.id, tc.ok)
		}
	}
	if g.Len() != 5 {
		t.Fatalf("Len = %d", g.Len())
	}
	g.Reset()
	if _, ok := g.At(5, 5); ok || g.Len() != 0 {
		t.Fatal("Reset kept marks")
	}
	g.Insert(9, 0, 0, math.Inf(1))
	g.Insert(9, math.NaN(), 0, 1)
	if g.Len() != 0 {
		t.Fatal("inserted a mark with an infinite or NaN value")
	}
}

// At agrees with checking every mark.
func TestGridMatchesBruteForce(t *testing.T) {
	type mark struct{ x, y, r float64 }
	rng := rand.New(rand.NewSource(1))
	marks := make([]mark, 500)
	g := canvas.NewGrid(12)
	for i := range marks {
		marks[i] = mark{rng.Float64() * 400, rng.Float64() * 300, 1 + rng.Float64()*10}
		g.Insert(i, marks[i].x, marks[i].y, marks[i].r)
	}
	for q := 0; q < 2000; q++ {
		x, y := rng.Float64()*400, rng.Float64()*300
		want, wantOK, best := 0, false, math.Inf(1)
		for i, m := range marks {
			d := (x-m.x)*(x-m.x) + (y-m.y)*(y-m.y)
			if d <= m.r*m.r && d <= best {
				want, wantOK, best = i, true, d
			}
		}
		if got, ok := g.At(x, y); got != want || ok != wantOK {
			t.Fatalf("At(%v, %v) = %d, %v; brute force %d, %v", x, y, got, ok, want, wantOK)
		}
	}
}

func TestGridReuseDoesNotAllocate(t *testing.T) {
	g := canvas.NewGrid(16)
	fill := func() {
		g.Reset()
		for i := 0; i < 10000; i++ {
			g.Insert(i, float64(i%100)*8, float64(i/100)*6, 3)
		}
	}
	fill()
	if n := testing.AllocsPerRun(5, fill); n != 0 {
		t.Fatalf("%v allocs per refill, want 0", n)
	}
}

func TestNewGridPanics(t *testing.T) {
	for _, cell := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, "cell size") {
					t.Errorf("NewGrid(%v): panic %q", cell, r)
				}
			}()
			canvas.NewGrid(cell)
		}()
	}
}

func BenchmarkGrid(b *testing.B) {
	g := canvas.NewGrid(16)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		g.Reset()
		for j := 0; j < 10000; j++ {
			g.Insert(j, float64(j%100)*8, float64(j/100)*6, 3)
		}
		g.At(400, 300)
	}
}
