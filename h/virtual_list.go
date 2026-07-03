package h

import (
	"fmt"
	"goowee/core"
	"goowee/hooks"
	"math"
)

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
				children = append(children,
					Div(Style(fmt.Sprintf("height:%dpx;flex-shrink:0;", topPad))))
			}

			for i := startIdx; i < endIdx; i++ {
				children = append(children, renderItem(i, items[i]))
			}

			if bottomPad > 0 {
				children = append(children,
					Div(Style(fmt.Sprintf("height:%dpx;flex-shrink:0;", bottomPad))))
			}

			return Div(
				Style(fmt.Sprintf("overflow-y:auto;height:%dpx;", cfg.height)),
				OnScrollE(func(e core.EventData) {
					setScrollTop(e.ScrollTop())
				}),
				Group(childrenToItems(children)...),
			)
		}, scrollTop, itemsSig)
	})
}

func childrenToItems(children []core.Node) []core.Item {
	items := make([]core.Item, len(children))
	for i, c := range children {
		items[i] = itemWrapper{c}
	}
	return items
}

type itemWrapper struct {
	node core.Node
}

func (w itemWrapper) Apply(el *core.ElementNode) {
	if w.node == nil {
		return
	}
	el.Children = append(el.Children, w.node)
}
