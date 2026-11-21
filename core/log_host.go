//go:build !(js && wasm)

package core

import "log"

// hostLog writes every entry to the standard logger.
func hostLog(kind LogKind, message string, fields map[string]any, _ bool) {
	log.Printf("goowee[%s] %s", kind, formatFields(message, fields))
}
