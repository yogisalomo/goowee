package core

import (
	"strings"
	"testing"
)

func TestLogStructuredFormat(t *testing.T) {
	var captured LogEntry
	SetLogSink(func(e LogEntry) { captured = e })
	defer SetLogSink(nil)

	Log(LogRecoverEventHandler, "panic in event handler", map[string]any{
		"event": "click",
		"panic": "boom",
	})

	if captured.Kind != LogRecoverEventHandler {
		t.Fatalf("kind = %q, want %q", captured.Kind, LogRecoverEventHandler)
	}
	if captured.Message != "panic in event handler" {
		t.Fatalf("message = %q", captured.Message)
	}
	if captured.Fields["event"] != "click" {
		t.Fatalf("fields[event] = %v", captured.Fields["event"])
	}
}

func TestRecoverLogsAndReturnsPanic(t *testing.T) {
	var captured LogEntry
	SetLogSink(func(e LogEntry) { captured = e })
	defer SetLogSink(nil)

	rec := Recover(LogRecoverReRender, "scope re-render failed", map[string]any{
		"scope": 3,
	}, func() {
		panic("nope")
	})
	if rec == nil {
		t.Fatal("expected panic value")
	}
	if captured.Kind != LogRecoverReRender {
		t.Fatalf("kind = %q", captured.Kind)
	}
	if captured.Fields["panic"] != "nope" {
		t.Fatalf("panic field = %v", captured.Fields["panic"])
	}
}

func TestRecoverNoPanic(t *testing.T) {
	var called bool
	SetLogSink(func(LogEntry) { called = true })
	defer SetLogSink(nil)

	rec := Recover(LogRecoverRender, "render failed", nil, func() {})
	if rec != nil {
		t.Fatalf("unexpected panic %v", rec)
	}
	if called {
		t.Fatal("sink should not run without panic")
	}
}

func TestFormatFieldsMessageOnly(t *testing.T) {
	s := formatFields("hello", nil)
	if s != "hello" {
		t.Fatalf("got %q", s)
	}
}

func TestFormatFieldsWithPairs(t *testing.T) {
	s := formatFields("msg", map[string]any{"a": 1})
	if !strings.Contains(s, "msg") || !strings.Contains(s, "a=1") {
		t.Fatalf("got %q", s)
	}
}
