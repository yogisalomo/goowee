package canvas

import "math"

// Grid finds the mark under a point, for hit testing a canvas: insert each
// mark as a circle when drawing, then ask At with a pointer event's
// e.OffsetX() and e.OffsetY(). It is a uniform grid: each mark is filed under
// every cell its circle overlaps, so At looks at one cell's marks only.
//
//	grid.Reset()
//	for i, n := range nodes {
//	    grid.Insert(i, n.X, n.Y, n.R)
//	}
//	...
//	if i, ok := grid.At(e.OffsetX(), e.OffsetY()); ok { hovered.Set(i) }
//
// Pick a cell size around the typical mark's diameter, a little larger:
// smaller cells file a mark several times, larger ones make At compare more
// marks. A Grid is reused from frame to frame; once it has grown to fit a
// scene, Reset and Insert don't allocate. It isn't safe for concurrent use.
type Grid struct {
	cell  float64
	items []gridItem
	cells map[gridKey][]int32 // indexes into items
}

type gridItem struct {
	id      int
	x, y, r float64
}

type gridKey struct{ x, y int32 }

// NewGrid returns an empty grid with square cells of the given size, in the
// canvas's CSS pixels. It panics if cell isn't positive.
func NewGrid(cell float64) *Grid {
	if !(cell > 0) || math.IsInf(cell, 0) {
		panic("canvas: NewGrid's cell size must be a positive number")
	}
	return &Grid{cell: cell, cells: make(map[gridKey][]int32)}
}

// Insert adds a mark: a circle of radius r around (x, y), identified by id
// (an index into the app's own slice of marks, say). A point mark has r = 0;
// give it a few pixels so a pointer can find it. A mark with a NaN or
// infinite coordinate or radius is ignored.
func (g *Grid) Insert(id int, x, y, r float64) {
	if r < 0 {
		r = -r
	}
	if math.IsInf(r, 0) || r != r || x != x || y != y {
		return // not a mark anything can point at
	}
	idx := int32(len(g.items))
	g.items = append(g.items, gridItem{id, x, y, r})
	x0, y0 := g.coord(x-r), g.coord(y-r)
	x1, y1 := g.coord(x+r), g.coord(y+r)
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			k := gridKey{cx, cy}
			g.cells[k] = append(g.cells[k], idx)
		}
	}
}

// At returns the mark whose circle contains (x, y). Of several, it returns
// the one whose center is nearest, and of those equally near, the one
// inserted last (drawn on top, when marks are drawn in insertion order).
func (g *Grid) At(x, y float64) (id int, ok bool) {
	best := math.Inf(1)
	for _, idx := range g.cells[gridKey{g.coord(x), g.coord(y)}] {
		it := &g.items[idx]
		dx, dy := x-it.x, y-it.y
		d := dx*dx + dy*dy
		if d <= it.r*it.r && d <= best {
			best, id, ok = d, it.id, true
		}
	}
	return id, ok
}

// Len reports how many marks were inserted since the last Reset.
func (g *Grid) Len() int { return len(g.items) }

// Reset removes every mark, keeping the memory for the next scene.
func (g *Grid) Reset() {
	g.items = g.items[:0]
	for k, v := range g.cells {
		g.cells[k] = v[:0]
	}
}

// coord is the cell a coordinate falls in. Coordinates far outside any
// canvas are clamped, so a stray value can't file a mark under billions of
// cells.
func (g *Grid) coord(v float64) int32 {
	c := math.Floor(v / g.cell)
	switch {
	case c != c: // NaN
		return 0
	case c < -1<<20:
		return -1 << 20
	case c > 1<<20:
		return 1 << 20
	}
	return int32(c)
}
