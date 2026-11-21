package core

import (
	goruntime "runtime"
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
	LogRecoverScheduled     LogKind = "recover.scheduled"
	LogRecoverEffect        LogKind = "recover.effect"
	LogRecoverFrame         LogKind = "recover.frame"
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
// console). Host builds always log via log.Printf as well; in the browser the
// sink is the only output (with none, println), so the log package isn't
// linked (#126).
func SetLogSink(fn func(LogEntry)) {
	logSinkMu.Lock()
	logSink = fn
	logSinkMu.Unlock()
}

// Log emits a structured entry to the host logger and any registered sink.
func Log(kind LogKind, message string, fields map[string]any) {
	entry := LogEntry{Kind: kind, Message: message, Fields: fields}
	logSinkMu.Lock()
	sink := logSink
	logSinkMu.Unlock()
	hostLog(kind, message, fields, sink != nil)
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
		b.WriteString(FormatValue(v))
	}
	return b.String()
}

// Recover runs fn and, on panic, logs a structured recover entry and returns the
// panic value. Fields may be nil; "panic" is added automatically.
func Recover(kind LogKind, message string, fields map[string]any, fn func()) (rec any) {
	defer func() {
		if rec = recover(); rec != nil {
			Log(kind, message, PanicFields(rec, fields))
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

// PanicFields is fields plus "panic": rec and, in dev mode, "stack": the
// panicking goroutine's stack, for the dev overlay (#148). Call it from the
// deferred function that recovered.
func PanicFields(rec any, fields map[string]any) map[string]any {
	f := copyFields(fields)
	f["panic"] = rec
	if DevChecks() {
		buf := make([]byte, 16<<10)
		if n := goruntime.Stack(buf, false); n > 0 { // TinyGo has no stacks
			f["stack"] = string(buf[:n])
		}
	}
	return f
}
