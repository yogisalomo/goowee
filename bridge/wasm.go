//go:build js && wasm

package bridge

import (
	"encoding/json"
	"goowee/core"
	"goowee/dom"
	"syscall/js"
)

func Init(sched *core.Scheduler, registry *dom.NodeRegistry) {
	js.Global().Set("handleEvent", js.FuncOf(func(this js.Value, args []js.Value) any {
		opts, handled := registry.Dispatch(args[0].Int(), args[1].String(), args[2].String())
		return map[string]any{
			"handled":         handled,
			"preventDefault":  opts.PreventDefault,
			"stopPropagation": opts.StopPropagation,
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
