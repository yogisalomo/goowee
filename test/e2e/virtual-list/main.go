//go:build js && wasm

// Command virtual-list is the app test/e2e/virtual-list.mjs drives (#157):
// a 100,000-row keyed VirtualList with variable heights, stateful rows and
// ScrollToIndex. The page exposes #prepend, #scroll and #status for the test.
package main

import (
	"fmt"
	"time"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

type row struct {
	ID int
}

func main() {
	bridge.Run(core.Component("VirtualListE2E", func() core.Node {
		items := make([]row, 100000)
		for i := range items {
			items[i] = row{ID: i}
		}
		data, setData := hooks.UseState(items)
		count := core.Computed([]core.SignalAccessor{data}, func() int { return len(data.Get()) })
		bench, setBench := hooks.UseState(0.0)
		var handle *VirtualListHandle[row]

		return Div(
			Button(ID("prepend"), OnClick(func() {
				next := make([]row, 0, len(data.Get())+1)
				next = append(next, row{ID: -1})
				next = append(next, data.Get()...)
				setData(next)
			}), Text("prepend")),
			Button(ID("scroll"), OnClick(func() { handle.ScrollToIndex(50000) }), Text("scroll")),
			Button(ID("bench"), OnClick(func() {
				start := time.Now()
				for i := 0; i < 120; i++ {
					handle.ScrollToIndex(i * 800 % 100000)
					handle.Refresh()
				}
				setBench(float64(time.Since(start).Microseconds()) / 120)
			}), Text("bench")),
			Div(ID("status"), Textf("items=%d", count)),
			Div(ID("benchresult"), Textf("bench=%.1fus", bench)),
			VirtualList(data, func(r row) int { return r.ID },
				func(i int, item row) core.Node {
					return core.Component("E2ERow", func() core.Node {
						n, setN := hooks.UseState(0)
						return Div(
							Class("vl-row"),
							Attr("data-id", fmt.Sprint(item.ID)),
							Textf("item-%d#%d", item.ID, n),
							Button(Class("bump"), OnClick(func() { setN(n.Get() + 1) }), Text("+")),
						)
					})
				},
				VirtualListHeight(300),
				VirtualListItemHeight(48),
				VirtualListMeasure(),
				VirtualListOverscan(2),
				VirtualListOnHandle(func(h *VirtualListHandle[row]) { handle = h }),
				VirtualListOnEndReached(func() {}),
			),
		)
	}))
}
