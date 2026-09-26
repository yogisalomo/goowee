package core

// Batch runs fn and defers every signal notification it causes until fn
// returns: values written inside fn are visible immediately (a Get sees the new
// value), but each written signal notifies its subscribers once, after the
// outermost Batch ends. Derived state — Computed, Watch, effects — therefore
// never observes a half-applied update:
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
	for len(batchQueue) > 0 {
		q := batchQueue
		batchQueue = nil
		for _, n := range q {
			n.flushBatched()
		}
	}
}
