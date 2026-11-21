package canvas

import (
	"strconv"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

type config struct {
	w, h     float64
	sized    bool
	fill     bool
	deps     []core.SignalAccessor
	draw     func(c *Ctx)
	animate  func(c *Ctx, dt time.Duration)
	label    string
	attrs    []core.Item
	fallback []core.Node
	noClear  bool
	position string // Stack's layers: placed over each other
}

// Option configures a View.
type Option func(*config)

// Options combines several options into one, for an API that takes a single
// Option (Stack's box, a layer that also sets NoClear).
func Options(opts ...Option) Option {
	return func(c *config) {
		for _, o := range opts {
			o(c)
		}
	}
}

// Size sets the canvas's size in CSS pixels. The runtime gives it a backing
// store of that size times the screen's device pixel ratio, so drawings stay
// sharp; draw functions always work in CSS pixels.
func Size(w, h float64) Option {
	return func(c *config) { c.w, c.h, c.sized = w, h, true }
}

// Fill makes the canvas follow its element's size instead of a fixed one:
// it fills its parent (width and height 100%), the runtime watches its size,
// and the picture is redrawn whenever it changes, with c.Width() and
// c.Height() giving the new size. The parent needs a height of its own (a
// fixed height, a flex or grid track) for the canvas to fill.
//
// Nothing is drawn until the canvas has been laid out with a non-zero size.
func Fill() Option {
	return func(c *config) { c.fill = true }
}

// Draw sets the function that records the picture. It runs once the canvas is
// in the document, and again whenever one of deps changes; each run records a
// whole picture (the canvas is cleared first, unless NoClear). It runs on the
// render loop, so it may read signals — the ones it reads belong in deps.
// It never runs during server rendering.
func Draw(deps []core.SignalAccessor, fn func(c *Ctx)) Option {
	return func(c *config) { c.deps, c.draw = deps, fn }
}

// Animate draws a new picture every frame, at the display's refresh rate,
// from the frame after the canvas mounts until it unmounts. dt is the time
// since the previous picture (0 for the first). Animation pauses while the
// page is hidden; the first picture after it is shown again gets the whole
// hidden time as its dt, so clamp dt when the motion integrates it.
//
// fn runs on the render loop and may read signals, but nothing is subscribed:
// every frame is drawn anyway. A View takes Draw or Animate, not both. In
// gooweetest, Screen.Frame draws one picture.
func Animate(fn func(c *Ctx, dt time.Duration)) Option {
	return func(c *config) { c.animate = fn }
}

// Label describes the picture for assistive technology: the canvas gets
// role="img" and aria-label. Give every canvas that carries information one,
// or a Fallback.
func Label(text string) Option {
	return func(c *config) { c.label = text }
}

// Attrs adds attributes and event handlers to the <canvas> element — a class,
// an id, OnPointerMove for hit testing (e.OffsetX and e.OffsetY are in the
// same CSS pixels as the drawing). The View sets the element's size through
// its style attribute, so style with a class rather than h.Style.
func Attrs(items ...core.Item) Option {
	return func(c *config) { c.attrs = append(c.attrs, items...) }
}

// Fallback renders children inside the <canvas>: shown when canvas isn't
// supported, and read by assistive technology — a table of the plotted data,
// say.
func Fallback(children ...core.Node) Option {
	return func(c *config) { c.fallback = append(c.fallback, children...) }
}

// NoClear keeps what earlier runs drew: each run draws on top of the last
// picture instead of starting from a blank canvas.
func NoClear() Option {
	return func(c *config) { c.noClear = true }
}

// View renders a <canvas> drawn from Go. Options are read once, when View is
// called, like h.VirtualList's: the draw function is captured at mount, so it
// must read signals (listed in its deps) to see new data, rather than closing
// over values that change.
//
//	canvas.View(
//	    canvas.Size(240, 100),
//	    canvas.Label("Visitors per day"),
//	    canvas.Draw([]core.SignalAccessor{visits}, func(c *canvas.Ctx) {
//	        c.FillStyle("#00add8")
//	        for i, v := range visits.Get() {
//	            c.FillRect(float64(i)*30, c.Height()-v, 24, v)
//	        }
//	    }),
//	)
//
// It needs a Size or Fill, and a Draw or Animate; it panics otherwise. On the
// server it renders the empty <canvas> (with its label and fallback); the
// picture is drawn once the app runs in the browser. In gooweetest,
// Element.Canvas returns what was drawn.
func View(opts ...Option) core.Node {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	cfg.checkSize("View")
	cfg.checkDrawing("View")
	return view(cfg)
}

func (cfg *config) checkSize(who string) {
	switch {
	case cfg.sized && cfg.fill:
		panic("canvas: " + who + " takes Size or Fill, not both")
	case !cfg.sized && !cfg.fill:
		panic("canvas: " + who + " needs a Size(width, height) or Fill()")
	case cfg.w < 0 || cfg.h < 0:
		panic("canvas: " + who + "'s Size can't be negative")
	}
}

func (cfg *config) checkDrawing(who string) {
	switch {
	case cfg.draw != nil && cfg.animate != nil:
		panic("canvas: " + who + " takes Draw or Animate, not both")
	case cfg.draw == nil && cfg.animate == nil:
		panic("canvas: " + who + " needs a Draw(deps, fn) or an Animate(fn)")
	}
}

// area is a canvas's size in CSS pixels.
type area struct{ w, h float64 }

func (a area) empty() bool { return a.w <= 0 || a.h <= 0 }

func view(cfg config) core.Node {
	var flags uint32 = FlagClear
	if cfg.noClear {
		flags = 0
	}
	style := "width:" + px(cfg.w) + ";height:" + px(cfg.h)
	if cfg.fill {
		flags |= FlagFollowSize
		style = "width:100%;height:100%;display:block"
	}
	style = cfg.position + style
	return core.Component("canvas.View", func() core.Node {
		ref := h.Ref()
		ctx := &Ctx{}
		size, setSize := hooks.UseState(area{cfg.w, cfg.h})

		// follow sends a buffer with no commands, so the runtime starts
		// watching a Fill canvas's size; its resize event sets size.
		follow := func() {
			ctx.begin(0, 0, flags)
			ref.Draw(ctx.buf)
		}
		record := func(sz area, fn func(*Ctx)) {
			ctx.begin(sz.w, sz.h, flags)
			fn(ctx)
			ref.Draw(ctx.buf)
		}

		if cfg.draw != nil {
			deps := cfg.deps
			if cfg.fill {
				deps = append(append([]core.SignalAccessor{}, cfg.deps...), size)
			}
			hooks.UseEffect(deps, func() func() {
				sz := size.Peek()
				if cfg.fill && sz.empty() {
					follow()
					return nil
				}
				record(sz, cfg.draw)
				return nil
			})
		} else {
			hooks.OnMount(func() func() {
				stopped := false
				var last time.Time
				var tick func()
				tick = func() {
					if stopped {
						return
					}
					now := time.Now()
					var dt time.Duration
					if !last.IsZero() {
						dt = now.Sub(last)
					}
					last = now
					if sz := size.Peek(); !cfg.fill || !sz.empty() {
						record(sz, func(c *Ctx) { cfg.animate(c, dt) })
					}
					core.NextFrame(tick)
				}
				if cfg.fill {
					follow()
				}
				core.NextFrame(tick)
				return func() { stopped = true }
			})
		}

		items := make([]core.Item, 0, 5+len(cfg.attrs)+len(cfg.fallback))
		items = append(items, h.RefTo(ref), h.Style(style))
		if cfg.fill {
			items = append(items, h.On(ResizeEvent, func(e core.EventData) {
				setSize(area{e.Number("width"), e.Number("height")})
			}))
		}
		if cfg.label != "" {
			items = append(items, h.Role("img"), h.AriaLabel(cfg.label))
		}
		items = append(items, cfg.attrs...)
		items = append(items, h.Nodes(cfg.fallback)...)
		return h.Canvas(items...)
	})
}

// ResizeEvent is the event the runtime dispatches on a Fill canvas when its
// size changes; its data has "width" and "height" in CSS pixels. View handles
// it. In gooweetest, Screen.Dispatch(el, canvas.ResizeEvent, ...) stands in
// for the browser's layout.
const ResizeEvent = "goowee:canvasresize"

func px(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "px" }
