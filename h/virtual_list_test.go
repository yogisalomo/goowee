package h_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	h "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

type vlItem struct {
	ID   int
	Name string
}

func statefulRow(i int, item vlItem) core.Node {
	return core.Component("StatefulRow", func() core.Node {
		n, setN := hooks.UseState(0)
		return h.Div(
			h.Class("vl-row"),
			h.Attr("data-id", fmt.Sprint(item.ID)),
			h.Textf("%s#%d", item.Name, n),
			h.Button(h.OnClick(func() { setN(n.Get() + 1) }), h.Text("+")),
		)
	})
}

func plainRow(i int, item vlItem) core.Node {
	return h.Div(h.Class("vl-row"), h.Textf("%s#%d", item.Name, i))
}

func vlApp(items *core.Signal[[]vlItem], setItems func([]vlItem), keyed bool, render func(int, vlItem) core.Node, opts ...h.VirtualListOption) core.Node {
	var list h.VirtualListHandle[vlItem]
	options := append([]h.VirtualListOption{
		h.VirtualListHeight(120),
		h.VirtualListItemHeight(30),
		h.VirtualListOverscan(1),
		h.VirtualListOnHandle(func(handle *h.VirtualListHandle[vlItem]) { list = *handle }),
	}, opts...)
	key := any(30)
	if keyed {
		key = func(it vlItem) int { return it.ID }
	}
	return h.Div(
		h.Button(h.ID("prepend"), h.OnClick(func() {
			setItems(append([]vlItem{{ID: 99, Name: "new"}}, items.Get()...))
		}), h.Text("prepend")),
		h.Button(h.ID("scroll"), h.OnClick(func() { list.ScrollToIndex(50) }), h.Text("scroll")),
		h.VirtualList(items, key, render, options...),
	)
}

func TestVirtualListKeyedRowsKeepStateOnInsertAboveViewport(t *testing.T) {
	items := make([]vlItem, 100)
	for i := range items {
		items[i] = vlItem{ID: 100 + i, Name: fmt.Sprintf("item-%d", i)}
	}
	sig := core.NewSignal(items)
	setItems := func(v []vlItem) { sig.Set(v) }
	s := gooweetest.Render(t, vlApp(sig, setItems, true, statefulRow))

	row := s.Find(".vl-row[data-id='100']")
	if row == nil {
		t.Fatalf("row 100 not mounted; HTML = %s", s.HTML())
	}
	s.Click(s.FindByText("+"))
	if !strings.Contains(row.Text(), "item-0#1") {
		t.Fatalf("counter didn't increment: %q", row.Text())
	}

	s.Click(s.Find("#prepend"))
	row = s.Find(".vl-row[data-id='100']")
	if row == nil {
		t.Fatalf("row 100 disappeared after prepend; HTML = %s", s.HTML())
	}
	if !strings.Contains(row.Text(), "item-0#1") {
		t.Fatalf("row state was lost on insert above the viewport: %q", row.Text())
	}
	if got := s.Find(".vl-row[data-id='99']"); got == nil {
		t.Fatal("new row at the top isn't mounted")
	}
}

func TestVirtualListIndexKeysKeepLegacyBehaviour(t *testing.T) {
	items := make([]vlItem, 100)
	for i := range items {
		items[i] = vlItem{ID: 100 + i, Name: fmt.Sprintf("item-%d", i)}
	}
	sig := core.NewSignal(items)
	setItems := func(v []vlItem) { sig.Set(v) }
	s := gooweetest.Render(t, vlApp(sig, setItems, false, plainRow))

	before := len(s.FindAll(".vl-row"))
	if before == 0 || before >= len(items) {
		t.Fatalf("unexpected mounted row count %d", before)
	}
	s.Click(s.Find("#prepend"))
	if got := len(s.FindAll(".vl-row")); got != before {
		t.Fatalf("mounted rows changed from %d to %d", before, got)
	}
	if !strings.Contains(s.Find(".vl-row").Text(), "new#0") {
		t.Fatalf("index-keyed row didn't adopt the new item: %q", s.Find(".vl-row").Text())
	}
}

func TestVirtualListMeasurementsUpdateSpacers(t *testing.T) {
	items := make([]vlItem, 20)
	for i := range items {
		items[i] = vlItem{ID: i, Name: fmt.Sprintf("item-%d", i)}
	}
	sig := core.NewSignal(items)
	setItems := func(v []vlItem) { sig.Set(v) }
	s := gooweetest.Render(t, vlApp(sig, setItems, true, plainRow, h.VirtualListMeasure()))

	list := s.Find(".vlist")
	spacer := list.Children()[0]
	if !strings.Contains(spacer.Attr("style"), "0.00px") {
		t.Fatalf("initial top spacer = %q", spacer.Attr("style"))
	}
	s.Dispatch(list, "goowee:rowmeasure", map[string]any{
		"rows": []any{[]any{0, 100, 80}, []any{1, 100, 120}},
	})
	// Scrolling past the first two measured rows should make the top spacer
	// reflect their real heights, not the 30px estimate.
	s.Dispatch(list, "scroll", map[string]any{"scrollTop": 250})
	spacer = s.Find(".vlist").Children()[0]
	if !strings.Contains(spacer.Attr("style"), "80.00px") && !strings.Contains(spacer.Attr("style"), "200.00px") {
		t.Fatalf("top spacer didn't use measured heights: %q", spacer.Attr("style"))
	}
}
