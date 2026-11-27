//go:build js && wasm

// Package host is how the Go side reaches the browser, other than applying
// batches and receiving events: through the runtime's goowee.host object
// (#158). Code in this module doesn't read document, location, history or
// the storage objects itself (guard_test.go), so a worker can provide the
// same methods over messages.
package host

import "syscall/js"

// Get is the runtime's goowee.host.
func Get() js.Value { return js.Global().Get("goowee").Get("host") }

// Available reports whether the page's goowee.js has a host: one older than
// this module doesn't.
func Available() bool { return Get().Type() == js.TypeObject }

// Worker reports whether the app runs in a Web Worker.
func Worker() bool { return Get().Get("worker").Truthy() }

// Call calls a host method.
func Call(method string, args ...any) js.Value { return Get().Call(method, args...) }
