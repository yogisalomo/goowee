//go:build js && wasm

// Command hello is the smallest goowee app: test/e2e/wasmmime.mjs boots it
// under different server behaviours (#109).
package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func main() {
	bridge.Run(core.Component("Hello", func() core.Node {
		n, setN := hooks.UseState(0)
		return Div(P(ID("hello"), Textf("clicked %d", n)), Button(ID("inc"), OnClick(func() { setN(n.Get() + 1) }), Text("+")))
	}))
}
