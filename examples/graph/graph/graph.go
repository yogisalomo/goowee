// Package graph is the goowee/canvas demo of #159: a graph of 10,000 nodes
// and 10,000 edges redrawn every frame with canvas.Animate, nodes grouped by
// color into one path each, and the node under the pointer found with
// canvas.Grid. Positions are closed-form (each node circles its cluster's
// center), so a frame measures drawing, not a simulation.
//
// examples/graph runs it on its own page; the tutorial site shows it at
// /graph; test/e2e/canvasbench.mjs measures it.
package graph

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/yogisalomo/goowee/canvas"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// Sizes of the demo graph.
const (
	NodeCount = 10000
	EdgeCount = 10000
	clusters  = 48
)

var colors = [...]string{"#00add8", "#ce3262", "#fddd00", "#5dc9e2"}

// layout is the graph's fixed shape: where each node circles, how fast, and
// which nodes the edges join. Coordinates are fractions of the canvas.
type layout struct {
	cx, cy  []float64 // the node's cluster center
	radius  []float64 // the node's distance from it
	angle   []float64 // where it starts on its circle
	speed   []float64 // radians per second
	group   []uint8   // color group
	edges   []int32   // pairs of node indexes
	degree  []int32   // edges per node
	cluster []int32
}

func newLayout() *layout {
	l := &layout{
		cx: make([]float64, NodeCount), cy: make([]float64, NodeCount),
		radius: make([]float64, NodeCount), angle: make([]float64, NodeCount), speed: make([]float64, NodeCount),
		group: make([]uint8, NodeCount), cluster: make([]int32, NodeCount),
		edges: make([]int32, 0, 2*EdgeCount), degree: make([]int32, NodeCount),
	}
	rng := uint32(2463534242) // xorshift: the same graph every time
	rand := func() float64 {
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		return float64(rng) / (1 << 32)
	}
	var groupOf [clusters]uint8 // each cluster's color, mixed so neighbors rarely match
	for k := range groupOf {
		groupOf[k] = uint8((k%8 + 3*(k/8)) % len(colors))
	}
	cols := 8
	rows := clusters / cols
	per := NodeCount / clusters
	for i := 0; i < NodeCount; i++ {
		k := min(i/per, clusters-1)
		col, row := k%cols, k/cols
		l.cluster[i] = int32(k)
		l.cx[i] = (float64(col) + 0.5 + 0.3*(rand()-0.5)) / float64(cols)
		l.cy[i] = (float64(row) + 0.5 + 0.3*(rand()-0.5)) / float64(rows)
		j := i - k*per
		l.radius[i] = 0.05 * math.Sqrt((float64(j)+0.5)/float64(per)) // a sunflower: even density
		l.angle[i] = float64(j) * 2.399963
		l.speed[i] = (0.15 + 0.25*rand()) * (1 - 2*float64(k%2)) // neighbors turn opposite ways
		l.group[i] = groupOf[k]
	}
	for e := 0; e < EdgeCount; e++ {
		a := e % NodeCount
		k := int(l.cluster[a])
		b := k*per + int(rand()*float64(per))
		if rand() < 0.04 { // a few edges reach a neighboring cluster
			b = (b + per) % NodeCount
		}
		l.edges = append(l.edges, int32(a), int32(b))
		l.degree[a]++
		l.degree[b]++
	}
	return l
}

// frame holds what one picture needs, reused from frame to frame so drawing
// doesn't allocate.
type frame struct {
	pos    []float32 // x, y per node, in CSS pixels
	groups [len(colors)][]float32
	segs   []float32
	hot    []float32
}

// View is the demo: the animated graph, filling its parent, with a line of
// numbers under it. The parent needs a height.
func View() core.Node {
	return core.Component("graph.View", func() core.Node {
		l := newLayout()
		f := &frame{pos: make([]float32, 2*NodeCount), segs: make([]float32, 0, 4*EdgeCount)}
		grid := canvas.NewGrid(14)
		hovered := -1
		var elapsed time.Duration
		stats, setStats := hooks.UseState("")
		info, setInfo := hooks.UseState("Point at a node to see its edges.")
		var recordSum time.Duration
		var intervalSum time.Duration
		frames := 0

		draw := func(c *canvas.Ctx, dt time.Duration) {
			start := time.Now()
			elapsed += min(dt, 100*time.Millisecond) // a hidden tab doesn't jump ahead
			t := elapsed.Seconds()
			w, h := c.Width(), c.Height()
			// A cluster fills most of its cell: its outermost node (radius
			// 0.05) circles at 0.42 of the cell's smaller side.
			scale := 0.42 * min(w/8, h/6) / 0.05
			dot := float32(max(1.2, math.Min(w, h)/420))
			for g := range f.groups {
				f.groups[g] = f.groups[g][:0]
			}
			for i := 0; i < NodeCount; i++ {
				a := l.angle[i] + t*l.speed[i]
				x := float32(l.cx[i]*w + l.radius[i]*scale*math.Cos(a))
				y := float32(l.cy[i]*h + l.radius[i]*scale*math.Sin(a))
				f.pos[2*i], f.pos[2*i+1] = x, y
				g := l.group[i]
				f.groups[g] = append(f.groups[g], x, y, dot)
			}
			f.segs = f.segs[:0]
			for e := 0; e < len(l.edges); e += 2 {
				a, b := l.edges[e], l.edges[e+1]
				f.segs = append(f.segs, f.pos[2*a], f.pos[2*a+1], f.pos[2*b], f.pos[2*b+1])
			}

			c.FillStyle("#0b1020")
			c.FillRect(0, 0, w, h)
			c.StrokeStyle("rgba(160, 190, 255, 0.18)")
			c.LineWidth(0.6)
			c.BeginPath()
			c.Segments(f.segs)
			c.Stroke()
			for g, color := range colors {
				c.FillStyle(color) // one path and one Fill per color
				c.BeginPath()
				c.Circles(f.groups[g])
				c.Fill()
			}
			if hovered >= 0 {
				f.hot = f.hot[:0]
				for e := 0; e < len(l.edges); e += 2 {
					if a, b := int(l.edges[e]), int(l.edges[e+1]); a == hovered || b == hovered {
						f.hot = append(f.hot, f.pos[2*a], f.pos[2*a+1], f.pos[2*b], f.pos[2*b+1])
					}
				}
				c.StrokeStyle("#ffffff")
				c.LineWidth(1.5)
				c.BeginPath()
				c.Segments(f.hot)
				c.Stroke()
				c.BeginPath()
				c.Arc(float64(f.pos[2*hovered]), float64(f.pos[2*hovered+1]), float64(dot)+4, 0, 2*math.Pi)
				c.Stroke()
			}

			recordSum += time.Since(start)
			intervalSum += dt
			frames++
			if frames == 30 {
				fps := 0.0
				if intervalSum > 0 {
					fps = float64(frames) / intervalSum.Seconds()
				}
				setStats(fmt.Sprintf("%s nodes, %s edges, %d KB per frame, recorded in %.2f ms, %.0f fps",
					thousands(NodeCount), thousands(EdgeCount), len(c.Commands())*4/1024,
					float64(recordSum.Microseconds())/1000/float64(frames), fps))
				recordSum, intervalSum, frames = 0, 0, 0
			}
		}

		pick := func(e core.EventData) {
			grid.Reset()
			for i := 0; i < NodeCount; i++ {
				grid.Insert(i, float64(f.pos[2*i]), float64(f.pos[2*i+1]), 6)
			}
			id, ok := grid.At(e.OffsetX(), e.OffsetY())
			if !ok {
				id = -1
			}
			if id == hovered {
				return
			}
			hovered = id
			if id < 0 {
				setInfo("Point at a node to see its edges.")
				return
			}
			setInfo(fmt.Sprintf("Node %d, cluster %d, %d edges.", id, l.cluster[id], l.degree[id]))
		}

		return Div(Class("graph"), Style("display:flex;flex-direction:column;gap:8px;height:100%"),
			Div(Style("flex:1;min-height:0"),
				canvas.View(
					canvas.Fill(),
					canvas.Label("A graph of 10,000 nodes in 48 clusters, each node circling its cluster's center"),
					canvas.Attrs(ID("graph"),
						OnPointerMove(pick),
						On("pointerleave", func(core.EventData) { hovered = -1; setInfo("Point at a node to see its edges.") })),
					canvas.Animate(draw),
				),
			),
			P(Class("graph-info"), Style("margin:0"), TextS(info)),
			P(Class("graph-stats"), ID("graph-stats"), Style("margin:0;font-family:monospace;font-size:13px"), TextS(stats)),
		)
	})
}

// thousands formats 12000 as "12,000".
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
