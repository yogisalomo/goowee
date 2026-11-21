//go:build js && wasm && tinygo

package bridge

import "github.com/yogisalomo/goowee/internal/dom"

// TinyGo builds keep every crossing on syscall/js for now: its scheduler and
// wasm_exec.js differ in how an export resumes goroutines (ADR-031).
func useExports(*dom.NodeRegistry, *transport) bool { return false }

func addr([]byte) uint32 { return 0 }
