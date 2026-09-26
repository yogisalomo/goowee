package dom

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// withDevChecks turns dev checks on and captures dependency warnings.
func withDevChecks(t *testing.T) *[]core.LogEntry {
	t.Helper()
	var warns []core.LogEntry
	core.SetDevChecks(true)
	core.SetLogSink(func(e core.LogEntry) {
		if e.Kind == core.LogWarn && strings.Contains(e.Message, "declared dependency") {
			warns = append(warns, e)
		}
	})
	t.Cleanup(func() {
		core.SetDevChecks(false)
		core.SetLogSink(nil)
	})
	return &warns
}

type row struct{ ID int }

// #74: the classic miss — a For row reads a signal the For doesn't depend on.
func TestDevCheckFlagsUndeclaredReadInFor(t *testing.T) {
	warns := withDevChecks(t)
	items := core.NewSignal([]row{{1}, {2}})
	selected := core.NewSignal(1)
	mount(t, h.Ul(h.For(items, func(r row) int { return r.ID }, func(r row) core.Node {
		cls := ""
		if selected.Get() == r.ID { // undeclared: For only depends on items
			cls = "on"
		}
		return h.Li(h.Class(cls))
	})))
	if len(*warns) != 1 {
		t.Fatalf("want one warning (once per read site), got %d: %+v", len(*warns), *warns)
	}
	if read, _ := (*warns)[0].Fields["read"].(string); !strings.Contains(read, "devcheck_test.go:") {
		t.Fatalf("warning should point at the read site, got %q", read)
	}
}

// Peek is an intentional snapshot; declared deps and Computed internals are
// not reported.
func TestDevCheckNoFalsePositives(t *testing.T) {
	warns := withDevChecks(t)
	a, b := core.NewSignal(1), core.NewSignal(2)
	sum := core.Computed([]core.SignalAccessor{a, b}, func() int { return a.Get() + b.Get() })
	show := core.NewSignal(true)
	r, d := mount(t, h.Div(
		hooks.UseScope(func() core.Node {
			return h.P(h.Text(string(rune('0' + sum.Get() + a.Peek())))) // sum declared; a peeked
		}, sum),
		h.Show(show, func() core.Node { return h.Span(h.Textf("%d", b)) }),
	))
	a.Set(5)
	flush(r, d)
	if len(*warns) != 0 {
		t.Fatalf("unexpected warnings: %+v", *warns)
	}
}

// A Computed that reads an undeclared signal is flagged too.
func TestDevCheckFlagsComputed(t *testing.T) {
	warns := withDevChecks(t)
	a, b := core.NewSignal(1), core.NewSignal(2)
	c := core.Computed([]core.SignalAccessor{a}, func() int { return a.Get() + b.Get() })
	c.Get()
	if len(*warns) != 1 || (*warns)[0].Fields["in"] != "Computed" {
		t.Fatalf("want one Computed warning, got %+v", *warns)
	}
}

// With dev checks off nothing is tracked or reported.
func TestDevCheckOffByDefault(t *testing.T) {
	var warns int
	core.SetLogSink(func(e core.LogEntry) {
		if strings.Contains(e.Message, "declared dependency") {
			warns++
		}
	})
	defer core.SetLogSink(nil)
	a, b := core.NewSignal(1), core.NewSignal(2)
	core.Computed([]core.SignalAccessor{a}, func() int { return a.Get() + b.Get() }).Get()
	if warns != 0 {
		t.Fatal("dev checks must be off unless enabled")
	}
}
