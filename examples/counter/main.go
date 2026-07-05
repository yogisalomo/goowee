//go:build js && wasm

package main

import (
	"goowee/bridge"
	"goowee/dom"
	"goowee/examples/counter/app"
	"goowee/router"
)

func main() {
	r := router.New(router.CurrentPath())
	r.BindHistory()

	renderer := dom.New()
	muts, _ := renderer.Render(app.App(r))
	renderer.Scheduler.Enqueue(muts...)
	bridge.Init(renderer.Scheduler, renderer.Registry)
	select {}
}
