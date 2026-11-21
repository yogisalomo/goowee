//go:build js && wasm

// Command islands is the client of test/e2e/islands.mjs (#154): it hydrates
// every counter island in the page and exposes window.unmountIsland(i).
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/test/e2e/islands/app"
)

func main() {
	roots := bridge.Islands(map[string]func(json.RawMessage) core.Node{"counter": app.Factory})
	js.Global().Set("unmountIsland", js.FuncOf(func(this js.Value, args []js.Value) any {
		roots[args[0].Int()].Unmount()
		return nil
	}))
	bridge.Wait()
}
