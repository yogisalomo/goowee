package router

import (
	"github.com/yogisalomo/goowee/core"
	"testing"
)

func TestNewRouter(t *testing.T) {
	r := New("/")
	if r.Path.Get() != "/" {
		t.Fatalf("expected /, got %s", r.Path.Get())
	}
}

func TestNavigate(t *testing.T) {
	r := New("/")
	r.Navigate("/counter")
	if r.Path.Get() != "/counter" {
		t.Fatalf("expected /counter, got %s", r.Path.Get())
	}
}

func TestNavigateCallsNavFn(t *testing.T) {
	r := New("/")
	var called string
	r.SetNavFn(func(path string) {
		called = path
	})
	r.Navigate("/about")
	if called != "/about" {
		t.Fatalf("expected /about, got %s", called)
	}
}

func TestLink(t *testing.T) {
	r := New("/")
	link := r.Link("/counter", "Counter")
	if link.Tag != "a" {
		t.Fatalf("expected a, got %s", link.Tag)
	}
	if len(link.Attrs) == 0 || link.Attrs[0].Value != "/counter" {
		t.Fatalf("expected href /counter, got %v", link.Attrs)
	}
}

func TestRouteExactMatch(t *testing.T) {
	r := New("/counter")
	var rendered string
	scope := r.Route(map[string]func() core.Node{
		"/": func() core.Node {
			rendered = "home"
			return &core.ElementNode{Tag: "h1"}
		},
		"/counter": func() core.Node {
			rendered = "counter"
			return &core.ElementNode{Tag: "div"}
		},
	})
	scope.Render()
	if rendered != "counter" {
		t.Fatalf("expected counter, got %s", rendered)
	}
}

func TestRouteNoMatchReturns404(t *testing.T) {
	r := New("/unknown")
	scope := r.Route(map[string]func() core.Node{
		"/": func() core.Node {
			return &core.ElementNode{Tag: "h1"}
		},
	})
	result := scope.Render()
	if result == nil {
		t.Fatal("expected non-nil 404 node")
	}
	if el, ok := result.(*core.ElementNode); !ok || el.Tag == "h1" {
		t.Fatalf("expected 404 fallback, got %T", result)
	}
}

func TestRouterIntegration(t *testing.T) {
	r := New("/")
	r.Navigate("/about")
	if r.Path.Get() != "/about" {
		t.Fatalf("expected /about, got %s", r.Path.Get())
	}
}

func TestLinkHasPreventDefault(t *testing.T) {
	r := New("/")
	link := r.Link("/counter", "Counter")
	if len(link.Handlers) == 0 {
		t.Fatal("expected handler on link")
	}
	if !link.Handlers[0].Options.PreventDefault {
		t.Fatal("expected PreventDefault on link handler")
	}
}

func TestRouteDeterministicPrefixMatch(t *testing.T) {
	newRoutes := func(hits map[string]int) map[string]func() core.Node {
		return map[string]func() core.Node{
			"/docs/*":     func() core.Node { hits["docs"]++; return &core.ElementNode{Tag: "div"} },
			"/docs/api/*": func() core.Node { hits["api"]++; return &core.ElementNode{Tag: "section"} },
			"/*":          func() core.Node { hits["root"]++; return &core.ElementNode{Tag: "main"} },
		}
	}

	cases := map[string]string{
		"/docs/api/v1": "api",  // most specific prefix wins
		"/docs/readme": "docs", // less specific
		"/other":       "root", // catch-all
	}

	// Build the router fresh many times so different map-iteration orders all
	// resolve the same way (the old code iterated the map directly → random).
	for path, want := range cases {
		for i := 0; i < 50; i++ {
			hits := map[string]int{}
			r := New(path)
			r.Route(newRoutes(hits)).Render()
			if hits[want] != 1 {
				t.Fatalf("path %s: expected %q to match once, got %v", path, want, hits)
			}
			total := hits["docs"] + hits["api"] + hits["root"]
			if total != 1 {
				t.Fatalf("path %s: expected exactly one match, got %v", path, hits)
			}
		}
	}
}

func TestRouteExactBeatsPrefix(t *testing.T) {
	got := ""
	r := New("/docs")
	r.Route(map[string]func() core.Node{
		"/docs":   func() core.Node { got = "exact"; return &core.ElementNode{Tag: "div"} },
		"/docs/*": func() core.Node { got = "prefix"; return &core.ElementNode{Tag: "section"} },
	}).Render()
	if got != "exact" {
		t.Fatalf("expected exact match to win, got %q", got)
	}
}

func TestRouteParamMatch(t *testing.T) {
	got := ""
	r := New("/users/42/posts/7")
	r.Route(map[string]func() core.Node{
		"/users/:uid/posts/:pid": func() core.Node {
			got = "match"
			return &core.ElementNode{Tag: "div"}
		},
	}).Render()
	if got != "match" {
		t.Fatal("expected param route to match")
	}
	if r.Param("uid") != "42" || r.Param("pid") != "7" {
		t.Fatalf("captured params uid=%q pid=%q", r.Param("uid"), r.Param("pid"))
	}
}

func TestRouteExactBeatsParam(t *testing.T) {
	got := ""
	r := New("/todos/new")
	r.Route(map[string]func() core.Node{
		"/todos/new": func() core.Node { got = "exact"; return &core.ElementNode{Tag: "div"} },
		"/todos/:id": func() core.Node { got = "param"; return &core.ElementNode{Tag: "span"} },
	}).Render()
	if got != "exact" {
		t.Fatalf("expected exact /todos/new to win, got %q", got)
	}
}

func TestRouteParamMoreLiteralsWin(t *testing.T) {
	got := ""
	r := New("/x/y")
	r.Route(map[string]func() core.Node{
		"/:a/:b": func() core.Node { got = "generic"; return &core.ElementNode{Tag: "div"} },
		"/x/:b":  func() core.Node { got = "specific"; return &core.ElementNode{Tag: "span"} },
	}).Render()
	if got != "specific" {
		t.Fatalf("more-literal route should win, got %q", got)
	}
}

func TestRouteParamSegmentCountMustMatch(t *testing.T) {
	got := "none"
	r := New("/users/1/extra")
	r.Route(map[string]func() core.Node{
		"/users/:id": func() core.Node { got = "match"; return &core.ElementNode{Tag: "div"} },
	}).Render()
	if got != "none" {
		t.Fatalf("param route should not match a different segment count, got %q", got)
	}
}

func TestNavigateReplace(t *testing.T) {
	r := New("/")
	r.NavigateReplace("/settings")
	if r.Path.Get() != "/settings" {
		t.Fatalf("expected /settings, got %s", r.Path.Get())
	}
}

func TestNavigateReplaceCallsReplaceFn(t *testing.T) {
	r := New("/")
	var called string
	r.replaceFn = func(path string) { called = path }
	r.NavigateReplace("/replace")
	if called != "/replace" {
		t.Fatalf("expected /replace, got %s", called)
	}
}

func TestNavigateReplaceDoesNotCallNavFn(t *testing.T) {
	r := New("/")
	var navCalled bool
	r.navFn = func(path string) { navCalled = true }
	r.replaceFn = func(path string) {}
	r.NavigateReplace("/replace")
	if navCalled {
		t.Fatal("NavigateReplace should not call navFn (pushState)")
	}
}

func TestBackCallsBackFn(t *testing.T) {
	r := New("/")
	var called bool
	r.backFn = func() { called = true }
	r.Back()
	if !called {
		t.Fatal("expected backFn to be called")
	}
}

func TestForwardCallsForwardFn(t *testing.T) {
	r := New("/")
	var called bool
	r.forwardFn = func() { called = true }
	r.Forward()
	if !called {
		t.Fatal("expected forwardFn to be called")
	}
}

func TestSubRouteExactPrefix(t *testing.T) {
	r := New("/dashboard")
	sub := r.SubRoute("/dashboard", map[string]func() core.Node{
		"/": func() core.Node { return &core.ElementNode{Tag: "div"} },
	})
	scope, ok := sub.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", sub)
	}
	result := scope.Render()
	if _, ok := result.(*core.ElementNode); !ok {
		t.Fatal("expected SubRoute to render when path matches prefix")
	}
}

func TestSubRouteNestedPath(t *testing.T) {
	r := New("/dashboard/settings")
	var rendered string
	sub := r.SubRoute("/dashboard", map[string]func() core.Node{
		"/":         func() core.Node { rendered = "home"; return &core.ElementNode{Tag: "div"} },
		"/settings": func() core.Node { rendered = "settings"; return &core.ElementNode{Tag: "section"} },
	})
	scope, ok := sub.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", sub)
	}
	scope.Render()
	if rendered != "settings" {
		t.Fatalf("expected 'settings', got %q", rendered)
	}
}

func TestSubRouteNoMatchReturnsEmpty(t *testing.T) {
	r := New("/other")
	sub := r.SubRoute("/dashboard", map[string]func() core.Node{
		"/": func() core.Node { return &core.ElementNode{Tag: "div"} },
	})
	scope, ok := sub.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", sub)
	}
	result := scope.Render()
	if _, ok := result.(*core.FragmentNode); !ok {
		t.Fatalf("expected empty FragmentNode when prefix doesn't match, got %T", result)
	}
}

func TestSubRouteUpdatesOnPathChange(t *testing.T) {
	r := New("/dashboard")
	var rendered string
	sub := r.SubRoute("/dashboard", map[string]func() core.Node{
		"/":         func() core.Node { rendered = "home"; return &core.ElementNode{Tag: "div"} },
		"/settings": func() core.Node { rendered = "settings"; return &core.ElementNode{Tag: "section"} },
	})
	scope, ok := sub.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", sub)
	}
	scope.Render()
	if rendered != "home" {
		t.Fatalf("expected 'home', got %q", rendered)
	}

	r.Path.Set("/dashboard/settings")
	scope.Render()
	if rendered != "settings" {
		t.Fatalf("expected 'settings' after path change, got %q", rendered)
	}
}

func TestGuardPassesWhenTrue(t *testing.T) {
	var ran bool
	guarded := Guard(func() bool { return true }, func() core.Node {
		return &core.ElementNode{Tag: "p"}
	}, func() core.Node {
		ran = true
		return &core.ElementNode{Tag: "div"}
	})
	guarded()
	if !ran {
		t.Fatal("expected guarded route to run when check passes")
	}
}

func TestGuardFallsBackWhenFalse(t *testing.T) {
	var ran, fellBack bool
	guarded := Guard(func() bool { return false }, func() core.Node {
		fellBack = true
		return &core.ElementNode{Tag: "p"}
	}, func() core.Node {
		ran = true
		return &core.ElementNode{Tag: "div"}
	})
	guarded()
	if ran {
		t.Fatal("expected guarded route NOT to run when check fails")
	}
	if !fellBack {
		t.Fatal("expected fallback to render when check fails")
	}
}

func TestGuardWithRoute(t *testing.T) {
	// Integration: Guard wraps a route handler.
	r := New("/admin")
	authenticated := true
	var rendered string
	r.Route(map[string]func() core.Node{
		"/": func() core.Node { return &core.ElementNode{Tag: "div"} },
		"/admin": Guard(func() bool { return authenticated },
			func() core.Node { rendered = "login"; return &core.ElementNode{Tag: "div"} },
			func() core.Node { rendered = "admin"; return &core.ElementNode{Tag: "div"} },
		),
	}).Render()
	if rendered != "admin" {
		t.Fatalf("expected 'admin', got %q", rendered)
	}
}

func TestLazyLoadsOnce(t *testing.T) {
	loadCount := 0
	lazy := Lazy(func() func() core.Node {
		loadCount++
		return func() core.Node { return &core.ElementNode{Tag: "div"} }
	})

	lazy()
	if loadCount != 1 {
		t.Fatalf("expected loadCount=1 after first call, got %d", loadCount)
	}

	lazy()
	if loadCount != 1 {
		t.Fatalf("expected loadCount=1 after second call (cached), got %d", loadCount)
	}
}

func TestLazyWithRoute(t *testing.T) {
	var loaded bool
	r := New("/heavy")
	r.Route(map[string]func() core.Node{
		"/": func() core.Node { return &core.ElementNode{Tag: "div"} },
		"/heavy": Lazy(func() func() core.Node {
			loaded = true
			return func() core.Node { return &core.ElementNode{Tag: "section"} }
		}),
	}).Render()
	if !loaded {
		t.Fatal("expected Lazy route to load when matched")
	}
}

func TestParamSignalReactive(t *testing.T) {
	r := New("/todos/1")
	scope := r.Route(map[string]func() core.Node{
		"/todos/:id": func() core.Node { return &core.ElementNode{Tag: "div"} },
	})
	scope.Render() // match /todos/1
	id := r.ParamSignal("id")
	if id.Get() != "1" || r.Param("id") != "1" {
		t.Fatalf("initial id: signal=%q param=%q", id.Get(), r.Param("id"))
	}

	// Navigate to a new param value; the derived signal must update (this is
	// what lets a preserved component re-render on param change).
	r.Path.Set("/todos/2")
	scope.Render()
	if id.Get() != "2" {
		t.Fatalf("ParamSignal did not react to param change: %q", id.Get())
	}
	if r.Param("id") != "2" {
		t.Fatalf("Param snapshot not updated: %q", r.Param("id"))
	}
}
