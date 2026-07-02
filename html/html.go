package html

import (
	"fmt"
	"goowee/core"
	"goowee/hooks"
	"math"
)

type Props = map[string]any

func Element(tag string, props Props, children ...core.Node) *core.ElementNode {
	return &core.ElementNode{Tag: tag, Props: props, Children: children}
}

func Text(value string) *core.TextNode {
	return &core.TextNode{Value: value}
}

func Fragment(children ...core.Node) *core.FragmentNode {
	return &core.FragmentNode{Children: children}
}

// VirtualList renders only the visible items in a scrollable viewport,
// keeping DOM nodes proportional to the viewport regardless of list size.
// Takes a signal so the list reactively updates when items change.
func VirtualList[T any](itemsSig *core.Signal[[]T], itemHeight int, renderItem func(int, T) core.Node, opts ...VirtualListOption) core.Node {
	cfg := virtualListConfig{
		height:   400,
		overscan: 5,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return core.Component("VirtualList", func() core.Node {
		scrollTop, setScrollTop := hooks.UseState(0.0)

		return hooks.UseScope(func() core.Node {
			items := itemsSig.Get()

			st := scrollTop.Get()
			startIdx := int(st / float64(itemHeight))
			startIdx -= cfg.overscan
			if startIdx < 0 {
				startIdx = 0
			}

			visibleCount := int(math.Ceil(float64(cfg.height)/float64(itemHeight))) + cfg.overscan*2
			endIdx := startIdx + visibleCount
			if endIdx > len(items) {
				endIdx = len(items)
			}

			topPad := startIdx * itemHeight
			remaining := len(items) - endIdx
			bottomPad := remaining * itemHeight
			if bottomPad < 0 {
				bottomPad = 0
			}

			var children []core.Node
			if topPad > 0 {
				children = append(children, &core.ElementNode{
					Tag: "div",
					Props: Props{
						"style": fmt.Sprintf("height:%dpx;flex-shrink:0;", topPad),
					},
				})
			}

			for i := startIdx; i < endIdx; i++ {
				children = append(children, renderItem(i, items[i]))
			}

			if bottomPad > 0 {
				children = append(children, &core.ElementNode{
					Tag: "div",
					Props: Props{
						"style": fmt.Sprintf("height:%dpx;flex-shrink:0;", bottomPad),
					},
				})
			}

			return &core.ElementNode{
				Tag: "div",
				Props: Props{
					"style": fmt.Sprintf("overflow-y:auto;height:%dpx;", cfg.height),
					"onscroll": func(ed core.EventData) {
						if st, ok := ed.Data["scrollTop"].(float64); ok {
							setScrollTop(st)
						}
					},
				},
				Children: children,
			}
		}, scrollTop, itemsSig)
	})
}

type virtualListConfig struct {
	height   int
	overscan int
}

type VirtualListOption func(*virtualListConfig)

func VirtualListHeight(h int) VirtualListOption {
	return func(c *virtualListConfig) { c.height = h }
}

func VirtualListOverscan(o int) VirtualListOption {
	return func(c *virtualListConfig) { c.overscan = o }
}

func Div(props Props, children ...core.Node) *core.ElementNode      { return Element("div", props, children...) }
func Span(props Props, children ...core.Node) *core.ElementNode     { return Element("span", props, children...) }
func P(props Props, children ...core.Node) *core.ElementNode        { return Element("p", props, children...) }
func H1(props Props, children ...core.Node) *core.ElementNode       { return Element("h1", props, children...) }
func H2(props Props, children ...core.Node) *core.ElementNode       { return Element("h2", props, children...) }
func H3(props Props, children ...core.Node) *core.ElementNode       { return Element("h3", props, children...) }
func H4(props Props, children ...core.Node) *core.ElementNode       { return Element("h4", props, children...) }
func H5(props Props, children ...core.Node) *core.ElementNode       { return Element("h5", props, children...) }
func H6(props Props, children ...core.Node) *core.ElementNode       { return Element("h6", props, children...) }
func A(props Props, children ...core.Node) *core.ElementNode        { return Element("a", props, children...) }
func Ul(props Props, children ...core.Node) *core.ElementNode       { return Element("ul", props, children...) }
func Ol(props Props, children ...core.Node) *core.ElementNode       { return Element("ol", props, children...) }
func Li(props Props, children ...core.Node) *core.ElementNode       { return Element("li", props, children...) }
func Form(props Props, children ...core.Node) *core.ElementNode     { return Element("form", props, children...) }
func Input(props Props) *core.ElementNode                           { return Element("input", props) }
func Button(props Props, children ...core.Node) *core.ElementNode   { return Element("button", props, children...) }
func Label(props Props, children ...core.Node) *core.ElementNode    { return Element("label", props, children...) }
func Select(props Props, children ...core.Node) *core.ElementNode   { return Element("select", props, children...) }
func Option(props Props, children ...core.Node) *core.ElementNode   { return Element("option", props, children...) }
func Textarea(props Props, children ...core.Node) *core.ElementNode { return Element("textarea", props, children...) }
func Nav(props Props, children ...core.Node) *core.ElementNode      { return Element("nav", props, children...) }
func Main(props Props, children ...core.Node) *core.ElementNode     { return Element("main", props, children...) }
func Section(props Props, children ...core.Node) *core.ElementNode  { return Element("section", props, children...) }
func Header(props Props, children ...core.Node) *core.ElementNode   { return Element("header", props, children...) }
func Footer(props Props, children ...core.Node) *core.ElementNode   { return Element("footer", props, children...) }
func Article(props Props, children ...core.Node) *core.ElementNode  { return Element("article", props, children...) }
func Aside(props Props, children ...core.Node) *core.ElementNode    { return Element("aside", props, children...) }
func Img(props Props) *core.ElementNode                             { return Element("img", props) }
func Br() *core.ElementNode                                         { return Element("br", nil) }
func Hr() *core.ElementNode                                         { return Element("hr", nil) }
func Strong(props Props, children ...core.Node) *core.ElementNode   { return Element("strong", props, children...) }
func Em(props Props, children ...core.Node) *core.ElementNode       { return Element("em", props, children...) }
func Code(props Props, children ...core.Node) *core.ElementNode     { return Element("code", props, children...) }
func Pre(props Props, children ...core.Node) *core.ElementNode      { return Element("pre", props, children...) }
func Table(props Props, children ...core.Node) *core.ElementNode    { return Element("table", props, children...) }
func Thead(props Props, children ...core.Node) *core.ElementNode    { return Element("thead", props, children...) }
func Tbody(props Props, children ...core.Node) *core.ElementNode    { return Element("tbody", props, children...) }
func Tr(props Props, children ...core.Node) *core.ElementNode       { return Element("tr", props, children...) }
func Th(props Props, children ...core.Node) *core.ElementNode       { return Element("th", props, children...) }
func Td(props Props, children ...core.Node) *core.ElementNode       { return Element("td", props, children...) }
