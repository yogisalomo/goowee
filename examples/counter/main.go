//go:build js && wasm

package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/dom"
	"github.com/yogisalomo/goowee/examples/counter/app"
	"github.com/yogisalomo/goowee/router"
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
