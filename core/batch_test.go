package core

import (
	"fmt"
	"testing"
)

// #73: without batching, a Computed over two signals written in one handler
// observed the half-applied state (1,0).
func TestBatchDerivedStateSeesOnlyFinalValues(t *testing.T) {
	x, y := NewSignal(0), NewSignal(0)
	var seen []string
	c := Computed([]SignalAccessor{x, y}, func() int {
		seen = append(seen, fmt.Sprintf("(%d,%d)", x.Get(), y.Get()))
		return x.Get() + y.Get()
	})
	c.Subscribe(func() {}) // observed, so it recomputes on dep changes
	seen = nil
	Batch(func() {
		x.Set(1)
		y.Set(1)
		if x.Get() != 1 || y.Get() != 1 {
			t.Fatal("values written inside a batch must be readable immediately")
		}
		if len(seen) != 0 {
			t.Fatalf("no notification may run inside the batch, got %v", seen)
		}
	})
	for _, s := range seen {
		if s != "(1,1)" {
			t.Fatalf("derived state observed an intermediate value: %v", seen)
		}
	}
	if len(seen) == 0 {
		t.Fatal("the batch must notify when it ends")
	}
}

func TestBatchNotifiesEachSignalOnce(t *testing.T) {
	s := NewSignal(0)
	n := 0
	s.Subscribe(func() { n++ })
	Batch(func() {
		for i := 1; i <= 10; i++ {
			s.Set(i)
		}
	})
	if n != 1 || s.Get() != 10 {
		t.Fatalf("want one notification with the final value, got n=%d value=%d", n, s.Get())
	}
}

func TestBatchNestsAndFlushesAtOutermost(t *testing.T) {
	s := NewSignal(0)
	n := 0
	s.Subscribe(func() { n++ })
	Batch(func() {
		Batch(func() { s.Set(1) })
		if n != 0 {
			t.Fatal("an inner batch must not flush")
		}
		s.Set(2)
	})
	if n != 1 {
		t.Fatalf("want 1 notification, got %d", n)
	}
}

func TestBatchFlushesWhenFnPanics(t *testing.T) {
	s := NewSignal(0)
	n := 0
	s.Subscribe(func() { n++ })
	func() {
		defer func() { _ = recover() }()
		Batch(func() {
			s.Set(1)
			panic("boom")
		})
	}()
	if n != 1 {
		t.Fatalf("pending notifications must still be delivered, got %d", n)
	}
	// And the batch state is reset: later writes notify immediately.
	s.Set(2)
	if n != 2 {
		t.Fatalf("batch depth leaked after a panic: n=%d", n)
	}
}

func TestBatchWritesDuringFlushNotify(t *testing.T) {
	a, b := NewSignal(0), NewSignal(0)
	a.Subscribe(func() { b.Set(a.Get() * 10) })
	got := 0
	b.Subscribe(func() { got = b.Get() })
	Batch(func() { a.Set(3) })
	if got != 30 {
		t.Fatalf("a write made by a subscriber during the flush must propagate, got %d", got)
	}
}

func TestScheduledCallbacksAreBatchedAndContained(t *testing.T) {
	s := NewScheduler()
	x, y := NewSignal(0), NewSignal(0)
	var seen []string
	c := Computed([]SignalAccessor{x, y}, func() int {
		seen = append(seen, fmt.Sprintf("(%d,%d)", x.Get(), y.Get()))
		return x.Get() + y.Get()
	})
	c.Subscribe(func() {})
	seen = nil
	var captured LogEntry
	SetLogSink(func(e LogEntry) { captured = e })
	defer SetLogSink(nil)

	s.Post(func() { x.Set(1) })
	s.Post(func() { panic("bad callback") })
	s.Post(func() { y.Set(1) })
	s.Flush()
	if len(seen) == 0 {
		t.Fatal("computed never re-evaluated")
	}
	for _, v := range seen {
		if v != "(1,1)" {
			t.Fatalf("posted callbacks should land as one batch, saw %v", seen)
		}
	}
	if y.Get() != 1 {
		t.Fatal("a panicking callback must not stop the ones after it")
	}
	if captured.Kind != LogRecoverScheduled {
		t.Fatalf("want a %s log entry, got %+v", LogRecoverScheduled, captured)
	}
}
