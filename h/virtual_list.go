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

			return Div(
				Style(fmt.Sprintf("overflow-y:auto;height:%dpx;", cfg.height)),
				OnScrollE(func(e core.EventData) {
					setScrollTop(e.ScrollTop())
				}),
				If(topPad > 0, Div(
					Style(fmt.Sprintf("height:%dpx;flex-shrink:0;", topPad)),
					Key("topPad"),
				)),
				Group(renderVisible(startIdx, endIdx, items, renderItem)...),
				If(bottomPad > 0, Div(
					Style(fmt.Sprintf("height:%dpx;flex-shrink:0;", bottomPad)),
					Key("bottomPad"),
				)),
			)
		}, scrollTop, itemsSig)
	})
}

func renderVisible[T any](startIdx, endIdx int, items []T, renderItem func(int, T) core.Node) []core.Item {
	out := make([]core.Item, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		idx := i
		n := renderItem(idx, items[i])
		if el, ok := n.(*core.ElementNode); ok && el != nil {
			el.Key = idx
		}
		out = append(out, n)
	}
	return out
}
