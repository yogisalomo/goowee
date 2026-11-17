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
	initBridge(renderer.Scheduler, renderer.Registry, newTransport())
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
func initBridge(sched *core.Scheduler, registry *dom.NodeRegistry, tr *transport) {
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

	startScheduler(sched, tr)
}

func startScheduler(sched *core.Scheduler, tr *transport) {
	scheduled := false
	var rAF js.Func
	rAF = js.FuncOf(func(this js.Value, args []js.Value) any {
		scheduled = false
		muts := sched.Flush()
		if len(muts) > 0 {
			sendMutations(sched, tr, muts)
		}
		// Effects queued by this frame's renders (OnMount, UseEffect) run now
		// that their DOM is in the document.
		sched.RunEffects()
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

// transport carries each frame's batch to the runtime. By default that is the
// binary format (internal/dom/batch.go), copied into a Uint8Array reused from
// frame to frame and read there in place; it sends JSON instead when the page
// asks for it (goowee.transport = "json") or its goowee.js predates the binary
// format.
type transport struct {
	enc   dom.BatchEncoder
	apply js.Value // goowee.applyBatch; undefined when sending JSON
	buf   js.Value // the Uint8Array batches are copied into
	size  int      // buf's length
	small int      // consecutive batches that used under an eighth of a big buf
}

// Like BatchEncoder's own buffers, buf is released after shrinkAfter batches in
// a row that used under an eighth of it once it is past shrinkAbove bytes.
const (
	shrinkAbove = 1 << 20
	shrinkAfter = 64
)

// newTransport picks the transport and records it in goowee.transport.
func newTransport() *transport {
	t := &transport{}
	g := js.Global().Get("goowee")
	apply, version := g.Get("applyBatch"), g.Get("batchVersion")
	switch {
	case g.Get("transport").Equal(js.ValueOf("json")):
	case apply.Type() != js.TypeFunction || version.Type() != js.TypeNumber || version.Int() != dom.BatchVersion:
		core.Log(core.LogWarn, "this page's goowee.js doesn't read the app's DOM update format, so updates are sent as JSON; serve the goowee.js from the app's goowee version", map[string]any{
			"batchVersion": dom.BatchVersion,
		})
	default:
		t.apply = apply
	}
	if t.apply.IsUndefined() {
		g.Set("transport", "json")
	} else {
		g.Set("transport", "binary")
	}
	return t
}

// send applies muts in the runtime and returns what it answered: the replies
// to the batch's reads, as JSON, or undefined.
func (t *transport) send(muts []core.Mutation) js.Value {
	if t.apply.IsUndefined() {
		data, dropped := dom.EncodeBatch(muts)
		dom.LogDropped(dropped)
		return js.Global().Call("applyMutations", string(data))
	}
	data, dropped := t.enc.Encode(muts)
	dom.LogDropped(dropped)
	t.fit(len(data))
	js.CopyBytesToJS(t.buf, data)
	return t.apply.Invoke(t.buf, len(data))
}

// fit makes buf hold an n-byte batch, growing it to the next power of two.
func (t *transport) fit(n int) {
	if n <= t.size {
		if t.size <= shrinkAbove || n >= t.size/8 {
			t.small = 0
			return
		}
		if t.small++; t.small < shrinkAfter {
			return
		}
	}
	size := 4096
	for size < n {
		size *= 2
	}
	t.buf, t.size, t.small = js.Global().Get("Uint8Array").New(size), size, 0
}

// sendMutations applies a batch and delivers the replies to any reads
// (ref.Get) it carried: the runtime returns them as JSON once every write in
// the batch is in the DOM. The callbacks run here, on the render loop.
func sendMutations(sched *core.Scheduler, tr *transport, muts []core.Mutation) {
	ret := tr.send(muts)
	if ret.Type() != js.TypeString {
		return
	}
	var replies []struct {
		Req   int `json:"req"`
		Value any `json:"value"`
	}
	if err := json.Unmarshal([]byte(ret.String()), &replies); err != nil {
		core.Log(core.LogWarn, "could not decode ref read replies", map[string]any{"error": err.Error()})
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

// Element returns the DOM element ref is attached to, for handing to a
// JavaScript library that draws into or manages it — a map, a chart, a code
// editor, a <canvas> context:
//
//	hooks.OnMount(func() func() {
//	    m := js.Global().Get("L").Call("map", bridge.Element(mapRef))
//	    return func() { m.Call("remove") }
//	})
//
// Call it from OnMount (the element is in the document by then) or later; it
// returns js.Undefined() before the element has rendered or after it has been
// removed. Give the library an element whose children goowee doesn't manage
// (render it empty), so the library and the renderer never edit the same
// nodes. Only available in js/wasm builds.
func Element(ref *core.Ref) js.Value {
	if ref == nil || ref.ID == 0 {
		return js.Undefined()
	}
	return js.Global().Get("goowee").Call("node", ref.ID)
}
