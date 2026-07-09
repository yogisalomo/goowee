package hooks

import (
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

	res := UseResource(nil, func() (string, error) { return "hello", nil })
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

	res := UseResource(nil, func() (int, error) { return 0, errors.New("boom") })
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
	res := UseResource(nil, func() (int64, error) { return atomic.AddInt64(&n, 1), nil })
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
