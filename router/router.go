package router

import (
	"goowee/core"
	"goowee/h"
	"sort"
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

// Route renders the handler whose pattern matches the current path. Patterns
// are either exact ("/counter") or a prefix wildcard ("/docs/*"). Exact paths
// win; among prefixes the most specific (longest) wins, so matching is
// deterministic regardless of Go's randomized map iteration. "/404" is used
// as the fallback if present.
//
// URL params (e.g. "/todos/:id") are not supported yet; a route needing an id
// reads it from the path in its own handler for now.
func (r *Router) Route(routes map[string]func() core.Node) *core.ScopeNode {
	// Precompute prefix patterns ordered most-specific-first, once.
	type prefixRoute struct {
		prefix string
		fn     func() core.Node
	}
	var prefixes []prefixRoute
	for pattern, fn := range routes {
		if strings.HasSuffix(pattern, "/*") {
			prefixes = append(prefixes, prefixRoute{pattern[:len(pattern)-1], fn})
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i].prefix) > len(prefixes[j].prefix)
	})

	return &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			path := r.Path.Get()
			if fn, ok := routes[path]; ok {
				return fn()
			}
			for _, pr := range prefixes {
				if strings.HasPrefix(path, pr.prefix) {
					return pr.fn()
				}
			}
			if fn, ok := routes["/404"]; ok {
				return fn()
			}
			return h.P(h.Text("404 — page not found"))
		},
	}
}
