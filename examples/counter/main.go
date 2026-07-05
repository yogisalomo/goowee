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
	r := router.New(router.CurrentPath())
	r.BindHistory()

	renderer := dom.New()
	// If the page was server-rendered (its nodes carry data-node-id), hydrate:
	// claim that DOM instead of rebuilding it.
	if js.Global().Get("document").Call("querySelector", "[data-node-id]").Truthy() {
		renderer.SetHydrating(true)
	}
	muts, _ := renderer.Render(app.App(r))
	renderer.Scheduler.Enqueue(muts...)
	bridge.Init(renderer.Scheduler, renderer.Registry)
	select {}
}
