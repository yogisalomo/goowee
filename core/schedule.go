package core

import "sync"

var (
	activeSchedulerMu sync.Mutex
	activeScheduler   *Scheduler
)

// SetActiveScheduler registers the scheduler that Schedule targets. The client
// runtime sets this once at startup (see bridge.Run). SSR and tests leave it
// unset, so Schedule is a no-op there.
func SetActiveScheduler(s *Scheduler) {
	activeSchedulerMu.Lock()
	activeScheduler = s
	activeSchedulerMu.Unlock()
}

// Schedule runs fn on the render loop at the next frame. Call it from
// goroutines — timers, network/fetch callbacks, channels — instead of mutating
// signals directly: signals and the renderer are not safe to touch off the
// render loop. Inside fn you Get/Set signals normally.
//
// No-op when no scheduler is active (SSR, or tests without a running client).
func Schedule(fn func()) {
	if s := scheduler(); s != nil {
		s.Post(fn)
	}
}

func scheduler() *Scheduler {
	activeSchedulerMu.Lock()
	defer activeSchedulerMu.Unlock()
	return activeScheduler
}

// NextFrame runs fn once at the start of the next frame, on the render loop;
// call it again from fn to keep animating. Frames follow the display's
// refresh rate, and stop while the page is hidden (a background tab doesn't
// animate). For drawing, canvas.Animate wraps this.
//
// No-op when no scheduler is active (SSR, or tests without a running client).
func NextFrame(fn func()) {
	if s := scheduler(); s != nil {
		s.NextFrame(fn)
	}
}
