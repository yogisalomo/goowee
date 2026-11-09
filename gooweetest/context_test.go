package gooweetest_test

import (
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
)

// #111: a typed, tree-scoped context.
var theme = core.NewContext("light")

// badge shows the theme its nearest provider supplies.
func badge(class string) core.Node {
	return theme.Use(func(t *core.Signal[string]) core.Node {
		return Span(Class(class), TextS(t))
	})
}

func TestContextDefaultShadowingAndUpdates(t *testing.T) {
	outer, inner := core.NewSignal("dark"), core.NewSignal("solarized")
	s := gooweetest.Render(t, Div(
		badge("none"),
		theme.Provide(outer,
			badge("outer"),
			theme.Provide(inner, badge("inner")),
			core.Component("Deep", func() core.Node { return Div(Div(badge("deep"))) }),
		),
	))
	for sel, want := range map[string]string{"span.none": "light", "span.outer": "dark", "span.inner": "solarized", "span.deep": "dark"} {
		if got := s.Find(sel).Text(); got != want {
			t.Errorf("%s: %q, want %q", sel, got, want)
		}
	}
	outer.Set("high-contrast")
	s.Flush()
	if s.Find("span.outer").Text() != "high-contrast" || s.Find("span.deep").Text() != "high-contrast" || s.Find("span.inner").Text() != "solarized" {
		t.Fatalf("after the provider's signal changed: %s", s.HTML())
	}
}

// Content mounted later (a Show turned on, a For row added) and a provider
// re-rendered by its parent still resolve to the provider above them.
func TestContextReachesLaterRenders(t *testing.T) {
	val := core.NewSignal("dark")
	show := core.NewSignal(false)
	rows := core.NewSignal([]string{"a"})
	page := core.NewSignal(1)
	s := gooweetest.Render(t, theme.Provide(val,
		Show(show, func() core.Node { return badge("shown") }),
		Ul(For(rows, func(r string) string { return r }, func(r string) core.Node { return Li(badge("row-" + r)) })),
		Switch(page, map[int]func() core.Node{
			1: func() core.Node { return theme.Provide(core.NewSignal("page1"), badge("page")) },
			2: func() core.Node { return theme.Provide(core.NewSignal("page2"), badge("page"), badge("page2-extra")) },
		}, nil),
	))
	show.Set(true)
	rows.Set([]string{"a", "b"})
	s.Flush()
	if s.Find("span.shown").Text() != "dark" || s.Find("span.row-b").Text() != "dark" {
		t.Fatalf("later content: %s", s.HTML())
	}
	if s.Find("span.page").Text() != "page1" {
		t.Fatalf("page 1: %s", s.HTML())
	}
	page.Set(2)
	s.Flush()
	if s.Find("span.page").Text() != "page2" || s.Find("span.page2-extra").Text() != "page2" {
		t.Fatalf("a provider re-rendered with new children: %s", s.HTML())
	}
}

// Removing a provider's subtree releases what its consumers subscribed to.
func TestContextUnmountReleasesSubscriptions(t *testing.T) {
	val := core.NewSignal("dark")
	on := core.NewSignal(true)
	s := gooweetest.Render(t, Div(Show(on, func() core.Node {
		return theme.Provide(val, badge("a"), Div(badge("b")))
	})))
	if n := val.SubscriberCount(); n != 2 {
		t.Fatalf("subscribers while mounted: %d, want 2", n)
	}
	on.Set(false)
	s.Flush()
	if n := val.SubscriberCount(); n != 0 {
		t.Fatalf("subscribers after the provider was removed: %d", n)
	}
}

// A panic inside a provider's content must not leave its value in scope for
// what renders after the error boundary.
func TestContextRestoredAfterPanic(t *testing.T) {
	s := gooweetest.Render(t, Div(
		ErrorBoundary(func(any) core.Node { return badge("fallback") },
			theme.Provide(core.NewSignal("dark"), core.Component("Boom", func() core.Node { panic("boom") })),
		),
		badge("after"),
	))
	if s.Find("span.after").Text() != "light" || s.Find("span.fallback").Text() != "light" {
		t.Fatalf("context leaked past a panic: %s", s.HTML())
	}
}

// #121: Title follows a TitleS signal; with two titles mounted, the first in
// the tree is the document's, as in the browser.
func TestTitleS(t *testing.T) {
	unread := core.NewSignal(0)
	title := core.Computed([]core.SignalAccessor{unread}, func() string {
		if unread.Get() == 0 {
			return "dure"
		}
		return fmt.Sprintf("(%d) dure", unread.Get())
	})
	s := gooweetest.Render(t, Div(
		Metadata(TitleS(title)),
		Metadata(Title("ignored: a later title")),
	))
	if s.Title() != "dure" {
		t.Fatalf("title %q", s.Title())
	}
	unread.Set(3)
	s.Flush()
	if s.Title() != "(3) dure" {
		t.Fatalf("title after update %q", s.Title())
	}
}
