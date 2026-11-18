//go:build js && wasm

// Command router is the app test/e2e/router.mjs drives (#142): a Watch on the
// router's path and query records each (path, x) pair it sees into #log, so
// the test can check that back and forward never show a mixed URL; buttons
// navigate and set query parameters.
package main

import (
	"strings"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

func main() {
	r := router.New(router.CurrentPath())
	r.BindHistory()
	bridge.Run(core.Component("RouterE2E", func() core.Node {
		log, setLog := hooks.UseState([]string{r.Path.Peek() + "|" + r.QueryParam("x")})
		hooks.Watch([]core.SignalAccessor{r.Path, r.Query}, func() {
			setLog(append(append([]string{}, log.Peek()...), r.Path.Peek()+"|"+r.QueryParam("x")))
		})
		text := core.Computed([]core.SignalAccessor{log}, func() string { return strings.Join(log.Get(), ",") })
		return Div(
			Pre(ID("log"), TextS(text)),
			Span(ID("fragment"), TextS(r.Fragment)),
			Button(ID("go-b"), OnClick(func() { r.Navigate("/b?x=2") }), Text("b")),
			Button(ID("go-c"), OnClick(func() { r.Navigate("/c?x=3#end") }), Text("c")),
			Button(ID("set-y"), OnClick(func() { r.SetQueryParam("y", "1") }), Text("y")),
			Button(ID("set-a"), OnClick(func() { r.SetQueryParam("a", "0") }), Text("a")),
			Button(ID("clear"), OnClick(func() {
				core.Batch(func() {
					r.SetQueryParam("a", "")
					r.SetQueryParam("x", "")
					r.SetQueryParam("y", "")
				})
			}), Text("clear")),
		)
	}))
}
