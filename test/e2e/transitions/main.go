//go:build js && wasm

// Command transitions is the app test/e2e/transitions.mjs drives (#156): a
// single Show-toggled panel with a Transition("fade") item. The test checks
// that a leaving node stays in the DOM until its animation ends, is inert,
// and is then forgotten by the runtime.
package main

import (
	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func main() {
	bridge.Run(core.Component("TransitionsE2E", func() core.Node {
		show, setShow := hooks.UseState(true)
		return Div(
			Button(ID("toggle"), OnClick(func() { setShow(!show.Get()) }), Text("toggle")),
			Show(show, func() core.Node {
				return Div(ID("panel"), Class("panel"), Transition("fade"),
					OnClick(func() {}), Text("panel"))
			}),
		)
	}))
}
