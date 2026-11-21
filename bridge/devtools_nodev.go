//go:build js && wasm && goowee_nodev

package bridge

import (
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
)

// initDevTools in a goowee_nodev build (#127): dev tools are compiled out, so
// ?goowee-dev turns nothing on, and goowee.inspect() and goowee.changes() say
// the build has none.
func initDevTools() {
	js.Global().Get("goowee").Set("noDevTools", true)
	core.TakeEarlyNames()
}

func setInspectorRoot(core.Node) {}

func recordDevLog(core.LogEntry) {}

func afterMount() {}
