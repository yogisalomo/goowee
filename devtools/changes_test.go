package devtools_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/hooks"
)

func last(t *testing.T) devtools.Change {
	t.Helper()
	cs := devtools.Changes()
	if len(cs) == 0 {
		t.Fatal("no changes logged")
	}
	return cs[len(cs)-1]
}

type cart struct {
	Items []string
	Total int
}

type store struct{ cart *core.Signal[cart] }

func (s *store) add(item string, price int) {
	c := s.cart.Get()
	s.cart.Set(cart{Items: append(append([]string(nil), c.Items...), item), Total: c.Total + price})
}

func TestChangeLogNamesTheSignalAndTheWriter(t *testing.T) {
	devtools.Enable()
	devtools.ClearChanges()
	s := &store{cart: core.NewSignal(cart{}).Named("cart")}
	s.add("tea", 3)

	c := last(t)
	if c.Name != "cart" || c.Signal == 0 || c.Signal != devtools.SignalID(s.cart) {
		t.Fatalf("signal: %+v", c)
	}
	if c.From != "{Items:[] Total:0}" || c.To != "{Items:[tea] Total:3}" {
		t.Fatalf("values: from %q to %q", c.From, c.To)
	}
	// The store method is the "action": the frame that called Set.
	if !strings.HasPrefix(c.At, "devtools/changes_test.go:") || c.Func != "devtools_test.(*store).add" {
		t.Fatalf("site: %s %s", c.At, c.Func)
	}
}

// A write through a hook's setter is reported at the app's call, not inside
// goowee.
func TestChangeLogSkipsGooweeFrames(t *testing.T) {
	devtools.Enable()
	devtools.ClearChanges()
	count, setCount := hooks.UseState(0)
	count.Named("count")
	setCount(1)
	c := last(t)
	if c.Name != "count" || c.From != "0" || c.To != "1" {
		t.Fatalf("change: %+v", c)
	}
	if !strings.HasPrefix(c.At, "devtools/changes_test.go:") || c.Func != "devtools_test.TestChangeLogSkipsGooweeFrames" {
		t.Fatalf("site: %s %s", c.At, c.Func)
	}
}

func TestChangeLogOrderSkipsAndCap(t *testing.T) {
	devtools.Enable()
	devtools.ClearChanges()
	s := core.NewSignal(0) // not named or registered: logged once written
	s.Set(0)               // no change, no entry
	for i := 1; i <= devtools.MaxChanges+5; i++ {
		s.Set(i)
	}
	cs := devtools.Changes()
	if len(cs) != devtools.MaxChanges {
		t.Fatalf("kept %d changes", len(cs))
	}
	if cs[0].To != "6" || cs[len(cs)-1].To != fmt.Sprint(devtools.MaxChanges+5) {
		t.Fatalf("oldest %+v newest %+v", cs[0], cs[len(cs)-1])
	}
	for i := 1; i < len(cs); i++ {
		if cs[i].Seq != cs[i-1].Seq+1 || cs[i].MS < cs[i-1].MS {
			t.Fatalf("out of order at %d: %+v %+v", i, cs[i-1], cs[i])
		}
	}
	if cs[0].Signal == 0 || cs[0].Name != "" {
		t.Fatalf("an unnamed signal gets an id once written: %+v", cs[0])
	}
	devtools.ClearChanges()
	if len(devtools.Changes()) != 0 {
		t.Fatal("ClearChanges left entries")
	}
}

func TestChangeLogTruncatesBigValues(t *testing.T) {
	devtools.Enable()
	devtools.ClearChanges()
	s := core.NewSignal("")
	s.Set(strings.Repeat("é", 300))
	if to := last(t).To; !strings.HasSuffix(to, "…") || len(to) > 205 || !strings.HasPrefix(to, "éé") {
		t.Fatalf("to = %q (%d bytes)", to, len(to))
	}
}

// Named shows in the inspector, also on a signal a hook registered first.
func TestSnapshotShowsNames(t *testing.T) {
	devtools.Enable()
	total := core.Computed(nil, func() int { return 3 }).Named("total")
	snap := devtools.Snapshot()
	for _, sig := range snap["signals"].([]map[string]any) {
		if sig["id"] == devtools.SignalID(total) {
			if sig["name"] != "total" || sig["kind"] != "computed" {
				t.Fatalf("signal %+v", sig)
			}
			return
		}
	}
	t.Fatal("named computed not in the snapshot")
}

// A value read for the snapshot that writes a signal (a Computed misusing its
// compute function) must not deadlock against the change log.
func TestSnapshotDoesNotDeadlockOnAWriteDuringARead(t *testing.T) {
	devtools.Enable()
	side := core.NewSignal(0)
	src := core.NewSignal(1)
	bad := core.Computed([]core.SignalAccessor{src}, func() int { side.Set(side.Peek() + 1); return src.Get() }).Named("bad")
	src.Set(2) // stale: the snapshot's read recomputes it
	done := make(chan struct{})
	go func() { devtools.Snapshot(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Snapshot deadlocked")
	}
	_ = bad
}
