package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

func TestErrorBoundaryLogsStructuredRecover(t *testing.T) {
	var entry core.LogEntry
	core.SetLogSink(func(e core.LogEntry) { entry = e })
	defer core.SetLogSink(nil)

	b := &core.ErrorBoundaryNode{
		Fallback: func(err any) core.Node {
			return &core.TextNode{Value: "fallback"}
		},
		Child: core.Component("Boom", func() core.Node { panic("boom") }),
	}
	if _, id := New().Render(b); id == 0 {
		t.Fatal("expected fallback root id")
	}
	if entry.Kind != core.LogRecoverErrorBoundary {
		t.Fatalf("kind = %q, want %q", entry.Kind, core.LogRecoverErrorBoundary)
	}
	if entry.Fields["phase"] != "render" {
		t.Fatalf("phase = %v", entry.Fields["phase"])
	}
}

func TestEventHandlerLogsStructuredRecover(t *testing.T) {
	var entry core.LogEntry
	core.SetLogSink(func(e core.LogEntry) { entry = e })
	defer core.SetLogSink(nil)

	r := New()
	n := &core.ElementNode{
		Tag: "button",
		Handlers: []core.Handler{{
			Event: "click",
			Fn:    func(core.EventData) { panic("handler boom") },
		}},
	}
	r.Render(n)
	if _, handled := r.Registry.Dispatch(1, "click", `{}`); !handled {
		t.Fatal("expected handler to run despite panic")
	}
	if entry.Kind != core.LogRecoverEventHandler {
		t.Fatalf("kind = %q, want %q", entry.Kind, core.LogRecoverEventHandler)
	}
	if entry.Fields["event"] != "click" {
		t.Fatalf("event = %v", entry.Fields["event"])
	}
}
