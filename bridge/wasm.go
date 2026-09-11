//go:build js && wasm

package bridge

import (
	"encoding/json"
	"errors"
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/internal/dom"
)

// Run mounts app into the page and drives the render loop. It is the single
// entry point for a goowee client: create your root node (typically an
// App(router) component) and hand it to Run from your program's main.
//
// If the page was server-rendered — its nodes carry data-node-id — Run hydrates
// by claiming that DOM instead of rebuilding it. Run does not return; it blocks
// forever so the WASM module stays alive to service events.
func Run(app core.Node) {
	initObservability()

	renderer := dom.New()
	// Route core.Schedule onto this scheduler before the first render, so a
	// mount effect can defer work (a ref read, say) to the first flush.
	core.SetActiveScheduler(renderer.Scheduler)
	// Hydrate when the document was server-rendered.
	if js.Global().Get("document").Call("querySelector", "[data-node-id]").Truthy() {
		renderer.SetHydrating(true)
	}
	muts, _ := renderer.Render(app)
	renderer.Scheduler.Enqueue(muts...)
	if devtools.Enabled() {
		devtools.SetRoot(app)
	}
	initBridge(renderer.Scheduler, renderer.Registry)
	select {}
}

func initObservability() {
	initLogSink()
	if js.Global().Get("goowee").Call("devEnabled").Bool() {
		devtools.Enable()
		initInspector()
	}
}

func initLogSink() {
	core.SetLogSink(func(e core.LogEntry) {
		payload := map[string]any{
			"kind":    string(e.Kind),
			"message": e.Message,
		}
		for k, v := range e.Fields {
			payload[k] = v
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		js.Global().Get("goowee").Call("log", string(data))
	})
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
}

// initBridge wires the render loop to the JS runtime: it registers the event
// dispatcher, announces the event types the app listens for, and drives one
// requestAnimationFrame flush whenever work appears.
func initBridge(sched *core.Scheduler, registry *dom.NodeRegistry) {
	core.SetFileReader(readFile)

	js.Global().Set("handleEvent", js.FuncOf(func(this js.Value, args []js.Value) any {
		opts, handled := registry.Dispatch(args[0].Int(), args[1].String(), args[2].String())
		return map[string]any{
			"handled":         handled,
			"preventDefault":  opts.PreventDefault,
			"stopPropagation": opts.StopPropagation,
			"selectOnFocus":   opts.SelectOnFocus,
		}
	}))

	announce := func(eventType string, capture bool) {
		js.Global().Call("goListen", eventType, capture)
	}
	registry.OnNewEventType = announce
	for _, t := range registry.EventTypes() {
		announce(t, dom.EventCapture(t))
	}

	startScheduler(sched)
}

func startScheduler(sched *core.Scheduler) {
	scheduled := false
	var rAF js.Func
	rAF = js.FuncOf(func(this js.Value, args []js.Value) any {
		scheduled = false
		muts := sched.Flush()
		if len(muts) > 0 {
			sendMutations(sched, muts)
		}
		return nil
	})
	// Schedule exactly one frame when work appears, instead of waking the
	// module every ~16ms while idle.
	sched.OnWork = func() {
		if scheduled {
			return
		}
		scheduled = true
		js.Global().Call("requestAnimationFrame", rAF)
	}
	// Flush whatever the initial render already enqueued.
	sched.OnWork()
}

// sendMutations applies a batch and delivers the replies to any reads
// (ref.Get) it carried: applyMutations returns them as JSON once every write in
// the batch is in the DOM. The callbacks run here, on the render loop.
func sendMutations(sched *core.Scheduler, muts []core.Mutation) {
	data, err := json.Marshal(muts)
	if err != nil {
		return
	}
	ret := js.Global().Call("applyMutations", string(data))
	if ret.Type() != js.TypeString {
		return
	}
	var replies []struct {
		Req   int `json:"req"`
		Value any `json:"value"`
	}
	if err := json.Unmarshal([]byte(ret.String()), &replies); err != nil {
		return
	}
	for _, r := range replies {
		sched.Resolve(r.Req, r.Value)
	}
}

// readFile backs core.File.Bytes: it asks the runtime for the parked File's
// bytes and blocks until the promise settles. It must run off the event loop
// (a goroutine), which is what File.Bytes documents.
func readFile(handle int) ([]byte, error) {
	promise := js.Global().Get("goowee").Call("readFile", handle)
	done := make(chan struct{})
	var (
		out     []byte
		readErr error
	)
	onOK := js.FuncOf(func(this js.Value, args []js.Value) any {
		u8 := args[0]
		out = make([]byte, u8.Get("length").Int())
		js.CopyBytesToGo(out, u8)
		close(done)
		return nil
	})
	onErr := js.FuncOf(func(this js.Value, args []js.Value) any {
		msg := "file read failed"
		if len(args) > 0 && args[0].Type() == js.TypeObject {
			if m := args[0].Get("message"); m.Type() == js.TypeString {
				msg = m.String()
			}
		}
		readErr = errors.New("goowee: " + msg)
		close(done)
		return nil
	})
	defer onOK.Release()
	defer onErr.Release()
	promise.Call("then", onOK, onErr)
	<-done
	return out, readErr
}
