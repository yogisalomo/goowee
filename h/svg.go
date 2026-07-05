package h

import "github.com/yogisalomo/goowee/core"

func Svg(items ...core.Item) *core.ElementNode   { return ElNS("svg", core.NamespaceSVG, items...) }
func G(items ...core.Item) *core.ElementNode      { return ElNS("g", core.NamespaceSVG, items...) }
func Path(items ...core.Item) *core.ElementNode    { return ElNS("path", core.NamespaceSVG, items...) }
func Circle(items ...core.Item) *core.ElementNode  { return ElNS("circle", core.NamespaceSVG, items...) }
func Ellipse(items ...core.Item) *core.ElementNode { return ElNS("ellipse", core.NamespaceSVG, items...) }
func Rect(items ...core.Item) *core.ElementNode    { return ElNS("rect", core.NamespaceSVG, items...) }
func Line(items ...core.Item) *core.ElementNode    { return ElNS("line", core.NamespaceSVG, items...) }
func Polyline(items ...core.Item) *core.ElementNode { return ElNS("polyline", core.NamespaceSVG, items...) }
func Polygon(items ...core.Item) *core.ElementNode  { return ElNS("polygon", core.NamespaceSVG, items...) }
func SvgText(items ...core.Item) *core.ElementNode  { return ElNS("text", core.NamespaceSVG, items...) }
func Tspan(items ...core.Item) *core.ElementNode    { return ElNS("tspan", core.NamespaceSVG, items...) }
func Use(items ...core.Item) *core.ElementNode      { return ElNS("use", core.NamespaceSVG, items...) }
func Defs(items ...core.Item) *core.ElementNode     { return ElNS("defs", core.NamespaceSVG, items...) }
func ClipPath(items ...core.Item) *core.ElementNode { return ElNS("clipPath", core.NamespaceSVG, items...) }
func Mask(items ...core.Item) *core.ElementNode     { return ElNS("mask", core.NamespaceSVG, items...) }
func LinearGradient(items ...core.Item) *core.ElementNode {
	return ElNS("linearGradient", core.NamespaceSVG, items...)
}
func RadialGradient(items ...core.Item) *core.ElementNode {
	return ElNS("radialGradient", core.NamespaceSVG, items...)
}
func Stop(items ...core.Item) *core.ElementNode { return ElNS("stop", core.NamespaceSVG, items...) }

func ViewBox(v string) core.Item    { return attrItem{"viewBox", v} }
func Fill(v string) core.Item       { return attrItem{"fill", v} }
func Stroke(v string) core.Item     { return attrItem{"stroke", v} }
func StrokeWidth(v string) core.Item { return attrItem{"stroke-width", v} }
func D(v string) core.Item          { return attrItem{"d", v} }
func Cx(v string) core.Item         { return attrItem{"cx", v} }
func Cy(v string) core.Item         { return attrItem{"cy", v} }
func R(v string) core.Item          { return attrItem{"r", v} }
func Rx(v string) core.Item         { return attrItem{"rx", v} }
func Ry(v string) core.Item         { return attrItem{"ry", v} }
func SvgX(v string) core.Item       { return attrItem{"x", v} }
func SvgY(v string) core.Item       { return attrItem{"y", v} }
func Dx(v string) core.Item         { return attrItem{"dx", v} }
func Dy(v string) core.Item         { return attrItem{"dy", v} }
func Points(v string) core.Item     { return attrItem{"points", v} }
func Transform(v string) core.Item  { return attrItem{"transform", v} }
func FillOpacity(v string) core.Item  { return attrItem{"fill-opacity", v} }
func StrokeOpacity(v string) core.Item { return attrItem{"stroke-opacity", v} }
func StrokeLinecap(v string) core.Item { return attrItem{"stroke-linecap", v} }
func StrokeLinejoin(v string) core.Item { return attrItem{"stroke-linejoin", v} }
func PathLength(v string) core.Item { return attrItem{"pathLength", v} }
