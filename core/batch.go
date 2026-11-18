package core

// Batch runs fn and defers every signal notification it causes until fn
// returns: values written inside fn are visible immediately (a Get sees the new
// value), but each written signal notifies its subscribers once, after the
// outermost Batch ends. Derived state — Computed, Watch, effects — therefore
// never observes a half-applied update, and a Watch or UseEffect over several
// signals the batch wrote runs once:
//
//	core.Batch(func() {
//	    first.Set("Ada")
//	    last.Set("Lovelace") // a Computed over both runs once, seeing both
//	})
//
// Batches nest; only the outermost one flushes. If fn panics, the pending
// notifications are still delivered before the panic continues.
//
// You rarely need to call it: event handlers, core.Schedule callbacks, ref
// read replies, and effect runs are batched automatically. Like signals
// themselves (ADR-010), Batch is for the render loop — don't call it from other
// goroutines; use Schedule.
func Batch(fn func()) {
	batchDepth++
	defer endBatch()
	fn()
}

// batchNotifier is a signal with a notification deferred by Batch.
type batchNotifier interface{ flushBatched() }

var (
	batchDepth int
	batchQueue []batchNotifier
)

func endBatch() {
	batchDepth--
	if batchDepth > 0 {
		return
	}
	// Every value is already stored, so the first subscriber to run already
	// sees the final state of all signals written in the batch. Notifications
	// may write more signals (depth is 0 now, so those notify immediately, or
	// queue again under a nested Batch) — loop until the queue is drained.
	// Coalesced callbacks (Watch, UseEffect) triggered by the notifications
	// wait for the whole round and then run once each, in the order they
	// were first triggered; what they write starts the next round.
	if delivering {
		return // an outer endBatch's loop is draining the queues
	}
	delivering = true
	defer func() { delivering = false }()
	for pass := 0; len(batchQueue) > 0 || len(pendingEffects) > 0; pass++ {
		for len(batchQueue) > 0 {
			q := batchQueue
			batchQueue = nil
			for _, n := range q {
				n.flushBatched()
			}
		}
		fx := pendingEffects
		pendingEffects = nil
		for _, e := range fx {
			e.pending = false // before any runs: a panicking one can't strand the rest
		}
		if pass >= maxNotifyPasses {
			// Effects keep writing what triggers them: stop running them
			// for this batch. Signals bound their own notify cycles, so with
			// no effect running the loop ends.
			if pass == maxNotifyPasses {
				Log(LogSignalCycle, "effect update cycle detected; dropping pending effects", map[string]any{
					"passes":  maxNotifyPasses,
					"effects": len(fx),
				})
			}
			continue
		}
		for _, e := range fx {
			if !e.stopped {
				Batch(e.fn) // its writes notify after it returns, as one batch
			}
		}
	}
}

// delivering is set while endBatch drains the queues: a callback that
// coalesces then waits for the current round instead of running at once.
var delivering bool

type coalesced struct {
	fn      func()
	pending bool
	stopped bool
}

var pendingEffects []*coalesced

// Coalesce returns a trigger for fn that runs fn at most once per batch
// round: called while a Batch is open or its notifications are being
// delivered, it queues fn (once, however often it is called) to run after
// every notification of the round; called otherwise, it runs fn at once.
// Subscribe the trigger to several signals and a batch that writes all of
// them runs fn once, seeing all the new values. hooks.Watch and
// hooks.UseEffect are built on it (#138).
//
// stop cancels a queued run and makes later triggers no-ops: call it when
// the owner is disposed.
func Coalesce(fn func()) (trigger, stop func()) {
	c := &coalesced{fn: fn}
	trigger = func() {
		if c.stopped {
			return
		}
		if batchDepth > 0 || delivering {
			if !c.pending {
				c.pending = true
				pendingEffects = append(pendingEffects, c)
			}
			return
		}
		Batch(c.fn)
	}
	stop = func() { c.stopped = true }
	return trigger, stop
}
