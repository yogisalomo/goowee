//go:build js && wasm

// Command early is the client of test/e2e/early.mjs (#131); render/ writes
// the same page server-rendered.
package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/test/e2e/early/app"
)

func main() { bridge.Run(app.App()) }
