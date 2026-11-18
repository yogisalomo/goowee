package core

import "testing"

// #143: two scopes that dirty each other on every render stop after
// maxDirtyPasses with one log entry per flush, and leave no work behind.
func TestDirtyCycleIsLoggedAndDropped(t *testing.T) {
	var logged []LogEntry
	SetLogSink(func(e LogEntry) { logged = append(logged, e) })
	defer SetLogSink(nil)
	s := NewScheduler()
	s.DescribeDirty = func(key any) string { return key.(string) }
	renders := 0
	var a, b func()
	a = func() { renders++; s.MarkDirty("B", 2, b) }
	b = func() { renders++; s.MarkDirty("A", 1, a) }
	s.MarkDirty("A", 1, a)
	s.Flush()
	if renders != maxDirtyPasses {
		t.Fatalf("%d renders, want %d", renders, maxDirtyPasses)
	}
	if len(logged) != 1 || logged[0].Kind != LogSignalCycle || logged[0].Fields["scopes"] != 1 {
		t.Fatalf("logged %+v", logged)
	}
	if paths, _ := logged[0].Fields["components"].([]string); len(paths) != 1 {
		t.Fatalf("components %v", logged[0].Fields["components"])
	}
	if s.HasWork() {
		t.Fatal("the dropped scopes are still pending")
	}
	s.Flush() // the next frame starts clean
	if renders != maxDirtyPasses || len(logged) != 1 {
		t.Fatalf("the next flush re-ran the cycle: %d renders, %d logs", renders, len(logged))
	}
	// A scope marked again later renders normally.
	ok := false
	s.MarkDirty("C", 3, func() { ok = true })
	s.Flush()
	if !ok {
		t.Fatal("a new dirty scope didn't render after the cycle was dropped")
	}
}

// Without DescribeDirty (production) the log has no component list.
func TestDirtyCycleLogWithoutPaths(t *testing.T) {
	var logged []LogEntry
	SetLogSink(func(e LogEntry) { logged = append(logged, e) })
	defer SetLogSink(nil)
	s := NewScheduler()
	var a func()
	a = func() { s.MarkDirty("A", 1, a) }
	s.MarkDirty("A", 1, a)
	s.Flush()
	if len(logged) != 1 {
		t.Fatalf("logged %d entries", len(logged))
	}
	if _, has := logged[0].Fields["components"]; has {
		t.Fatal("component paths without DescribeDirty")
	}
}
