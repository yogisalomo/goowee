package router

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
)

func TestNavLinkActiveFollowsPath(t *testing.T) {
	r := New("/docs")
	r.SetNavFn(func(string) {}) // no browser in a headless test
	s := gooweetest.Render(t, core.Component("Nav", func() core.Node {
		return Div(
			r.NavLink("/", "home"),
			r.NavLink("/docs", "docs"),
			r.NavLink("/docs/install?tab=a#top", "install"),
		)
	}))

	active := func(label string) bool {
		return s.FindByText(label).Attr("class") == "active"
	}
	current := func(label string) string {
		return s.FindByText(label).Attr("aria-current")
	}

	if !active("docs") || current("docs") != "page" {
		t.Fatalf("/docs: docs class=%q aria-current=%q", s.FindByText("docs").Attr("class"), current("docs"))
	}
	if active("home") || current("home") != "false" {
		t.Fatalf("/docs: home should be inactive")
	}
	if active("install") {
		t.Fatalf("/docs: a link to a child route should not be active on the parent")
	}

	r.Navigate("/docs/install")
	s.Flush()
	if !active("docs") || !active("install") {
		t.Fatalf("/docs/install: docs=%q install=%q", s.FindByText("docs").Attr("class"), s.FindByText("install").Attr("class"))
	}

	r.Navigate("/docsx")
	s.Flush()
	if active("docs") || active("install") {
		t.Fatalf("/docsx must not segment-match /docs: docs=%q install=%q", s.FindByText("docs").Attr("class"), s.FindByText("install").Attr("class"))
	}
	if r.linkPathSig == nil {
		t.Fatal("NavLink did not create the shared path computed")
	}
}

func TestBeforeLeaveCancelsNavigation(t *testing.T) {
	r := New("/a")
	var urls []string
	r.SetNavFn(func(url string) { urls = append(urls, url) })
	blocked := true

	s := gooweetest.Render(t, core.Component("Editor", func() core.Node {
		r.BeforeLeave(func(to string) bool {
			if blocked {
				return false
			}
			return true
		})
		return Div(
			r.Link("/blocked", "blocked"),
			r.Link("/ok", "ok"),
			P(ID("path"), TextS(r.Path)),
		)
	}))

	s.Click(s.FindByText("blocked"))
	if got := r.Path.Peek(); got != "/a" {
		t.Fatalf("blocked navigation changed the path to %q", got)
	}
	if len(urls) != 0 {
		t.Fatalf("blocked navigation reached history: %v", urls)
	}

	blocked = false
	s.Click(s.FindByText("ok"))
	if got := r.Path.Peek(); got != "/ok" {
		t.Fatalf("allowed navigation left the path at %q", got)
	}
	if len(urls) != 1 || urls[0] != "/ok" {
		t.Fatalf("history calls after allowed navigation: %v", urls)
	}
	if got := s.Find("#path").Text(); got != "/ok" {
		t.Fatalf("path text = %q", got)
	}
}

func TestChangedNavScope(t *testing.T) {
	cases := []struct {
		name                           string
		prevPath, prevFrag, path, frag string
		want                           navScope
	}{
		{"query only", "/docs", "", "/docs", "", navScopeNone},
		{"fragment only", "/docs", "", "/docs", "top", navScopeFragment},
		{"fragment changed", "/docs", "a", "/docs", "b", navScopeFragment},
		{"fragment removed", "/docs", "a", "/docs", "", navScopeFragment},
		{"path only", "/docs", "", "/docs/install", "", navScopeRoute},
		{"path and query", "/docs", "", "/docs/install", "", navScopeRoute},
		{"path and fragment", "/docs", "a", "/docs/install", "b", navScopeRoute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := changedNavScope(tc.prevPath, tc.prevFrag, tc.path, tc.frag); got != tc.want {
				t.Fatalf("changedNavScope() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNavigationOptions(t *testing.T) {
	r := New("/")
	want := ScrollOptions{
		Disabled:         true,
		Container:        "#app-scroll",
		FocusDisabled:    true,
		AnnounceDisabled: true,
	}
	r.SetNavigationOptions(want)
	if got := r.NavigationOptions(); got != want {
		t.Fatalf("NavigationOptions() = %+v, want %+v", got, want)
	}
}
