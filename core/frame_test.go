package core

import (
	"strings"
	"testing"
)

func TestNextFrameRunsAtTheStartOfTheNextFlush(t *testing.T) {
	s := NewScheduler()
	requests := 0
	s.OnWork = func() { requests++ }
	var order []string
	s.NextFrame(func() {
		order = append(order, "frame")
		s.Enqueue(Mutation{Type: MutSetProperty, NodeID: 1, Key: "x", Value: 1})
	})
	if requests != 1 || !s.HasFrames() || s.HasWork() {
		t.Fatalf("after NextFrame: requests=%d HasFrames=%v HasWork=%v; want 1, true, false", requests, s.HasFrames(), s.HasWork())
	}
	s.Post(func() { order = append(order, "posted") })
	muts := s.Flush()
	if strings.Join(order, ",") != "frame,posted" {
		t.Fatalf("order %v, want the frame callback first", order)
	}
	if len(muts) != 1 {
		t.Fatalf("the frame callback's mutation didn't go out with the frame: %v", muts)
	}
	if s.HasFrames() {
		t.Fatal("a frame callback ran twice")
	}
}

// A callback registered from a frame callback runs on the following frame,
// and asks for that frame even though a flush is under way.
func TestNextFrameFromAFrameCallbackWaitsForTheNextFrame(t *testing.T) {
	s := NewScheduler()
	requests := 0
	s.OnWork = func() { requests++ }
	runs := 0
	var tick func()
	tick = func() {
		runs++
		s.NextFrame(tick)
	}
	s.NextFrame(tick)
	for frame := 1; frame <= 3; frame++ {
		s.Flush()
		if runs != frame {
			t.Fatalf("after %d frames the callback ran %d times", frame, runs)
		}
	}
	if requests != 4 {
		t.Fatalf("requested %d frames, want 4 (the first, then one per frame)", requests)
	}
}

func TestManualFrames(t *testing.T) {
	s := NewScheduler()
	s.ManualFrames = true
	runs := 0
	s.NextFrame(func() { runs++ })
	s.Flush()
	if runs != 0 {
		t.Fatal("Flush ran a frame callback with ManualFrames set")
	}
	s.RunFrames()
	if runs != 1 || s.HasFrames() {
		t.Fatalf("RunFrames: runs=%d HasFrames=%v", runs, s.HasFrames())
	}
}

// A panicking frame callback is contained and logged; the others still run.
func TestRunFramesContainsPanics(t *testing.T) {
	var logged []LogKind
	SetLogSink(func(e LogEntry) { logged = append(logged, e.Kind) })
	defer SetLogSink(nil)
	s := NewScheduler()
	ran := false
	s.NextFrame(func() { panic("boom") })
	s.NextFrame(func() { ran = true })
	s.RunFrames()
	if !ran || len(logged) != 1 || logged[0] != LogRecoverFrame {
		t.Fatalf("ran=%v logged=%v", ran, logged)
	}
}

// Signal writes in one frame's callbacks notify once, as a batch.
func TestRunFramesBatches(t *testing.T) {
	s := NewScheduler()
	a := NewSignal(0)
	notified := 0
	a.Subscribe(func() { notified++ })
	s.NextFrame(func() { a.Set(1) })
	s.NextFrame(func() { a.Set(2) })
	s.RunFrames()
	if notified != 1 || a.Peek() != 2 {
		t.Fatalf("notified %d times, value %d", notified, a.Peek())
	}
}

func TestNextFrameWithoutAScheduler(t *testing.T) {
	SetActiveScheduler(nil)
	NextFrame(func() { t.Fatal("ran without a scheduler") }) // a no-op, as on the server
}
