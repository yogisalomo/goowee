//go:build js && wasm

// Command notes is the notes example's client; examples/notes/server serves
// it, server-rendered, with the API.
package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/examples/notes/app"
	"github.com/yogisalomo/goowee/router"
)

func main() {
	r := router.New(router.CurrentPath())
	r.BindHistory()
	bridge.Run(app.App(r))
}
