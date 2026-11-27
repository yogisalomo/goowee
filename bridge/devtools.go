//go:build js && wasm && !goowee_nodev

package bridge

import (
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/devtools/overlay"
	"github.com/yogisalomo/goowee/internal/host"
	"github.com/yogisalomo/goowee/internal/jsonlite"
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
		// Through JSON text, which turns the snapshot's typed slices and
		// numbers into what js.ValueOf accepts.
		data, ok := jsonlite.Append(nil, devtools.Snapshot())
		if !ok {
			return map[string]any{"error": "the inspector snapshot can't be encoded"}
		}
		snap, _ := jsonlite.Decode(string(data))
		return snap
	}))
	js.Global().Get("goowee").Set("changesGo", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && args[0].Truthy() {
			devtools.ClearChanges()
			return []any{}
		}
		changes := devtools.Changes()
		rows := make([]any, 0, len(changes))
		for _, c := range changes {
			row := map[string]any{"seq": c.Seq, "ms": c.MS, "signal": c.Signal, "from": c.From, "to": c.To, "at": c.At, "func": c.Func}
			if c.Name != "" {
				row["name"] = c.Name
			}
			rows = append(rows, row)
		}
		return rows
	}))
}

// recordDevLog keeps a log entry for the dev overlay (#148), in dev mode.
func recordDevLog(e core.LogEntry) {
	if devtools.Enabled() {
		devtools.RecordLog(e)
	}
}

var overlayMounted bool

// afterMount mounts the dev overlay as a root of its own once the app's
// first root is mounted, in dev mode.
func afterMount() {
	if overlayMounted || !devtools.Enabled() || host.Worker() {
		return
	}
	overlayMounted = true
	body := host.Call("body")
	el := body.Get("ownerDocument").Call("createElement", "div")
	el.Set("id", "goowee-devtools")
	body.Call("appendChild", el)
	mountInto(el, overlay.View(), false)
}
