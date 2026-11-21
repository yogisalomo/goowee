//go:build js && wasm

// Command router is the app test/e2e/router.mjs drives (#142, #155): a Watch on
// the router's path and query records each (path, x) pair it sees into #log,
// so the test can check that back and forward never show a mixed URL; buttons
// navigate and set query parameters. Each route is tall enough to scroll, has
// a heading to receive focus, and the nav links exercise active state.
package main

import (
	"net/url"
	"strings"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

func page(title string, middle core.Node) core.Node {
	return Div(
		H1(ID("heading"), Text(title)),
		Div(Style("height:2000px"), Text("scroll space")),
		middle,
		Div(Style("height:1200px"), Text("more scroll space")),
	)
}

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
			Nav(ID("nav"),
				r.NavLink("/a", "A", ID("nav-a")),
				r.NavLink("/docs", "Docs", ID("nav-docs")),
			),
			Button(ID("go-b"), OnClick(func() { r.Navigate("/b?x=2") }), Text("b")),
			Button(ID("go-c"), OnClick(func() { r.Navigate("/c?x=3#end") }), Text("c")),
			Button(ID("go-docs"), OnClick(func() { r.Navigate("/docs#install") }), Text("docs fragment")),
			Button(ID("go-top"), OnClick(func() { r.Navigate("#heading") }), Text("fragment only")),
			Input(ID("search"), Placeholder("search"), OnInput(func(v string) {
				r.NavigateReplace("?q=" + url.QueryEscape(v))
			})),
			Button(ID("set-y"), OnClick(func() { r.SetQueryParam("y", "1") }), Text("y")),
			Button(ID("set-a"), OnClick(func() { r.SetQueryParam("a", "0") }), Text("a")),
			Button(ID("clear"), OnClick(func() {
				core.Batch(func() {
					r.SetQueryParam("a", "")
					r.SetQueryParam("x", "")
					r.SetQueryParam("y", "")
				})
			}), Text("clear")),
			Switch(r.Path, map[string]func() core.Node{
				"/a": func() core.Node { return page("Page A", Div(ID("end-a"), Text("end of A"))) },
				"/b": func() core.Node { return page("Page B", Div(ID("end-b"), Text("end of B"))) },
				"/c": func() core.Node { return page("Page C", Div(ID("end"), Text("end of C"))) },
				"/docs": func() core.Node {
					return Div(
						H1(ID("heading"), Text("Docs")),
						Div(Style("height:1600px"), Text("scroll space")),
						H2(ID("install"), Text("Install")),
						Div(Style("height:1200px"), Text("more scroll space")),
					)
				},
			}, func() core.Node { return page("Not found", Text("404")) }),
		)
	}))
}
