package core

import "testing"

func TestAfterFrameWaitsForEffects(t *testing.T) {
	s := NewScheduler()
	SetActiveScheduler(s)
	defer SetActiveScheduler(nil)

	ran := false
	AfterFrame(func() { ran = true })
	if !s.HasWork() {
		t.Fatal("AfterFrame did not queue work")
	}
	s.Flush() // no DOM mutations here, but the frame effect is not an effect yet
	if ran {
		t.Fatal("AfterFrame ran before frame effects")
	}
	s.RunEffects()
	if !ran {
		t.Fatal("AfterFrame did not run with frame effects")
	}
}
