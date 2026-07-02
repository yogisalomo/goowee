package hooks

import (
	"goowee/core"
	"testing"
)

func TestUseState(t *testing.T) {
	comp := core.PushComponent()
	defer core.PopComponent()

	sig, setter := UseState(0)
	if sig.Get() != 0 {
		t.Fatal("expected initial value 0")
	}
	setter(42)
	if sig.Get() != 42 {
		t.Fatal("expected value 42 after setter")
	}
	_ = comp
}

func TestUseEffect(t *testing.T) {
	sig := core.NewSignal(0)
	var result int
	UseEffect([]core.SignalAccessor{sig}, func() func() {
		result = sig.Get()
		return nil
	})
	if result != 0 {
		t.Fatal("expected initial effect run with value 0")
	}
	sig.Set(1)
	if result != 1 {
		t.Fatal("expected effect re-run with value 1")
	}
}

func TestUseEffectCleanup(t *testing.T) {
	sig := core.NewSignal(0)
	var cleanups []int
	UseEffect([]core.SignalAccessor{sig}, func() func() {
		id := sig.Get()
		return func() {
			cleanups = append(cleanups, id)
		}
	})
	sig.Set(1)
	if len(cleanups) != 1 || cleanups[0] != 0 {
		t.Fatalf("expected cleanup [0], got %v", cleanups)
	}
	sig.Set(2)
	if len(cleanups) != 2 || cleanups[1] != 1 {
		t.Fatalf("expected cleanup [0,1], got %v", cleanups)
	}
}

func TestUseStateMultiple(t *testing.T) {
	core.PushComponent()

	s1, _ := UseState("a")
	s2, _ := UseState(1)
	s3, _ := UseState(true)

	if s1.Get() != "a" || s2.Get() != 1 || s3.Get() != true {
		t.Fatal("multiple UseState calls should isolate correctly")
	}
	core.PopComponent()
}

func TestUseStateNoComponentContext(t *testing.T) {
	sig, setter := UseState(0)
	if sig.Get() != 0 {
		t.Fatal("UseState should work even outside component context")
	}
	setter(5)
	if sig.Get() != 5 {
		t.Fatal("setter should update value outside component context")
	}
}
