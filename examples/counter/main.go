//go:build js && wasm

package main

import (
	"goowee/bridge"
	"goowee/dom"
	"goowee/examples/counter/app"
	"goowee/router"
	"syscall/js"
)

func main() {
	r := router.New(js.Global().Get("location").Get("pathname").String())
	r.SetNavFn(func(path string) {
		js.Global().Get("history").Call("pushState", nil, "", path)
	})

	js.Global().Call("addEventListener", "popstate", js.FuncOf(func(this js.Value, args []js.Value) any {
		r.Navigate(js.Global().Get("location").Get("pathname").String())
		return nil
	}))

	renderer := dom.New()
	muts, _ := renderer.Render(app.App(r))
	renderer.Scheduler.Enqueue(muts...)
	bridge.Init(renderer.Scheduler, renderer.Registry)
	select {}
}
