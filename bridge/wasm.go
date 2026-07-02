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
		nodeID := args[0].Int()
		eventType := args[1].String()
		data := args[2].String()
		registry.Dispatch(nodeID, eventType, data)
		return nil
	}))
	startScheduler(sched)
}

func startScheduler(sched *core.Scheduler) {
	var rAF js.Func
	rAF = js.FuncOf(func(this js.Value, args []js.Value) any {
		muts := sched.Flush()
		if len(muts) > 0 {
			sendMutations(muts)
		}
		js.Global().Call("requestAnimationFrame", rAF)
		return nil
	})
	js.Global().Call("requestAnimationFrame", rAF)
}

func sendMutations(muts []core.Mutation) {
	data, err := json.Marshal(muts)
	if err != nil {
		return
	}
	js.Global().Call("applyMutations", string(data))
}
