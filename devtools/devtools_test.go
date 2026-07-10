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
