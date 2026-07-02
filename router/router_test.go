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
	if link.Props["href"] != "/counter" {
		t.Fatalf("expected href /counter, got %v", link.Props["href"])
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
