//go:build js && wasm && !goowee_nodev

package bridge

import (
	"encoding/json"
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
)

// initDevTools turns dev mode on when the page asks for it (?goowee-dev):
// the inspector, the change log and dependency checks. A goowee_nodev build
// leaves all of it out (devtools_nodev.go).
func initDevTools() {
	if js.Global().Get("goowee").Call("devEnabled").Bool() {
		devtools.Enable()
		initInspector()
	} else {
		core.TakeEarlyNames() // no inspector to give Named signals to: let them go
	}
}

// setInspectorRoot gives the inspector the app's tree, in dev mode.
func setInspectorRoot(app core.Node) {
	if devtools.Enabled() {
		devtools.SetRoot(app)
	}
}

func initInspector() {
	js.Global().Get("goowee").Set("inspectGo", js.FuncOf(func(this js.Value, args []js.Value) any {
		data, err := json.Marshal(devtools.Snapshot())
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		var snap map[string]any
		_ = json.Unmarshal(data, &snap)
		return snap
	}))
	js.Global().Get("goowee").Set("changesGo", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && args[0].Truthy() {
			devtools.ClearChanges()
			return []any{}
		}
		data, err := json.Marshal(devtools.Changes())
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		var rows []any
		_ = json.Unmarshal(data, &rows)
		return rows
	}))
}
