package core

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// LogKind categorizes structured log entries from recover boundaries and the
// reactive core. Recover kinds use the "recover." prefix so sinks can route
// them to console.error consistently.
type LogKind string

const (
	LogRecoverEventHandler  LogKind = "recover.event_handler"
	LogRecoverErrorBoundary LogKind = "recover.error_boundary"
	LogRecoverReRender      LogKind = "recover.re_render"
	LogRecoverRender        LogKind = "recover.render"
	LogRecoverRead          LogKind = "recover.read"
	LogSignalCycle          LogKind = "signal.cycle"
	LogWarn                 LogKind = "warn"
)

// LogEntry is a single structured observability event.
type LogEntry struct {
	Kind    LogKind
	Message string
	Fields  map[string]any
}

var (
	logSinkMu sync.Mutex
	logSink   func(LogEntry)
)

// SetLogSink registers an optional sink (e.g. the WASM bridge forwarding to
// console). The default host sink always logs via log.Printf as well.
func SetLogSink(fn func(LogEntry)) {
	logSinkMu.Lock()
	logSink = fn
	logSinkMu.Unlock()
}

// Log emits a structured entry to the host logger and any registered sink.
func Log(kind LogKind, message string, fields map[string]any) {
	entry := LogEntry{Kind: kind, Message: message, Fields: fields}
	log.Printf("goowee[%s] %s", kind, formatFields(message, fields))
	logSinkMu.Lock()
	sink := logSink
	logSinkMu.Unlock()
	if sink != nil {
		sink(entry)
	}
}

func formatFields(message string, fields map[string]any) string {
	if len(fields) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	for k, v := range fields {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		fmt.Fprintf(&b, "%v", v)
	}
	return b.String()
}

// Recover runs fn and, on panic, logs a structured recover entry and returns the
// panic value. Fields may be nil; "panic" is added automatically.
func Recover(kind LogKind, message string, fields map[string]any, fn func()) (rec any) {
	defer func() {
		if rec = recover(); rec != nil {
			f := copyFields(fields)
			f["panic"] = rec
			Log(kind, message, f)
		}
	}()
	fn()
	return nil
}

func copyFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	return out
}
