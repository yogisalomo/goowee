package canvas

import (
	"strconv"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

type config struct {
	w, h     float64
	sized    bool
	deps     []core.SignalAccessor
	draw     func(c *Ctx)
	label    string
	attrs    []core.Item
	fallback []core.Node
	noClear  bool
}

// Option configures a View.
type Option func(*config)

// Size sets the canvas's size in CSS pixels. The runtime gives it a backing
// store of that size times the screen's device pixel ratio, so drawings stay
// sharp; draw functions always work in CSS pixels.
func Size(w, h float64) Option {
	return func(c *config) { c.w, c.h, c.sized = w, h, true }
}

// Draw sets the function that records the picture. It runs once the canvas is
// in the document, and again whenever one of deps changes; each run records a
// whole picture (the canvas is cleared first, unless NoClear). It runs on the
// render loop, so it may read signals — the ones it reads belong in deps.
// It never runs during server rendering.
func Draw(deps []core.SignalAccessor, fn func(c *Ctx)) Option {
	return func(c *config) { c.deps, c.draw = deps, fn }
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
// It panics without a Size or a Draw. On the server it renders the empty
// <canvas> (with its label and fallback); the picture is drawn once the app
// runs in the browser. In gooweetest, Element.Canvas returns what was drawn.
func View(opts ...Option) core.Node {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	if !cfg.sized {
		panic("canvas: View needs a Size(width, height)")
	}
	if cfg.draw == nil {
		panic("canvas: View needs a Draw(deps, fn)")
	}
	if cfg.w < 0 || cfg.h < 0 {
		panic("canvas: View's Size can't be negative")
	}
	var flags uint32 = FlagClear
	if cfg.noClear {
		flags = 0
	}
	style := "width:" + px(cfg.w) + ";height:" + px(cfg.h)
	return core.Component("canvas.View", func() core.Node {
		ref := h.Ref()
		ctx := &Ctx{}
		hooks.UseEffect(cfg.deps, func() func() {
			ctx.begin(cfg.w, cfg.h, flags)
			cfg.draw(ctx)
			ref.Draw(ctx.buf)
			return nil
		})
		items := make([]core.Item, 0, 4+len(cfg.attrs)+len(cfg.fallback))
		items = append(items, h.RefTo(ref), h.Style(style))
		if cfg.label != "" {
			items = append(items, h.Role("img"), h.AriaLabel(cfg.label))
		}
		items = append(items, cfg.attrs...)
		items = append(items, h.Nodes(cfg.fallback)...)
		return h.Canvas(items...)
	})
}

func px(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "px" }
