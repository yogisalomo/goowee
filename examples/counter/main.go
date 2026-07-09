//go:build js && wasm

package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/examples/counter/app"
	"github.com/yogisalomo/goowee/router"
)

func main() {
	r := router.New(router.CurrentPath())
	r.BindHistory()
	bridge.Run(app.App(r))
}
