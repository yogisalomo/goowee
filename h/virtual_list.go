package h

import (
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"strconv"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

// VirtualList renders only the rows near the viewport. The component's body
// runs once (it is setup, not render); a scroll signal drives a small reactive
// region that swaps the visible window of keyed row components.
//
// The second argument is either an int item height (the legacy form, which
// keys rows by index) or a key function func(T) K. Use the key form whenever
// items can be inserted, removed or reordered: rows are cached and reused by
// key, so a stateful row keeps its state when items move around it.
//
//	VirtualList(items, func(t Todo) int64 { return t.ID },
//		func(i int, t Todo) core.Node { return Li(Text(t.Title)) })
//
// For rows with different heights, pass VirtualListMeasure(): the runtime's
// ResizeObserver reports the real size of each mounted row and the scroll bar
// is kept in sync with a Fenwick prefix-sum tree.
type VirtualListOption func(*virtualListConfig)

type virtualListConfig struct {
	height        int
	overscan      int
	itemHeight    float64
	horizontal    bool
	measure       bool
	windowScroll  bool
	stickToBottom bool
	onEndReached  func()
	keyFn         any // func(T) K, overriding keyOrHeight
	onHandle      any // func(*VirtualListHandle[T])
}

// VirtualListHeight sets the viewport height in CSS pixels (default 400).
func VirtualListHeight(height int) VirtualListOption {
	return func(c *virtualListConfig) { c.height = height }
}

// VirtualListItemHeight sets the estimated item size in CSS pixels. It is
// required when the key form of VirtualList is used without
// VirtualListMeasure(); the legacy int second argument sets it directly.
func VirtualListItemHeight(height float64) VirtualListOption {
	return func(c *virtualListConfig) { c.itemHeight = height }
}

// VirtualListWindow is an alias for VirtualListHeight.
func VirtualListWindow(height int) VirtualListOption { return VirtualListHeight(height) }

// VirtualListOverscan sets how many rows to render beyond each edge of the
// viewport (default 5).
func VirtualListOverscan(n int) VirtualListOption {
	return func(c *virtualListConfig) {
		if n >= 0 {
			c.overscan = n
		}
	}
}

// VirtualListHorizontal measures/scrolls along the x axis.
func VirtualListHorizontal() VirtualListOption {
	return func(c *virtualListConfig) { c.horizontal = true }
}

// VirtualListWindowScroll uses the browser window (not the list element) as
// the scroll container. Useful for a page-length list; ScrollToIndex still
// works through core.ScrollWindow.
func VirtualListWindowScroll() VirtualListOption {
	return func(c *virtualListConfig) { c.windowScroll = true }
}

// VirtualListMeasure asks the runtime to observe mounted rows and use their
// real sizes instead of a fixed item height.
func VirtualListMeasure() VirtualListOption {
	return func(c *virtualListConfig) { c.measure = true }
}

// VirtualListStickToBottom keeps the list pinned to the end while the user is
// already at the end (useful for logs and chat streams).
func VirtualListStickToBottom() VirtualListOption {
	return func(c *virtualListConfig) { c.stickToBottom = true }
}

// VirtualListOnEndReached fires once each time the visible window reaches the
// last item. Fetch the next page inside it by updating the items signal.
func VirtualListOnEndReached(fn func()) VirtualListOption {
	return func(c *virtualListConfig) { c.onEndReached = fn }
}

// VirtualListKeyFunc supplies the key function as an option instead of the
// second argument to VirtualList.
func VirtualListKeyFunc[T any, K comparable](fn func(T) K) VirtualListOption {
	return func(c *virtualListConfig) { c.keyFn = fn }
}

// VirtualListHandle is filled in by VirtualListOnHandle. It is safe to call
// after mount (store it in a variable and use it from a click handler).
type VirtualListHandle[T any] struct {
	toIndex func(int)
	toKey   func(string)
	refresh func()
}

func (h *VirtualListHandle[T]) ScrollToIndex(i int) {
	if h != nil && h.toIndex != nil {
		h.toIndex(i)
	}
}

func (h *VirtualListHandle[T]) ScrollToKey(key string) {
	if h != nil && h.toKey != nil {
		h.toKey(key)
	}
}

// Refresh recomputes the visible window and re-measures mounted rows.
func (h *VirtualListHandle[T]) Refresh() {
	if h != nil && h.refresh != nil {
		h.refresh()
	}
}

// VirtualListOnHandle receives the list's imperative handle.
func VirtualListOnHandle[T any](fn func(*VirtualListHandle[T])) VirtualListOption {
	return func(c *virtualListConfig) { c.onHandle = fn }
}

type virtualRange struct{ start, end int }

type virtualRowProps[T any] struct {
	Index int
	Item  T
}

type fenwick struct {
	n        int
	fallback float64
	tree     []float64 // 1-based
}

func newFenwick(n int, fallback float64) *fenwick {
	f := &fenwick{n: n, fallback: fallback, tree: make([]float64, n+1)}
	if fallback > 0 {
		for i := 1; i <= n; i++ {
			f.tree[i] += fallback
			if j := i + (i & -i); j <= n {
				f.tree[j] += f.tree[i]
			}
		}
	}
	return f
}

func (f *fenwick) Set(i int, v float64) {
	if i < 0 || i >= f.n {
		return
	}
	delta := v - f.at(i)
	for j := i + 1; j <= f.n; j += j & -j {
		f.tree[j] += delta
	}
}

func (f *fenwick) at(i int) float64 { return f.prefix(i+1) - f.prefix(i) }

func (f *fenwick) prefix(end int) float64 {
	if end <= 0 {
		return 0
	}
	if end > f.n {
		end = f.n
	}
	sum := 0.0
	for j := end; j > 0; j -= j & -j {
		sum += f.tree[j]
	}
	return sum
}

func (f *fenwick) Total() float64 { return f.prefix(f.n) }

// find returns the number of rows whose cumulative size is <= offset.
func (f *fenwick) find(offset float64) int {
	if f.n == 0 {
		return 0
	}
	idx := 0
	step := 1 << (bits.Len(uint(f.n)) - 1)
	for step > 0 {
		next := idx + step
		if next <= f.n && f.tree[next] <= offset {
			idx = next
			offset -= f.tree[next]
		}
		step >>= 1
	}
	return idx
}

func normalizeKey[T any](keyOrHeight any) (func(int, T) string, float64) {
	switch v := keyOrHeight.(type) {
	case int:
		return func(i int, _ T) string { return "i:" + strconv.Itoa(i) }, float64(v)
	case func(int, T) string:
		return v, 0
	}
	rv := reflect.ValueOf(keyOrHeight)
	if rv.IsValid() && rv.Kind() == reflect.Func && rv.Type().NumIn() == 1 && rv.Type().NumOut() == 1 {
		return func(_ int, item T) string {
			in := reflect.ValueOf(item)
			if !in.IsValid() || !in.Type().AssignableTo(rv.Type().In(0)) {
				return ""
			}
			return "k:" + fmt.Sprint(rv.Call([]reflect.Value{in})[0].Interface())
		}, 0
	}
	panic("h.VirtualList: second argument must be an int height or func(T) comparable")
}

// VirtualList renders a windowed list. It is backwards compatible with the
// original VirtualList[T](items, itemHeight, render, opts...) signature.
func VirtualList[T any](itemsSig *core.Signal[[]T], keyOrHeight any, render func(int, T) core.Node, opts ...VirtualListOption) core.Node {
	cfg := virtualListConfig{height: 400, overscan: 5}
	for _, o := range opts {
		o(&cfg)
	}

	rawKey := keyOrHeight
	if cfg.keyFn != nil {
		rawKey = cfg.keyFn
	}
	keyFn, fallback := normalizeKey[T](rawKey)
	if cfg.itemHeight > 0 {
		fallback = cfg.itemHeight
	}

	return core.Component("VirtualList", func() core.Node {
		listRef := Ref()
		scrollTop, setScrollTop := hooks.UseState(0.0)
		heights, setHeights := hooks.UseState([]float64{})
		rng, _ := hooks.UseState(virtualRange{})
		atBottom := core.NewSignal(true)
		visibleNodes := core.NewSignal([]core.Node{})
		rowNodes := core.NewSignal(map[string]core.Node{})

		type cachedRow struct {
			node  core.Node
			props virtualRowProps[T]
			set   func(virtualRowProps[T])
		}
		rowCache := map[string]*cachedRow{}
		tree := newFenwick(0, fallback)
		lastNotified := -1
		var handle *VirtualListHandle[T]

		count := core.Computed([]core.SignalAccessor{itemsSig}, func() int { return len(itemsSig.Get()) })

		heightAt := func(i int, hs []float64) float64 {
			if i >= 0 && i < len(hs) && hs[i] > 0 {
				return hs[i]
			}
			return fallback
		}

		applied := []float64(nil)
		rebuild := func() {
			n := count.Get()
			hs := heights.Get()
			if tree.n != n || fallback != tree.fallback {
				*tree = *newFenwick(n, fallback)
				applied = make([]float64, n)
			}
			// Only push rows whose size actually changed: with 100k rows a
			// measurement batch must not rewrite the whole tree.
			for i := 0; i < n; i++ {
				want := heightAt(i, hs)
				if applied[i] != want {
					tree.Set(i, want)
					applied[i] = want
				}
			}
		}

		recompute := func() {
			n := count.Get()
			if n == 0 {
				rng.Set(virtualRange{})
				return
			}
			top := scrollTop.Get()
			if cfg.stickToBottom && atBottom.Get() {
				top = tree.Total()
			}
			if fallback <= 0 {
				// Without a fixed estimate the measured tree is the only source;
				// find() below still works from the prefix sums.
			}
			start := tree.find(top) - cfg.overscan
			if start < 0 {
				start = 0
			}
			end := tree.find(top+float64(cfg.height)) + cfg.overscan + 1
			if end > n {
				end = n
			}
			if start > end {
				start = end
			}
			rng.Set(virtualRange{start: start, end: end})
		}

		hooks.UseEffect([]core.SignalAccessor{itemsSig, heights}, func() func() {
			rebuild()
			// Snapshot: the scrollTop effect owns atBottom updates; this
			// effect must not re-run when the user scrolls.
			if cfg.stickToBottom && atBottom.Peek() {
				setScrollTop(tree.Total())
			}
			recompute()
			return nil
		})

		hooks.UseEffect([]core.SignalAccessor{scrollTop}, func() func() {
			top := scrollTop.Get()
			atBottom.Set(top >= tree.Total()-float64(cfg.height)-16)
			recompute()
			return nil
		})

		// Build the visible list from cached, keyed row components. Reusing the
		// component node (not just its key) is what preserves row-local state.
		syncRows := func() {
			r := rng.Get()
			items := itemsSig.Get()
			nodes := make([]core.Node, 0, max(0, r.end-r.start))
			byKey := make(map[string]core.Node, cap(nodes))
			live := make(map[string]bool, cap(nodes))
			for i := r.start; i < r.end && i < len(items); i++ {
				key := keyFn(i, items[i])
				props := virtualRowProps[T]{Index: i, Item: items[i]}
				entry := rowCache[key]
				if entry == nil {
					var setProps func(virtualRowProps[T])
					node := core.ComponentWithProps("VirtualRow", props, func(p *core.Signal[virtualRowProps[T]]) core.Node {
						setProps = func(v virtualRowProps[T]) { p.Set(v) }
						index := core.Computed([]core.SignalAccessor{p}, func() int { return p.Get().Index })
						return Div(
							Class("vlist-row"),
							BindAttr("data-row-index", index),
							Attr("data-row-key", key),
							Style("flex-shrink:0;"),
							hooks.UseScope(func() core.Node {
								pr := p.Get()
								return render(pr.Index, pr.Item)
							}, p),
						)
					})
					keyed := core.SetKey(node, key)
					entry = &cachedRow{node: keyed, props: props, set: func(v virtualRowProps[T]) {
						if setProps != nil {
							setProps(v)
						}
					}}
					rowCache[key] = entry
				} else if entry.set != nil && !reflect.DeepEqual(entry.props, props) {
					entry.set(props)
					entry.props = props
				}
				nodes = append(nodes, entry.node)
				byKey[key] = entry.node
				live[key] = true
			}
			visibleNodes.Set(nodes)
			rowNodes.Set(byKey)
			if len(rowCache) > 4*len(live)+64 {
				for key := range rowCache {
					if !live[key] {
						delete(rowCache, key)
					}
				}
			}
			if r.end >= len(items) && len(items) > 0 && lastNotified != len(items) && cfg.onEndReached != nil {
				lastNotified = len(items)
				cfg.onEndReached()
			}
		}
		hooks.Watch([]core.SignalAccessor{rng, itemsSig}, syncRows)

		measureHandler := On("goowee:rowmeasure", func(e core.EventData) {
			if !cfg.measure {
				return
			}
			ms := e.Measurements()
			if len(ms) == 0 {
				return
			}
			hs := append([]float64(nil), heights.Get()...)
			for len(hs) < count.Get() {
				hs = append(hs, 0)
			}
			changed := false
			for _, m := range ms {
				size := m.Height
				if cfg.horizontal {
					size = m.Width
				}
				if m.Index >= 0 && m.Index < len(hs) && size > 0 && math.Abs(hs[m.Index]-size) > 0.5 {
					hs[m.Index] = size
					changed = true
				}
			}
			if changed {
				setHeights(hs)
			}
		})

		axis := "y"
		if cfg.horizontal {
			axis = "x"
		}
		overflow := "overflow-y:auto;overflow-x:hidden;"
		flexDir := "column"
		if cfg.horizontal {
			overflow, flexDir = "overflow-x:auto;overflow-y:hidden;", "row"
		}

		handle = &VirtualListHandle[T]{
			toIndex: func(i int) {
				if i < 0 || i >= count.Get() {
					return
				}
				off := tree.prefix(i)
				setScrollTop(off)
				if cfg.horizontal {
					core.ScrollWindow(off, -1)
				} else {
					core.ScrollWindow(-1, off)
				}
			},
			toKey: func(key string) {
				items := itemsSig.Get()
				for i, item := range items {
					if keyFn(i, item) == key {
						handle.ScrollToIndex(i)
						return
					}
				}
			},
			refresh: func() { rebuild(); recompute() },
		}
		if fn, ok := cfg.onHandle.(func(*VirtualListHandle[T])); ok {
			fn(handle)
		}

		if cfg.measure {
			// Only a measuring list asks the runtime to watch its rows. The
			// ref call runs after the first frame, when the list node exists.
			hooks.OnMount(func() func() {
				listRef.Call("gooweeObserveRows")
				return func() { listRef.Call("gooweeUnobserveRows") }
			})
		}

		sizeStyle := "height:auto;"
		if cfg.height > 0 && !cfg.windowScroll {
			sizeStyle = fmt.Sprintf("height:%dpx;", cfg.height)
		}
		attrs := []core.Item{
			Class("vlist"),
			RefTo(listRef),
			Attr("data-goowee-virtual-list", "true"),
			Attr("data-axis", axis),
		}
		if cfg.windowScroll {
			attrs = append(attrs, Attr("data-goowee-scroll", "window"))
		}
		attrs = append(attrs,
			Style(overflow+sizeStyle+fmt.Sprintf("display:flex;flex-direction:%s;", flexDir)),
			measureHandler,
			OnScrollE(func(e core.EventData) {
				top := e.ScrollTop()
				if cfg.horizontal {
					top = e.ScrollLeft()
				}
				setScrollTop(top)
			}),
		)
		if cfg.windowScroll {
			attrs = append(attrs, On("goowee:windowscroll", func(e core.EventData) {
				top := e.ScrollTop()
				if cfg.horizontal {
					top = e.ScrollLeft()
				}
				setScrollTop(top)
			}))
		}
		// Populate the first window during setup so the initial render (and SSR)
		// already contains rows; effects only handle subsequent changes.
		rebuild()
		recompute()
		syncRows()

		attrs = append(attrs, hooks.UseScope(func() core.Node {
			r := rng.Get()
			n := count.Get()
			topSize := tree.prefix(r.start)
			bottomSize := tree.Total() - tree.prefix(r.end)
			if n == 0 {
				topSize, bottomSize = 0, 0
			}
			main := "height"
			if cfg.horizontal {
				main = "width"
			}
			children := []core.Node{
				Div(Style(fmt.Sprintf("%s:%.2fpx;flex-shrink:0;", main, topSize)), Key("vlist-top")),
			}
			children = append(children, visibleNodes.Get()...)
			children = append(children, Div(Style(fmt.Sprintf("%s:%.2fpx;flex-shrink:0;", main, bottomSize)), Key("vlist-bottom")))
			return Fragment(children...)
		}, rng, heights, itemsSig, visibleNodes, count))

		return Div(attrs...)
	})
}
