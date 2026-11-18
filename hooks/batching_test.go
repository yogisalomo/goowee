package hooks

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// #138: one batch that writes several deps runs a Watch once.
func TestWatchRunsOncePerBatch(t *testing.T) {
	runtime.PushComponent()
	defer runtime.PopComponent()
	a, b := core.NewSignal(0), core.NewSignal(0)
	runs := 0
	var seen [2]int
	Watch([]core.SignalAccessor{a, b}, func() { runs++; seen = [2]int{a.Get(), b.Get()} })
	core.Batch(func() { a.Set(1); b.Set(1) })
	if runs != 1 || seen != [2]int{1, 1} {
		t.Fatalf("runs = %d, saw %v; want 1 run seeing [1 1]", runs, seen)
	}
	a.Set(2) // outside a batch: runs at once
	if runs != 2 || seen[0] != 2 {
		t.Fatalf("unbatched write: runs = %d, saw %v", runs, seen)
	}
}

// UseEffect re-runs once per batch: one cleanup, one run.
func TestUseEffectRunsOncePerBatch(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	frame := runtime.PushComponent()
	defer runtime.PopComponent()
	a, b := core.NewSignal(0), core.NewSignal(0)
	var log []string
	UseEffect([]core.SignalAccessor{a, b}, func() func() {
		log = append(log, "run")
		return func() { log = append(log, "cleanup") }
	})
	sched.RunEffects()
	core.Batch(func() { a.Set(1); b.Set(1) })
	if got := strings.Join(log, ","); got != "run,cleanup,run" {
		t.Fatalf("log %s, want run,cleanup,run", got)
	}
	RunFrameCleanup(frame)
	core.Batch(func() { a.Set(2) })
	if got := strings.Join(log, ","); got != "run,cleanup,run,cleanup" {
		t.Fatalf("after unmount: %s", got)
	}
}

// An effect disposed while its run is queued doesn't run.
func TestUseEffectDisposedInTheSameBatch(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	frame := runtime.PushComponent()
	defer runtime.PopComponent()
	a := core.NewSignal(0)
	runs := 0
	UseEffect([]core.SignalAccessor{a}, func() func() { runs++; return nil })
	sched.RunEffects()
	core.Batch(func() {
		a.Set(1)
		RunFrameCleanup(frame)
	})
	if runs != 1 {
		t.Fatalf("runs = %d; the disposed effect re-ran", runs)
	}
}

// An effect that writes one of its own deps runs again with the new value:
// the update isn't lost.
func TestEffectWritingItsDepReruns(t *testing.T) {
	runtime.PushComponent()
	defer runtime.PopComponent()
	a, b := core.NewSignal(0), core.NewSignal(0)
	var seen []int
	Watch([]core.SignalAccessor{a, b}, func() {
		seen = append(seen, a.Get())
		if a.Get() < 3 {
			a.Set(a.Get() + 1)
		}
	})
	core.Batch(func() { a.Set(1); b.Set(1) })
	if len(seen) != 3 || seen[2] != 3 {
		t.Fatalf("saw %v, want [1 2 3]", seen)
	}
}

// One that always writes its dep is stopped and logged, not looped forever.
func TestEffectCycleIsBounded(t *testing.T) {
	var logged []core.LogKind
	core.SetLogSink(func(e core.LogEntry) { logged = append(logged, e.Kind) })
	defer core.SetLogSink(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()
	a, b := core.NewSignal(0), core.NewSignal(0)
	runs := 0
	Watch([]core.SignalAccessor{a, b}, func() { runs++; a.Set(a.Peek() + 1) })
	core.Batch(func() { a.Set(1); b.Set(1) })
	if len(logged) == 0 || logged[len(logged)-1] != core.LogSignalCycle {
		t.Fatalf("no cycle logged (runs %d, logged %v)", runs, logged)
	}
	if runs > 2000 {
		t.Fatalf("ran %d times", runs)
	}
	// The next batch works normally.
	before := runs
	core.Batch(func() { b.Set(2) })
	if runs == before {
		t.Fatal("the effect stopped for good after a cycle")
	}
}

// UseResource with two deps written together fetches once.
func TestUseResourceFetchesOncePerBatch(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()
	page, filter := core.NewSignal(1), core.NewSignal("all")
	var calls int64
	res := UseResource([]core.SignalAccessor{page, filter}, func(context.Context) (int64, error) {
		return atomic.AddInt64(&calls, 1), nil
	})
	sched.RunEffects()
	waitLoaded(t, sched, res.Loading)
	core.Batch(func() { page.Set(2); filter.Set("go") })
	waitLoaded(t, sched, res.Loading)
	if n := atomic.LoadInt64(&calls); n != 2 {
		t.Fatalf("fetch called %d times, want 2 (mount, then one per batch)", n)
	}
}
