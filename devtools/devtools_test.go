package devtools

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

func TestSnapshotWalksTreeAndSignals(t *testing.T) {
	Enable()

	sig := core.NewSignal(3)
	RegisterSignal(sig, "state")

	frame := &core.ComponentFrame{Path: "/0"}
	frame.Hooks = []any{sig}

	root := &core.ComponentNode{
		Name:  "App",
		Frame: frame,
		Prev: &core.ElementNode{
			Tag: "div",
			ID:  1,
			Children: []core.Node{
				&core.ScopeNode{
					Deps: []core.SignalAccessor{sig},
					Prev: &core.TextNode{ID: 2, Value: "hi"},
				},
			},
		},
	}
	SetRoot(root)

	snap := Snapshot()
	tree, ok := snap["tree"].(map[string]any)
	if !ok {
		t.Fatalf("tree type = %T", snap["tree"])
	}
	if tree["type"] != "component" || tree["name"] != "App" {
		t.Fatalf("root tree = %#v", tree)
	}

	signals, ok := snap["signals"].([]map[string]any)
	if !ok {
		t.Fatalf("signals type = %T", snap["signals"])
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0]["id"] != 1 {
		t.Fatalf("signal id = %v", signals[0]["id"])
	}
}

func TestRegisterSignalWhenEnabled(t *testing.T) {
	Enable()
	sig := core.NewSignal("hello")
	RegisterSignal(sig, "test")
	id := SignalID(sig)
	if id == 0 {
		t.Fatal("expected non-zero signal id")
	}
}

func TestSignalIDUnknown(t *testing.T) {
	sig := core.NewSignal(1)
	// Without registration, id is 0 even when enabled (separate registration).
	if SignalID(sig) != 0 {
		t.Fatalf("unregistered signal id = %d", SignalID(sig))
	}
}

func TestPackageOf(t *testing.T) {
	for fn, want := range map[string]string{
		"github.com/yogisalomo/goowee/core.(*Signal[go.shape.int]).Set":            "github.com/yogisalomo/goowee/core",
		"github.com/me/app.(*Store[go.shape.struct { github.com/x/y.Z }]).Add":     "github.com/me/app",
		"github.com/yogisalomo/goowee/examples/counter/app.heroDemo.func1.1":       "github.com/yogisalomo/goowee/examples/counter/app",
		"github.com/yogisalomo/goowee/hooks.UsePersist[go.shape.[]string].func1.1": "github.com/yogisalomo/goowee/hooks",
		"syscall/js.handleEvent": "syscall/js",
		"runtime.goexit":         "runtime",
		"main.main":              "main",
	} {
		if got := packageOf(fn); got != want {
			t.Errorf("packageOf(%q) = %q, want %q", fn, got, want)
		}
	}
	for pkg, want := range map[string]bool{
		"github.com/yogisalomo/goowee/h":                    true,
		"github.com/yogisalomo/goowee/internal/dom":         true,
		"github.com/yogisalomo/goowee/examples/counter/app": false,
		"github.com/yogisalomo/goowee/devtools_test":        false,
		"syscall/js":        true,
		"main":              false,
		"github.com/me/app": false,
	} {
		if got := framework(pkg); got != want {
			t.Errorf("framework(%q) = %v", pkg, got)
		}
	}
}
