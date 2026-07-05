package router

import (
	"goowee/core"
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
