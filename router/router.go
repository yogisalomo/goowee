package router

import (
	"goowee/core"
	"goowee/h"
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
	return h.A(
		h.Href(to),
		h.OnClickE(func(core.EventData) { r.Navigate(to) }, h.PreventDefault()),
		h.Text(text),
	)
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
			return h.P(h.Text("404 — page not found"))
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
