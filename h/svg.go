package h

import "github.com/yogisalomo/goowee/core"

// Svg is the root of an inline SVG subtree. It carries the SVG namespace, which
// descendants inherit — so children built with SvgEl / Path / Circle / etc.
// (or even plain El) render as SVG elements. Set attributes with the usual
// helpers: Attr("viewBox", ...), Attr("d", ...), Attr("fill", ...).
func Svg(items ...core.Item) *core.ElementNode {
	el := El("svg", items...)
	el.Namespace = core.SVGNamespace
	return el
}

// SvgEl builds an arbitrary SVG child element (e.g. "text", "use", "defs").
// The namespace is inherited from the enclosing Svg at render time.
func SvgEl(tag string, items ...core.Item) *core.ElementNode { return El(tag, items...) }

// Common SVG shape elements (names that don't collide with the HTML helpers).
func Path(items ...core.Item) *core.ElementNode     { return El("path", items...) }
func Circle(items ...core.Item) *core.ElementNode   { return El("circle", items...) }
func Rect(items ...core.Item) *core.ElementNode     { return El("rect", items...) }
func Ellipse(items ...core.Item) *core.ElementNode  { return El("ellipse", items...) }
func Line(items ...core.Item) *core.ElementNode     { return El("line", items...) }
func Polyline(items ...core.Item) *core.ElementNode { return El("polyline", items...) }
func Polygon(items ...core.Item) *core.ElementNode  { return El("polygon", items...) }
func G(items ...core.Item) *core.ElementNode        { return El("g", items...) }
