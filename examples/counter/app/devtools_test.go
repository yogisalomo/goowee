package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/router"
)

func TestLandingDevtoolsSnapshot(t *testing.T) {
	devtools.Enable()
	r := testRouter()
	root := landingPage(r)
	mount(t, root)
	devtools.SetRoot(root)

	snap := devtools.Snapshot()
	tree, ok := snap["tree"].(map[string]any)
	if !ok {
		t.Fatalf("tree type = %T", snap["tree"])
	}
	if tree["type"] != "component" {
		t.Fatalf("root type = %v", tree["type"])
	}
	signals, ok := snap["signals"].([]map[string]any)
	if !ok || len(signals) == 0 {
		t.Fatalf("expected registered signals, got %#v", snap["signals"])
	}
}

func TestLandingHeroIncrementHeadless(t *testing.T) {
	h := mount(t, landingPage(testRouter()))
	incr := h.dom.buttonWithText("increment")
	if incr == 0 {
		t.Fatalf("increment button not found; tree=%q", h.dom.text(0))
	}
	h.click(incr)
	count := h.dom.find(func(n *fnode) bool { return n.attrs["class"] == "count" })
	if count == 0 || h.dom.text(count) != "1" {
		t.Fatalf("after increment want count 1, got node %d text %q", count, h.dom.text(count))
	}
}

func TestErrorPageBoundaryAndStructuredLog(t *testing.T) {
	var entry core.LogEntry
	core.SetLogSink(func(e core.LogEntry) { entry = e })
	defer core.SetLogSink(nil)

	h := mount(t, errorPage(testRouter()))
	if !strings.Contains(h.dom.text(0), "Recovered:") {
		t.Fatalf("expected fallback text, tree=%q", h.dom.text(0))
	}
	if !strings.Contains(h.dom.text(0), "This line still renders") {
		t.Fatal("expected sibling content outside the boundary")
	}
	if entry.Kind != core.LogRecoverErrorBoundary {
		t.Fatalf("kind = %q, want %q", entry.Kind, core.LogRecoverErrorBoundary)
	}
}

// #74: the example app itself must declare every dependency — render every
// page and walk the router through them with dev checks on, and expect no
// undeclared-read warnings.
func TestExampleAppDeclaresItsDependencies(t *testing.T) {
	var warns []string
	core.SetDevChecks(true)
	core.SetLogSink(func(e core.LogEntry) {
		if strings.Contains(e.Message, "declared dependency") {
			warns = append(warns, fmt.Sprint(e.Fields))
		}
	})
	defer func() {
		core.SetDevChecks(false)
		core.SetLogSink(nil)
	}()
	r := router.New("/")
	h := mount(t, App(r))
	for _, path := range []string{"/tutorial", "/counter", "/form", "/todos", "/dashboard", "/stopwatch",
		"/async", "/greet/alice", "/greet/bob", "/events", "/getting-started", "/about", "/nope", "/"} {
		r.Navigate(path)
		h.flush()
	}
	if len(warns) != 0 {
		t.Fatalf("undeclared dependencies in the example app:\n%s", strings.Join(warns, "\n"))
	}
}
