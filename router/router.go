package router

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"sort"
	"strings"
	"sync"
)

type Router struct {
	Path      *core.Signal[string]
	params    *core.Signal[map[string]string]
	navFn     func(string) // Navigate — calls pushState
	replaceFn func(string) // NavigateReplace — calls replaceState
	backFn    func()       // Back
	forwardFn func()      // Forward
}

func New(initial string) *Router {
	return &Router{
		Path:   core.NewSignal(initial),
		params: core.NewSignal(map[string]string{}).WithEquals(sameParams),
	}
}

// SetNavFn sets the browser-history callback for Navigate. Kept for backward
// compatibility; prefer BindHistory which sets all callbacks at once.
func (r *Router) SetNavFn(fn func(string)) {
	r.navFn = fn
}

func (r *Router) Navigate(path string) {
	r.Path.Set(path)
	if r.navFn != nil {
		r.navFn(path)
	}
}

// NavigateReplace updates the path without pushing a new history entry
// (calls replaceState instead of pushState). Use it for non-destructive
// transitions — tab switches, search params, wizard steps where back
// should skip the intermediate step.
func (r *Router) NavigateReplace(path string) {
	r.Path.Set(path)
	if r.replaceFn != nil {
		r.replaceFn(path)
	}
}

// Back navigates backward in browser history. Only works after BindHistory.
func (r *Router) Back() {
	if r.backFn != nil {
		r.backFn()
	}
}

// Forward navigates forward in browser history. Only works after BindHistory.
func (r *Router) Forward() {
	if r.forwardFn != nil {
		r.forwardFn()
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

// SubRoute creates a nested route group under prefix. When the current path
// starts with prefix, the prefix is stripped and routing continues against
// the sub-routes. Returns nothing (empty FragmentNode) when the prefix doesn't
// match, so it can be placed inside a parent route handler that provides the
// surrounding layout.
//
//	r.Route(map[string]func() core.Node{
//	    "/dashboard/*": func() core.Node {
//	        return h.Div(
//	            h.Nav(h.A(...)),
//	            r.SubRoute("/dashboard", map[string]func() core.Node{
//	                "/":         dashboardHome,
//	                "/settings": dashboardSettings,
//	            }),
//	        )
//	    },
//	})
func (r *Router) SubRoute(prefix string, routes map[string]func() core.Node) core.Node {
	var (
		mu       sync.Mutex
		children *core.ScopeNode
	)

	matchSub := func(path string) core.Node {
		if path == prefix {
			return matchRoute(r, "/", routes)
		}
		if strings.HasPrefix(path, prefix+"/") {
			suffix := path[len(prefix):]
			return matchRoute(r, suffix, routes)
		}
		return &core.FragmentNode{}
	}

	children = &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			mu.Lock()
			defer mu.Unlock()
			return matchSub(r.Path.Get())
		},
	}
	return children
}

// matchRoute is like Route's internal dispatch but without the sticky params
// and without compiling routes (SubRoute routes are already compiled).
func matchRoute(r *Router, path string, routes map[string]func() core.Node) core.Node {
	if fn, ok := routes[path]; ok {
		return fn()
	}

	pathSegs := segsOf(path)
	for pattern, fn := range routes {
		if !strings.Contains(pattern, ":") && !strings.HasSuffix(pattern, "/*") {
			continue
		}
		m := compileMatcher(pattern, fn)
		if params, ok := m.match(pathSegs); ok {
			r.setParams(params)
			return fn()
		}
	}

	if fn, ok := routes["/404"]; ok {
		return fn()
	}
	return &core.FragmentNode{}
}

// Guard wraps a route handler with an access check. If check returns true,
// the route handler runs; otherwise fallback is rendered. Use it for auth
// guards, feature flags, or any conditional rendering at the route level.
//
//	r.Route(map[string]func() core.Node{
//	    "/admin": router.Guard(isAdmin, loginPage, adminPanel),
//	})
func Guard(check func() bool, fallback, route func() core.Node) func() core.Node {
	return func() core.Node {
		if check() {
			return route()
		}
		return fallback()
	}
}

// Lazy defers route handler initialization. load is called once, the first
// time the route is matched, and its result is cached for subsequent matches.
// Use it for code splitting: Lazy(func() func() core.Node { return heavyPage })
// defers importing heavyPage until the route is actually visited.
func Lazy(load func() func() core.Node) func() core.Node {
	var (
		once sync.Once
		fn   func() core.Node
	)
	return func() core.Node {
		once.Do(func() { fn = load() })
		return fn()
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
