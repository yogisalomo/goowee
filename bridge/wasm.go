//go:build js && wasm

package bridge

import (
	"encoding/json"
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
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
	renderer := dom.New()
	// Hydrate when the document was server-rendered.
	if js.Global().Get("document").Call("querySelector", "[data-node-id]").Truthy() {
		renderer.SetHydrating(true)
	}
	muts, _ := renderer.Render(app)
	renderer.Scheduler.Enqueue(muts...)
	initBridge(renderer.Scheduler, renderer.Registry)
	select {}
}

// initBridge wires the render loop to the JS runtime: it registers the event
// dispatcher, announces the event types the app listens for, and drives one
// requestAnimationFrame flush whenever work appears.
func initBridge(sched *core.Scheduler, registry *dom.NodeRegistry) {
	// Route core.Schedule (off-loop goroutine updates) onto this scheduler.
	core.SetActiveScheduler(sched)

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
			sendMutations(muts)
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

func sendMutations(muts []core.Mutation) {
	data, err := json.Marshal(muts)
	if err != nil {
		return
	}
	js.Global().Call("applyMutations", string(data))
}
