// Package web serves the browser assets that must match a goowee build:
// goowee.js, embedded from this module, and wasm_exec.js, from the toolchain
// that built the app.
package web

import (
	"github.com/yogisalomo/goowee/internal/version"
	goruntime "github.com/yogisalomo/goowee/runtime"
)

// Version is the goowee version of the embedded runtime.
const Version = version.Module

// RuntimeJS returns a copy of this module's goowee.js.
func RuntimeJS() []byte {
	return goruntime.JS()
}

// WasmExecJS returns the Go toolchain's wasm_exec.js, the loader that matches
// the WASM binary this module built.
func WasmExecJS() ([]byte, error) {
	return wasmExecJS()
}
