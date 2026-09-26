package hooks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// waitLoaded drains the scheduler (applying the goroutine's posted result) and
// polls until the resource stops loading, or fails on timeout.
func waitLoaded(t *testing.T, s *core.Scheduler, loading *core.Signal[bool]) {
	t.Helper()
	for i := 0; i < 500; i++ {
		s.Flush()
		if !loading.Get() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("resource still loading after timeout")
}

func TestUseResourceSuccess(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()

	res := UseResource(nil, func(context.Context) (string, error) { return "hello", nil })
	if !res.Loading.Get() {
		t.Fatal("expected Loading=true before the fetch resolves")
	}
	waitLoaded(t, sched, res.Loading)
	if res.Data.Get() != "hello" {
		t.Fatalf("Data = %q, want hello", res.Data.Get())
	}
	if res.Err.Get() != nil {
		t.Fatalf("Err = %v, want nil", res.Err.Get())
	}
}

func TestUseResourceError(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()

	res := UseResource(nil, func(context.Context) (int, error) { return 0, errors.New("boom") })
	waitLoaded(t, sched, res.Loading)
	if res.Err.Get() == nil || res.Err.Get().Error() != "boom" {
		t.Fatalf("Err = %v, want boom", res.Err.Get())
	}
}

func TestUseResourceRefetch(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()

	var n int64
	res := UseResource(nil, func(context.Context) (int64, error) { return atomic.AddInt64(&n, 1), nil })
	waitLoaded(t, sched, res.Loading)
	if res.Data.Get() != 1 {
		t.Fatalf("first load Data = %d, want 1", res.Data.Get())
	}

	res.Refetch()
	waitLoaded(t, sched, res.Loading)
	if res.Data.Get() != 2 {
		t.Fatalf("after Refetch Data = %d, want 2", res.Data.Get())
	}
}

// blockingFetch returns a fetch that waits for release (or its context) and
// records each call's context so a test can see which were cancelled.
func blockingFetch(release chan int) (func(context.Context) (int, error), chan context.Context) {
	ctxs := make(chan context.Context, 8)
	return func(ctx context.Context) (int, error) {
		ctxs <- ctx
		select {
		case v := <-release:
			return v, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}, ctxs
}

// #75: Refetch cancels the fetch still in flight; only the newest result lands.
func TestUseResourceRefetchCancelsInFlight(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()

	release := make(chan int)
	fetch, ctxs := blockingFetch(release)
	res := UseResource(nil, fetch)
	first := <-ctxs
	res.Refetch()
	waitCancelled(t, first)
	release <- 2
	waitLoaded(t, sched, res.Loading)
	if res.Data.Get() != 2 || res.Err.Get() != nil {
		t.Fatalf("want the newest result 2 and no error, got %d / %v", res.Data.Get(), res.Err.Get())
	}
}

// #75: unmounting cancels the in-flight fetch and ignores its result.
func TestUseResourceUnmountCancels(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	frame := runtime.PushComponent()
	release := make(chan int, 1)
	fetch, ctxs := blockingFetch(release)
	res := UseResource(nil, fetch)
	runtime.PopComponent()

	RunFrameCleanup(frame)
	waitCancelled(t, <-ctxs)
	for i := 0; i < 20; i++ {
		sched.Flush()
		time.Sleep(time.Millisecond)
	}
	if res.Err.Get() != nil {
		t.Fatalf("a cancelled fetch's error must be ignored, got %v", res.Err.Get())
	}
}

// #75: a dep change cancels the previous fetch.
func TestUseResourceDepChangeCancels(t *testing.T) {
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	defer core.SetActiveScheduler(nil)
	runtime.PushComponent()
	defer runtime.PopComponent()

	id := core.NewSignal(1)
	release := make(chan int)
	fetch, ctxs := blockingFetch(release)
	res := UseResource([]core.SignalAccessor{id}, fetch)
	first := <-ctxs
	id.Set(2)
	waitCancelled(t, first)
	release <- 20
	waitLoaded(t, sched, res.Loading)
	if res.Data.Get() != 20 {
		t.Fatalf("want 20, got %d", res.Data.Get())
	}
}

func waitCancelled(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("fetch context was not cancelled")
	}
}
