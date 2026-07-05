package router

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"sort"
	"strings"
)

type Router struct {
	Path   *core.Signal[string]
	params *core.Signal[map[string]string]
	navFn  func(string)
}

func New(initial string) *Router {
	return &Router{
		Path:   core.NewSignal(initial),
		params: core.NewSignal(map[string]string{}).WithEquals(sameParams),
	}
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

// Param returns the value of a URL param captured by the currently matched
// route ("" if absent). This is a snapshot — good for event handlers and
// one-shot reads. To update a still-mounted component when only the param
// changes (e.g. /todos/1 -> /todos/2, where the same component is preserved),
// bind reactively with ParamSignal instead.
func (r *Router) Param(name string) string {
	return r.params.Get()[name]
}

// Params is the reactive signal of the current route's params.
func (r *Router) Params() *core.Signal[map[string]string] {
	return r.params
}

// ParamSignal returns a derived signal of one param's value that updates as the
// route's params change. Bind it reactively, e.g. h.TextS(r.ParamSignal("id"))
// or h.Textf("Todo %s", r.ParamSignal("id")).
func (r *Router) ParamSignal(name string) *core.Signal[string] {
	return core.Computed([]core.SignalAccessor{r.params}, func() string {
		return r.params.Get()[name]
	})
}

func (r *Router) setParams(p map[string]string) {
	if p == nil {
		p = map[string]string{}
	}
	r.params.Set(p)
}

// Route renders the handler whose pattern matches the current path. Patterns:
//
//	"/counter"        exact
//	"/todos/:id"      param — captures id, read via Param/ParamSignal
//	"/docs/*"         prefix wildcard — matches /docs and anything under it
//
// Exact paths win. Among the rest the most specific wins (more literal
// segments first, then non-wildcard over wildcard), so matching is
// deterministic regardless of Go's randomized map iteration. "/404" is the
// fallback if present.
func (r *Router) Route(routes map[string]func() core.Node) *core.ScopeNode {
	// Compile non-exact patterns (params or prefix wildcards) once, ordered
	// most-specific-first.
	var matchers []routeMatcher
	for pattern, fn := range routes {
		if strings.Contains(pattern, ":") || strings.HasSuffix(pattern, "/*") {
			matchers = append(matchers, compileMatcher(pattern, fn))
		}
	}
	sort.Slice(matchers, func(i, j int) bool {
		if matchers[i].literals != matchers[j].literals {
			return matchers[i].literals > matchers[j].literals
		}
		if matchers[i].wildcard != matchers[j].wildcard {
			return !matchers[i].wildcard // non-wildcard is more specific
		}
		return len(matchers[i].segs) > len(matchers[j].segs)
	})

	return &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			path := r.Path.Get()
			if fn, ok := routes[path]; ok {
				r.setParams(nil)
				return fn()
			}
			pathSegs := segsOf(path)
			for _, m := range matchers {
				if params, ok := m.match(pathSegs); ok {
					r.setParams(params)
					return m.fn()
				}
			}
			r.setParams(nil)
			if fn, ok := routes["/404"]; ok {
				return fn()
			}
			return h.P(h.Text("404 — page not found"))
		},
	}
}

type routeMatcher struct {
	segs     []string // pattern segments; a trailing "*" matches the rest
	wildcard bool     // last segment is "*"
	literals int      // count of fixed (non-param, non-wildcard) segments
	fn       func() core.Node
}

func compileMatcher(pattern string, fn func() core.Node) routeMatcher {
	m := routeMatcher{segs: segsOf(pattern), fn: fn}
	for _, s := range m.segs {
		switch {
		case s == "*":
			m.wildcard = true
		case strings.HasPrefix(s, ":"):
			// param segment
		default:
			m.literals++
		}
	}
	return m
}

// match returns captured params if pathSegs matches, else (nil, false).
func (m routeMatcher) match(pathSegs []string) (map[string]string, bool) {
	var params map[string]string
	for i, seg := range m.segs {
		if seg == "*" {
			return params, true // trailing wildcard consumes the rest
		}
		if i >= len(pathSegs) {
			return nil, false
		}
		if strings.HasPrefix(seg, ":") {
			if params == nil {
				params = map[string]string{}
			}
			params[seg[1:]] = pathSegs[i]
			continue
		}
		if seg != pathSegs[i] {
			return nil, false
		}
	}
	if len(m.segs) != len(pathSegs) {
		return nil, false
	}
	return params, true
}

func segsOf(path string) []string {
	var out []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func sameParams(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
