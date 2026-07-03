package core

import (
	"slices"
	"testing"
)

// TestUnsubscribeRemovesOnlyItsOwn — the §2.2 bug regression.
// Two closures from the same source literal in a loop; unsubscribing
// the second must not remove the first (as the old reflect-based
// implementation did, because it matched by code pointer alone).
// This test must FAIL against the old signal.go.
func TestUnsubscribeRemovesOnlyItsOwn(t *testing.T) {
	s := NewSignal(0)
	var calls [2][]int
	var unsubs [2]func()
	for i := 0; i < 2; i++ {
		i := i
		unsubs[i] = s.Subscribe(func() { calls[i] = append(calls[i], s.Get()) })
	}
	unsubs[1]()
	s.Set(1)
	if len(calls[0]) != 1 {
		t.Fatalf("subscriber 0 should still fire, got %v", calls[0])
	}
	if len(calls[1]) != 0 {
		t.Fatalf("subscriber 1 was unsubscribed, got %v", calls[1])
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	s := NewSignal(0)
	var calls []int

	a := func() { calls = append(calls, 1) }
	b := func() { calls = append(calls, 2) }
	c := func() { calls = append(calls, 3) }

	unsubB := s.Subscribe(b)
	s.Subscribe(a)
	s.Subscribe(c)

	unsubB()
	unsubB()

	s.Set(1)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (a and c), got %v", calls)
	}
}

func TestUnsubscribeDuringNotifySkipsRemoved(t *testing.T) {
	s := NewSignal(0)
	var bRan bool

	var unsubB func()
	a := func() { unsubB() }
	b := func() { bRan = true }

	s.Subscribe(a)
	unsubB = s.Subscribe(b)

	s.Set(1)
	if bRan {
		t.Fatal("expected B not to run after being unsubscribed by A")
	}
}

func TestSubscribeDuringNotifyDeferredToNextSet(t *testing.T) {
	s := NewSignal(0)
	var cCalls int

	a := func() {
		s.Subscribe(func() { cCalls++ })
	}
	s.Subscribe(a)
	s.Set(1)

	if cCalls != 0 {
		t.Fatalf("expected C not to run in current pass, got %d calls", cCalls)
	}

	s.Set(2)
	if cCalls != 1 {
		t.Fatalf("expected C to run once on second Set, got %d", cCalls)
	}
}

func TestNestedSetObservesFinalValue(t *testing.T) {
	s := NewSignal(0)
	var lastSeen int
	var ranOnce bool

	a := func() {
		if !ranOnce {
			ranOnce = true
			s.Set(s.Get() + 1)
		}
	}
	b := func() { lastSeen = s.Get() }

	s.Subscribe(a)
	s.Subscribe(b)

	s.Set(1)
	if s.Get() != 2 {
		t.Fatalf("expected final value 2, got %d", s.Get())
	}
	if lastSeen != 2 {
		t.Fatalf("expected B to see 2, got %d", lastSeen)
	}
}

func TestSetCycleDetectionStops(t *testing.T) {
	s := NewSignal(0)
	var callCount int

	s.Subscribe(func() {
		callCount++
		s.Set(s.Get() + 1)
	})

	// Start with a non-equal value to trigger the cycle
	s.Set(1)

	if s.Get() <= 1 {
		t.Fatal("expected value to advance past 1 despite cycle")
	}
	if n := callCount; n > maxNotifyPasses+5 {
		t.Fatalf("expected subscriber calls <= %d, got %d", maxNotifyPasses+5, n)
	}
}

func TestEqualValueSkipsNotify(t *testing.T) {
	s := NewSignal(5)
	var calls int

	s.Subscribe(func() { calls++ })

	s.Set(5)
	if calls != 0 {
		t.Fatalf("expected 0 calls for equal value, got %d", calls)
	}

	s.Set(6)
	if calls != 1 {
		t.Fatalf("expected 1 call for changed value, got %d", calls)
	}
}

func TestNonComparableAlwaysNotifiesWithoutPanic(t *testing.T) {
	s := NewSignal([]int{1, 2, 3})
	var calls int

	s.Subscribe(func() { calls++ })

	s.Set([]int{1, 2, 3})
	s.Set([]int{4, 5, 6})

	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestInterfaceTypedNonComparableValueNoPanic(t *testing.T) {
	s := NewSignal[any]([]int{1})
	var calls int

	s.Subscribe(func() { calls++ })

	s.Set([]int{1})
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestWithEqualsCustom(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	s := NewSignal(User{ID: 1, Name: "alice"}).WithEquals(func(a, b User) bool {
		return a.ID == b.ID
	})
	var calls int
	s.Subscribe(func() { calls++ })

	s.Set(User{ID: 1, Name: "bob"})
	if calls != 0 {
		t.Fatalf("expected 0 calls for equal-with-custom-eq, got %d", calls)
	}

	s.Set(User{ID: 2, Name: "bob"})
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if s.Get().ID != 2 {
		t.Fatalf("expected ID=2, got %d", s.Get().ID)
	}
}

func TestNotificationOrderIsSubscriptionOrder(t *testing.T) {
	s := NewSignal(0)
	var order []int

	s.Subscribe(func() { order = append(order, 1) })
	unsub := s.Subscribe(func() { order = append(order, 2) })
	s.Subscribe(func() { order = append(order, 3) })

	s.Set(1)
	expected := []int{1, 2, 3}
	if !slices.Equal(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}

	unsub()

	order = nil
	s.Set(2)
	expected = []int{1, 3}
	if !slices.Equal(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}



func TestUpdate(t *testing.T) {
	s := NewSignal(0)
	s.Update(func(v int) int { return v + 5 })
	if s.Get() != 5 {
		t.Fatalf("expected 5 after Update, got %d", s.Get())
	}
}

func TestSignalAccessorValue(t *testing.T) {
	s := NewSignal("hello")
	var acc SignalAccessor = s
	if acc.Value() != "hello" {
		t.Fatalf("SignalAccessor.Value() failed")
	}
}
