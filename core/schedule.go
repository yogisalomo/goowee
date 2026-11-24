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

// ScrollWindow scrolls the document to (x, y) after the current batch of DOM
// updates. It is the window-scrolling counterpart of ref.Call("scrollTo", …)
// and is used by h.VirtualList's ScrollToIndex/ScrollToKey handle. No-op in
// SSR and tests without a running client.
func ScrollWindow(x, y float64) {
	if s := scheduler(); s != nil {
		s.Enqueue(Mutation{Type: MutCall, NodeID: 0, Key: "scrollTo", Value: []any{x, y}})
	}
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

// AfterFrame runs fn after the DOM mutations of the current frame have been
// applied, on the render loop. It is a frame effect: callbacks queue in order
// and run in the same batch as OnMount/UseEffect effects, so the elements a
// signal write replaces are in the document by the time fn runs. Use it for
// post-navigation work — scrolling, focus, measuring — rather than Schedule,
// which runs before that frame's DOM is applied.
//
// No-op when no scheduler is active (SSR, or tests without a running client).
func AfterFrame(fn func()) {
	if s := scheduler(); s != nil {
		s.QueueEffect(fn)
	}
}
