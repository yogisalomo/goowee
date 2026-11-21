//go:build js && wasm && !tinygo

package bridge

import (
	"syscall/js"
	"unsafe"

	"github.com/yogisalomo/goowee/internal/dom"
)

// The hot crossings between the runtime and Go are plain WebAssembly exports
// rather than syscall/js callbacks (#133, ADR-031). A js.FuncOf call costs
// about 18 µs, because it goes through the Go scheduler; an export call costs
// about 20 ns. Exports only, no imports: a module that imported functions from
// goowee.js would fail to instantiate under an older goowee.js or a page that
// boots it some other way, while unused exports cost nothing. The runtime
// opts in (goowee.useExports) when it has the instance; otherwise everything
// goes through syscall/js as before.
//
// An export runs on the caller's stack and returns without running the Go
// scheduler, so a goroutine a handler starts or wakes would wait for the next
// timer or callback. The runtime resumes the scheduler (go._resume) in a
// microtask after each call, once per burst, which costs about 1 µs.

// exportsVersion is the export protocol: goowee_frame, goowee_scratch and
// goowee_event as below, and goowee.applyBatchAt(ptr, len).
const exportsVersion = 1

var (
	exportRegistry *dom.NodeRegistry
	// scratch is where the runtime writes an event's type and payload, as
	// UTF-8, before calling goowee_event. goowee_scratch grows it.
	scratch []byte
)

// useExports asks the runtime to call the exports and read batches in place,
// and reports whether it will.
func useExports(registry *dom.NodeRegistry, tr *transport) bool {
	g := js.Global().Get("goowee")
	use := g.Get("useExports")
	if use.Type() != js.TypeFunction {
		return false // an older goowee.js
	}
	exportRegistry = registry
	gooweeScratch(4096)
	if !use.Invoke(exportsVersion).Truthy() {
		return false // a page that booted the module itself, or another version
	}
	if !tr.apply.IsUndefined() {
		tr.applyAt = g.Get("applyBatchAt")
	}
	return true
}

// gooweeFrame runs one frame of the render loop: the runtime calls it from
// requestAnimationFrame (or a timer when the page is hidden) after
// goowee.requestFrame.
//
//go:wasmexport goowee_frame
func gooweeFrame() {
	if loop != nil {
		loop.run()
	}
}

// gooweeScratch makes the scratch buffer hold at least n bytes and returns its
// address. Growing it may grow the module's memory, so the runtime takes its
// views of the buffer after this returns.
//
//go:wasmexport goowee_scratch
func gooweeScratch(n uint32) uint32 {
	if int(n) > len(scratch) {
		size := 4096
		for size < int(n) {
			size *= 2
		}
		scratch = make([]byte, size)
	}
	return addr(scratch)
}

// gooweeEvent dispatches an event to node id's handlers: the scratch buffer
// holds the event type (typeLen bytes) followed by the JSON payload. It
// returns dom.DispatchBits. The strings are copied out before any handler
// runs, so an event dispatched while a handler runs (it focuses an element,
// say) may reuse the buffer.
//
//go:wasmexport goowee_event
func gooweeEvent(id int32, typeLen, payloadLen uint32) uint32 {
	if exportRegistry == nil || int(typeLen+payloadLen) > len(scratch) {
		return 0
	}
	typ := string(scratch[:typeLen])
	payload := string(scratch[typeLen : typeLen+payloadLen])
	opts, handled := exportRegistry.Dispatch(int(id), typ, payload)
	return uint32(dom.DispatchBits(opts, handled))
}

// addr is the address of b's first byte in the module's memory.
func addr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}
