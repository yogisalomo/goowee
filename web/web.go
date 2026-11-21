// Package web exposes the browser assets that must match a goowee build.
// Handler serves goowee.js, embedded from this module; WasmExecJS returns the
// app toolchain's loader for the `goowee assets` command to copy into the
// app's static files. A server does not need a Go toolchain at request time.
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
// the WASM binary this module built. It reads the toolchain at asset-copy time
// (`goowee assets`); ssr.Handler does not call it.
func WasmExecJS() ([]byte, error) {
	return wasmExecJS()
}
