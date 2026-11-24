//go:build js && wasm

// Command list is the app test/e2e/list.mjs drives: an h.ForList (#140) in
// HTML and in <svg>, changed by buttons through each core.List operation.
package main

import (
	"strconv"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
)

type item struct {
	ID   int
	Text string
}

func main() {
	list := core.NewList(func(i item) int { return i.ID }, item{1, "a"}, item{2, "b"}, item{3, "c"})
	next := 4
	text := func(it *core.Signal[item]) *core.Signal[string] {
		return core.Computed([]core.SignalAccessor{it}, func() string { return strconv.Itoa(it.Get().ID) + it.Get().Text })
	}
	bridge.Run(core.Component("ListE2E", func() core.Node {
		return Div(
			Button(ID("append"), OnClick(func() { list.Append(item{next, "n"}); next++ }), Text("append")),
			Button(ID("prepend"), OnClick(func() { list.Insert(0, item{next, "p"}); next++ }), Text("prepend")),
			Button(ID("update"), OnClick(func() { list.Update(2, func(i *item) { i.Text += "!" }) }), Text("update")),
			Button(ID("remove"), OnClick(func() { list.Remove(1) }), Text("remove")),
			Button(ID("move"), OnClick(func() { list.Move(3, 0) }), Text("move")),
			Button(ID("set"), OnClick(func() { list.Set([]item{{2, "B"}, {9, "z"}}) }), Text("set")),
			Ul(ID("rows"), ForList(list, func(it *core.Signal[item]) core.Node { return Li(TextS(text(it))) })),
			Svg(ID("dots"), ForList(list, func(it *core.Signal[item]) core.Node {
				return El("circle", BindAttr("data-t", text(it)))
			})),
		)
	}))
}
