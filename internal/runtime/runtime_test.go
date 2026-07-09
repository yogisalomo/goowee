package runtime

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

func TestCollectIDs(t *testing.T) {
	el := &core.ElementNode{ID: 5, Tag: "div", Children: []core.Node{
		&core.ElementNode{ID: 3, Tag: "span"},
		&core.TextNode{ID: 7, Value: "hello"},
	}}
	ids := CollectIDs(el)
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %v", ids)
	}
	if ids[0] != 5 || ids[1] != 3 || ids[2] != 7 {
		t.Fatalf("expected [5 3 7], got %v", ids)
	}
}

func TestCollectIDsFiltersZero(t *testing.T) {
	el := &core.ElementNode{ID: 0, Tag: "div", Children: []core.Node{
		&core.ElementNode{ID: 2, Tag: "span"},
	}}
	ids := CollectIDs(el)
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("expected [2], got %v", ids)
	}
}

func TestComponentFrame(t *testing.T) {
	frame := PushComponent()
	if frame.Path != "/" {
		t.Fatalf("expected root path '/', got %q", frame.Path)
	}
	if CurrentComponent() != frame {
		t.Fatal("expected current component to be root")
	}
	PopComponent()
	if CurrentComponent() != nil {
		t.Fatal("expected nil after pop")
	}
}

func TestComponentTreeNesting(t *testing.T) {
	root := PushComponent()
	child1 := PushComponent()
	PopComponent()
	child2 := PushComponent()
	PopComponent()
	PopComponent()

	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}
	if child1.Parent != root {
		t.Fatal("child1 parent should be root")
	}
	if child2.Parent != root {
		t.Fatal("child2 parent should be root")
	}
}

func TestComputedRecomputesAndDisposes(t *testing.T) {
	a := core.NewSignal(1)
	b := core.NewSignal(2)

	PushComponent()
	comp := core.Computed([]core.SignalAccessor{a, b}, func() int { return a.Get() + b.Get() })

	if comp.Get() != 3 {
		t.Fatalf("expected 3, got %d", comp.Get())
	}

	a.Set(10)
	if comp.Get() != 12 {
		t.Fatalf("expected 12 after a change, got %d", comp.Get())
	}

	b.Set(20)
	if comp.Get() != 30 {
		t.Fatalf("expected 30 after b change, got %d", comp.Get())
	}

	frame := CurrentComponent()
	PopComponent()

	for _, d := range frame.Disposers {
		d()
	}

	a.Set(100)
	b.Set(200)
	if got := comp.Get(); got != 30 {
		t.Fatalf("after disposal, expected last cached value 30, got %d", got)
	}
}
