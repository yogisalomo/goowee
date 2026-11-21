//go:build js && wasm

package bridge

import (
	"encoding/json"
	"errors"
	"syscall/js"
	"unsafe"

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
	initObservability()

	renderer := dom.New()
	// Route core.Schedule onto this scheduler before the first render, so a
	// mount effect can defer work (a ref read, say) to the first flush.
	core.SetActiveScheduler(renderer.Scheduler)
	// Hydrate when the document was server-rendered.
	if serverRendered() {
		renderer.SetHydrating(true)
	}
	muts, _ := renderer.Render(app)
	renderer.Scheduler.Enqueue(muts...)
	setInspectorRoot(app)
	initBridge(renderer.Scheduler, renderer.Registry, newTransport())
	select {}
}

// serverRendered reports whether the page holds server markup for this URL
// to hydrate (goowee.serverRendered: node ids, and a prerendered page was
// rendered for this path). An older goowee.js has no such function: then any
// node id means server-rendered, as before.
func serverRendered() bool {
	if fn := js.Global().Get("goowee").Get("serverRendered"); fn.Type() == js.TypeFunction {
		return fn.Invoke().Bool()
	}
	return js.Global().Get("document").Call("querySelector", "[data-node-id]").Truthy()
}

func initObservability() {
	initLogSink()
	initDevTools()
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

// initBridge wires the render loop to the JS runtime: it registers the event
// dispatcher, announces the event types the app listens for, and drives one
// requestAnimationFrame flush whenever work appears.
func initBridge(sched *core.Scheduler, registry *dom.NodeRegistry, tr *transport) {
	core.SetFileReader(readFile)

	// The result is packed into one number (dom.DispatchBits): a map costs a
	// quarter more per call (#132). A goowee.js from before 0.3.0 reads an
	// object instead and doesn't announce dispatchBits; answer it in its own
	// format, so a stale runtime still runs the app's handlers.
	packed := js.Global().Get("goowee").Get("dispatchBits").Truthy()
	if !packed {
		core.Log(core.LogWarn, "this page's goowee.js is older than the app; events still work, but serve the goowee.js of the same goowee version", nil)
	}
	js.Global().Set("handleEvent", js.FuncOf(func(this js.Value, args []js.Value) any {
		opts, handled := registry.Dispatch(args[0].Int(), args[1].String(), args[2].String())
		if packed {
			return dom.DispatchBits(opts, handled)
		}
		return map[string]any{
			"handled":         handled,
			"preventDefault":  opts.PreventDefault,
			"stopPropagation": opts.StopPropagation,
			"selectOnFocus":   opts.SelectOnFocus,
		}
	}))
	// This Go side sends MutListen for every node with a handler, so the
	// runtime asks Go only about those nodes.
	if g := js.Global().Get("goowee"); g.Type() == js.TypeObject {
		g.Set("listenSets", true)
	}

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
	global := js.Global()
	doc := global.Get("document")
	scheduled := false // a frame is requested and hasn't run yet
	inFrame := false   // the frame callback is running
	wanted := false    // work appeared during the frame; request after it
	var frame js.Func
	// request schedules exactly one frame when work appears, instead of
	// waking the module every ~16ms while idle. A hidden page (a background
	// tab) gets no animation frames, so work arriving then (a socket message,
	// a fetch result) would wait until the page is shown: run the frame on a
	// timer instead, which the browser still fires, if throttled. Animation
	// (frame callbacks, core.NextFrame) is the exception: a hidden tab
	// doesn't animate, so when frame callbacks are all that's pending the
	// frame waits for the page to be shown.
	request := func() {
		if scheduled {
			return
		}
		hidden := doc.Get("hidden").Bool()
		if hidden && !sched.HasWork() {
			return
		}
		scheduled = true
		if hidden {
			global.Call("setTimeout", frame, 0)
		} else {
			global.Call("requestAnimationFrame", frame)
		}
	}
	frame = js.FuncOf(func(this js.Value, args []js.Value) any {
		if !scheduled {
			return nil // this frame already ran, from the other callback
		}
		scheduled = false
		inFrame = true
		muts := sched.Flush()
		if len(muts) > 0 {
			sendMutations(sched, tr, muts)
		}
		// Effects queued by this frame's renders (OnMount, UseEffect) run now
		// that their DOM is in the document.
		sched.RunEffects()
		inFrame = false
		if wanted || sched.HasWork() || sched.HasFrames() {
			wanted = false
			request()
		}
		return nil
	})
	// Work that appears while a frame runs is requested once the frame is
	// done, when HasWork tells real work from frame callbacks alone.
	sched.OnWork = func() {
		if inFrame {
			wanted = true
			return
		}
		request()
	}
	doc.Call("addEventListener", "visibilitychange", js.FuncOf(func(this js.Value, args []js.Value) any {
		if doc.Get("hidden").Bool() {
			// A frame requested just before the page was hidden would wait
			// for it to be shown again; move it to a timer too.
			if scheduled {
				global.Call("setTimeout", frame, 0)
			}
		} else if sched.HasFrames() {
			request() // resume the animation that paused while hidden
		}
		return nil
	}))
	// Flush whatever the initial render already enqueued.
	request()
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

// sendMutations sends a frame to the runtime: the DOM batch first, then each
// canvas draw (ref.Draw, canvas.View) as raw bytes, so a draw sees the frame's
// DOM — a <canvas> created in the same frame exists by then.
func sendMutations(sched *core.Scheduler, tr *transport, muts []core.Mutation) {
	muts, draws := dom.SplitDraws(muts)
	if len(muts) > 0 {
		applyBatch(sched, tr, muts)
	}
	for _, d := range draws {
		sendDraw(d)
	}
}

// applyBatch applies a batch and delivers the replies to any reads (ref.Get)
// it carried: the runtime returns them as JSON once every write in the batch
// is in the DOM. The callbacks run here, on the render loop.
func applyBatch(sched *core.Scheduler, tr *transport, muts []core.Mutation) {
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

// drawBuf is the Uint8Array draws are copied into, reused from frame to frame
// and grown by powers of two.
var (
	drawBuf     js.Value
	drawBufLen  int
	drawChecked bool
	drawOK      bool
	drawFn      js.Value
)

// sendDraw copies a goowee/canvas buffer into JS memory in one copy and hands
// it to the runtime's replayer (goowee.draw), which issues the canvas calls.
func sendDraw(m core.Mutation) {
	cmds, _ := m.Value.([]uint32)
	if len(cmds) == 0 {
		return
	}
	if !drawChecked {
		drawChecked = true
		g := js.Global().Get("goowee")
		drawFn = g.Get("draw")
		v := g.Get("canvasVersion")
		drawOK = drawFn.Type() == js.TypeFunction && v.Type() == js.TypeNumber && v.Int() == core.DrawVersion
		if !drawOK {
			core.Log(core.LogWarn, "this page's goowee.js can't draw goowee/canvas buffers; serve the goowee.js of the same version", map[string]any{"want": core.DrawVersion})
		}
	}
	if !drawOK {
		return
	}
	n := len(cmds) * 4
	if n > drawBufLen {
		size := 1 << 16
		for size < n {
			size <<= 1
		}
		drawBuf = js.Global().Get("Uint8Array").New(size)
		drawBufLen = size
	}
	// wasm is little-endian, so the words' memory is the buffer's byte form.
	js.CopyBytesToJS(drawBuf, unsafe.Slice((*byte)(unsafe.Pointer(&cmds[0])), n))
	drawFn.Invoke(drawBuf, n, m.NodeID)
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

// FileValue returns the browser's File object behind f (picked on a file
// input, pasted or dropped), for handing to a JavaScript library or browser
// API without copying its contents through WebAssembly memory:
//
//	OnChangeE(func(e core.EventData) {
//	    for _, f := range e.Files() {
//	        js.Global().Get("URL").Call("createObjectURL", bridge.FileValue(f)) // a preview
//	    }
//	})
//
// To upload a file, use goowee/net's Form, which does this for you. It
// returns js.Undefined() once the file is no longer available (the input's
// selection changed, the next paste or drop replaced it, or its element was
// removed), or for a File that didn't come from the browser. Only available in
// js/wasm builds.
func FileValue(f core.File) js.Value {
	if f.Handle() == 0 {
		return js.Undefined()
	}
	return js.Global().Get("goowee").Call("fileValue", f.Handle())
}
