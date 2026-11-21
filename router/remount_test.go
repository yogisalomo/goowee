package router_test

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
)

// #124: two routes whose functions return a component of the same name: a
// route change shows the new route's component, not the old instance.
func TestRouteChangeRemountsSameNamedComponent(t *testing.T) {
	r := router.New("/r/2/tasks")
	tab := func(name string) core.Node {
		return core.Component("RoomTab", func() core.Node { return P(ID("tab"), Text(name)) })
	}
	s := gooweetest.Render(t, Div(r.Route(map[string]func() core.Node{
		"/r/:id/tasks":     func() core.Node { return tab("tasks") },
		"/r/:id/questions": func() core.Node { return tab("questions") },
		"/about":           func() core.Node { return tab("about") },
		"/contact":         func() core.Node { return tab("contact") },
	})))
	for _, step := range []struct{ path, want string }{
		{"/r/2/questions", "questions"}, // param routes
		{"/about", "about"},             // to an exact route
		{"/contact", "contact"},         // exact to exact
		{"/r/2/tasks", "tasks"},
	} {
		r.Navigate(step.path)
		s.Flush()
		if got := s.Find("#tab").Text(); got != step.want {
			t.Fatalf("after Navigate(%q) the page shows %q, want %q", step.path, got, step.want)
		}
	}
}

// The same pattern with different params keeps the instance: that is what
// ParamSignal is for.
func TestSameRouteNewParamsKeepsTheInstance(t *testing.T) {
	r := router.New("/users/1")
	mounts := 0
	s := gooweetest.Render(t, Div(r.Route(map[string]func() core.Node{
		"/users/:id": func() core.Node {
			return core.Component("User", func() core.Node {
				mounts++
				return P(ID("user"), TextS(r.ParamSignal("id")))
			})
		},
	})))
	r.Navigate("/users/2")
	s.Flush()
	if mounts != 1 || s.Find("#user").Text() != "2" {
		t.Fatalf("mounts = %d, text %q; want 1 mount showing 2", mounts, s.Find("#user").Text())
	}
}

// SubRoute: the same rule.
func TestSubRouteChangeRemountsSameNamedComponent(t *testing.T) {
	r := router.New("/admin/users")
	page := func(name string) core.Node {
		return core.Component("AdminPage", func() core.Node { return P(ID("page"), Text(name)) })
	}
	s := gooweetest.Render(t, Div(r.SubRoute("/admin", map[string]func() core.Node{
		"/users":    func() core.Node { return page("users") },
		"/settings": func() core.Node { return page("settings") },
	})))
	r.Navigate("/admin/settings")
	s.Flush()
	if got := s.Find("#page").Text(); got != "settings" {
		t.Fatalf("sub-route shows %q, want settings", got)
	}
}
