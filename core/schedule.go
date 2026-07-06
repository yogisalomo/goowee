package core

import "sync"

var (
	activeSchedulerMu sync.Mutex
	activeScheduler   *Scheduler
)

// SetActiveScheduler registers the scheduler that Schedule targets. The client
// runtime sets this once at startup (see bridge.Init). SSR and tests leave it
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
	activeSchedulerMu.Lock()
	s := activeScheduler
	activeSchedulerMu.Unlock()
	if s != nil {
		s.Post(fn)
	}
}
