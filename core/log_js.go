//go:build js && wasm

package core

// hostLog: in the browser the bridge's sink prints entries to the console.
// Before it is registered (or in a program without the bridge), println
// does, which needs neither log nor fmt (#126).
func hostLog(kind LogKind, message string, fields map[string]any, hasSink bool) {
	if !hasSink {
		println("goowee[" + string(kind) + "] " + formatFields(message, fields))
	}
}
