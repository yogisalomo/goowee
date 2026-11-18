// Package app is the page test/e2e/early.mjs drives (#131): server-rendered
// controls the user touches before the WASM has booted. A text input and a
// checkbox bound to signals, a button that counts clicks, and a form whose
// submit is counted; #state shows what Go holds.
package app

import (
	"fmt"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func App() core.Node {
	return core.Component("EarlyApp", func() core.Node {
		name, _ := hooks.UseState("initial")
		agreed, _ := hooks.UseState(false)
		clicks, setClicks := hooks.UseState(0)
		submits, setSubmits := hooks.UseState(0)
		state := core.Computed([]core.SignalAccessor{name, agreed, clicks, submits}, func() string {
			return fmt.Sprintf("name=%s agreed=%v clicks=%d submits=%d", name.Get(), agreed.Get(), clicks.Get(), submits.Get())
		})
		return Div(
			Input(ID("name"), BindValue(name)),
			Input(ID("agree"), Type("checkbox"), BindChecked(agreed)),
			Button(ID("go"), OnClick(func() { setClicks(clicks.Get() + 1) }), Text("Go")),
			Form(ID("form"), OnSubmit(func(map[string]string) { setSubmits(submits.Get() + 1) }),
				Button(ID("send"), Type("submit"), Text("Send"))),
			P(ID("state"), TextS(state)),
		)
	})
}
