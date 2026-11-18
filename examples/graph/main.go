//go:build js && wasm

// Command graph is the goowee/canvas demo on a page of its own: 10,000 nodes
// and 10,000 edges redrawn every frame. Build and serve it with
//
//	GOOS=js GOARCH=wasm go build -o examples/graph/main.wasm ./examples/graph
//
// next to wasm_exec.js, goowee.js and examples/graph/index.html.
// test/e2e/canvasbench.mjs builds, serves and measures it.
package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/examples/graph/graph"
	. "github.com/yogisalomo/goowee/h"
)

func main() {
	bridge.Run(core.Component("GraphPage", func() core.Node {
		return Div(Style("height:100vh;padding:12px;box-sizing:border-box"), graph.View())
	}))
}
