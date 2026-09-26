package core

import "testing"

// #58: an unobserved Computed holds no subscriptions, so one created and
// dropped (h.Textf in a re-rendering scope, SSR) can't leak.
func TestComputedIsLazyAndReleasesDeps(t *testing.T) {
	a := NewSignal(1)
	evals := 0
	c := Computed([]SignalAccessor{a}, func() int { evals++; return a.Get() * 2 })
	if a.SubscriberCount() != 0 {
		t.Fatalf("an unobserved computed must not subscribe, got %d", a.SubscriberCount())
	}
	if c.Get() != 2 {
		t.Fatalf("want 2, got %d", c.Get())
	}
	a.Set(5)
	if c.Get() != 10 {
		t.Fatalf("an unobserved computed must recompute on read, got %d", c.Get())
	}
	n := evals
	c.Get()
	if evals != n {
		t.Fatal("an unchanged dep must not trigger re-evaluation")
	}

	var seen []int
	unsub := c.Subscribe(func() { seen = append(seen, c.Get()) })
	if a.SubscriberCount() != 1 {
		t.Fatalf("observing a computed subscribes it to deps, got %d", a.SubscriberCount())
	}
	a.Set(6)
	if len(seen) != 1 || seen[0] != 12 {
		t.Fatalf("observer should see 12, got %v", seen)
	}
	unsub()
	if a.SubscriberCount() != 0 {
		t.Fatalf("losing its last observer releases the deps, got %d", a.SubscriberCount())
	}
	a.Set(7)
	if c.Get() != 14 {
		t.Fatalf("after deactivation reads still work, got %d", c.Get())
	}
}

// Reading an observed computed inside a batch sees the batch's writes, and the
// read does not swallow the notification that follows.
func TestComputedReadInsideBatchIsFreshAndStillNotifies(t *testing.T) {
	a := NewSignal(1)
	c := Computed([]SignalAccessor{a}, func() int { return a.Get() + 100 })
	var notified []int
	c.Subscribe(func() { notified = append(notified, c.Get()) })
	Batch(func() {
		a.Set(2)
		if got := c.Get(); got != 102 {
			t.Fatalf("read inside batch should be fresh, got %d", got)
		}
	})
	if len(notified) != 1 || notified[0] != 102 {
		t.Fatalf("observers must still be notified once, got %v", notified)
	}
}

// A chain of computeds activates and deactivates transitively.
func TestComputedChain(t *testing.T) {
	a := NewSignal(1)
	b := Computed([]SignalAccessor{a}, func() int { return a.Get() + 1 })
	c := Computed([]SignalAccessor{b}, func() int { return b.Get() * 10 })
	got := 0
	unsub := c.Subscribe(func() { got = c.Get() })
	a.Set(4)
	if got != 50 {
		t.Fatalf("want 50, got %d", got)
	}
	unsub()
	if a.SubscriberCount() != 0 || b.SubscriberCount() != 0 {
		t.Fatalf("chain must fully release: a=%d b=%d", a.SubscriberCount(), b.SubscriberCount())
	}
	a.Set(9)
	if c.Get() != 100 {
		t.Fatalf("inactive chain reads through, got %d", c.Get())
	}
}
