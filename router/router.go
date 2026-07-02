package router

import (
	"goowee/core"
	"strings"
)

type Router struct {
	Path  *core.Signal[string]
	navFn func(string)
}

func New(initial string) *Router {
	return &Router{Path: core.NewSignal(initial)}
}

func (r *Router) SetNavFn(fn func(string)) {
	r.navFn = fn
}

func (r *Router) Navigate(path string) {
	r.Path.Set(path)
	if r.navFn != nil {
		r.navFn(path)
	}
}

func (r *Router) Link(to, text string) *core.ElementNode {
	return &core.ElementNode{
		Tag: "a",
		Props: map[string]any{
			"href": to,
			"onclick": func(ed core.EventData) {
				r.Navigate(to)
			},
		},
		Children: []core.Node{&core.TextNode{Value: text}},
	}
}

func (r *Router) Route(routes map[string]func() core.Node) *core.ScopeNode {
	return &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			path := r.Path.Get()
			if fn, ok := routes[path]; ok {
				return fn()
			}
			for pattern, fn := range routes {
				if matchPrefix(pattern, path) {
					return fn()
				}
			}
			if fn, ok := routes["/404"]; ok {
				return fn()
			}
			return &core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": "404 — page not found"},
			}
		},
	}
}

func matchPrefix(pattern, path string) bool {
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(path, prefix)
	}
	return pattern == path
}
