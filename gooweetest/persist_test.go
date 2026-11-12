package gooweetest_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/storage"
)

type prefs struct {
	Theme string
	Size  int
}

// The stored value is loaded once the component mounts, and every change is
// saved as JSON.
func TestUsePersistLoadsAndSaves(t *testing.T) {
	storage.Local.Set("prefs", `{"Theme":"dark","Size":14}`)
	p := core.NewSignal(prefs{Theme: "light", Size: 12})
	s := gooweetest.Render(t, core.Component("Prefs", func() core.Node {
		hooks.UsePersist(p, "prefs")
		return Div(
			P(TextS(core.Computed([]core.SignalAccessor{p}, func() string { return fmt.Sprintf("%+v", p.Get()) }))),
			Button(OnClick(func() { p.Update(func(v prefs) prefs { v.Size++; return v }) }), Text("bigger")),
		)
	}))
	if got := s.Find("p").Text(); got != "{Theme:dark Size:14}" {
		t.Fatalf("loaded %q", got)
	}
	s.Click(s.FindByText("bigger"))
	if v, _ := storage.Local.Get("prefs"); v != `{"Theme":"dark","Size":15}` || s.Find("p").Text() != "{Theme:dark Size:15}" {
		t.Fatalf("saved %q, showing %q", v, s.Find("p").Text())
	}
}

// With nothing stored the signal keeps its value, and nothing is written
// until it changes.
func TestUsePersistWithNothingStored(t *testing.T) {
	todos := core.NewSignal([]string{"tea"})
	s := gooweetest.Render(t, core.Component("Todos", func() core.Node {
		hooks.UsePersist(todos, "todos")
		return Button(OnClick(func() { todos.Set(append(todos.Get(), "milk")) }), Text("add"))
	}))
	if len(todos.Get()) != 1 || len(storage.Local.Keys()) != 0 {
		t.Fatalf("todos %v, stored %v", todos.Get(), storage.Local.Keys())
	}
	s.Click(s.FindByText("add"))
	if v, _ := storage.Local.Get("todos"); v != `["tea","milk"]` {
		t.Fatalf("saved %q", v)
	}
}

func TestUsePersistInSessionStorage(t *testing.T) {
	storage.Session.Set("step", "3")
	step := core.NewSignal(1)
	s := gooweetest.Render(t, core.Component("Wizard", func() core.Node {
		hooks.UsePersist(step, "step", hooks.PersistOptions{Store: storage.Session})
		return Button(OnClick(func() { step.Set(step.Get() + 1) }), Textf("step %d", step))
	}))
	s.Click(s.Find("button"))
	if s.Find("button").Text() != "step 4" {
		t.Fatalf("button %q", s.Find("button").Text())
	}
	if v, _ := storage.Session.Get("step"); v != "4" {
		t.Fatalf("session %q", v)
	}
	if _, ok := storage.Local.Get("step"); ok {
		t.Fatal("wrote to localStorage")
	}
}

// A stored value of another shape is logged and left alone; the next change
// is saved over it.
func TestUsePersistIgnoresAValueThatDoesNotDecode(t *testing.T) {
	var mu sync.Mutex
	var warnings []string
	core.SetLogSink(func(e core.LogEntry) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == core.LogWarn {
			warnings = append(warnings, fmt.Sprint(e.Message, " ", e.Fields["key"]))
		}
	})
	defer core.SetLogSink(nil)

	storage.Local.Set("count", `"seven"`)
	count := core.NewSignal(1)
	s := gooweetest.Render(t, core.Component("Count", func() core.Node {
		hooks.UsePersist(count, "count")
		return Button(OnClick(func() { count.Set(count.Get() + 1) }), Textf("%d", count))
	}))
	if count.Get() != 1 {
		t.Fatalf("count = %d", count.Get())
	}
	mu.Lock()
	logged := len(warnings) == 1 && strings.Contains(warnings[0], "doesn't decode") && strings.HasSuffix(warnings[0], " count")
	mu.Unlock()
	if !logged {
		t.Fatalf("warnings %q", warnings)
	}
	s.Click(s.Find("button"))
	if v, _ := storage.Local.Get("count"); v != "2" {
		t.Fatalf("saved %q", v)
	}
}

// Persisting lasts as long as the component.
func TestUsePersistStopsWhenUnmounted(t *testing.T) {
	note := core.NewSignal("")
	show := core.NewSignal(true)
	s := gooweetest.Render(t, Div(
		Show(show, func() core.Node {
			return core.Component("Editor", func() core.Node {
				hooks.UsePersist(note, "note")
				return Textarea(BindValue(note))
			})
		}),
		Button(OnClick(func() { show.Set(false) }), Text("close")),
	))
	s.Input(s.Find("textarea"), "draft")
	if v, _ := storage.Local.Get("note"); v != `"draft"` {
		t.Fatalf("saved %q", v)
	}
	s.Click(s.FindByText("close"))
	note.Set("after")
	s.Flush()
	if v, _ := storage.Local.Get("note"); v != `"draft"` {
		t.Fatalf("saved after unmount: %q", v)
	}
}
