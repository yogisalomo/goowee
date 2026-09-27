package router

import (
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
)

type Router struct {
	Path      *core.Signal[string]
	Query     *core.Signal[map[string]string]
	params    *core.Signal[map[string]string]
	navFn     func(string) // Navigate — calls pushState
	replaceFn func(string) // NavigateReplace — calls replaceState
	backFn    func()       // Back
	forwardFn func()       // Forward
	// base is the URL prefix the app is served under (e.g. "/goowee" on a
	// GitHub Pages project site), "" at the domain root. Internal paths
	// (Path, route patterns) are always base-relative; the base is added back
	// for the browser URL (Link hrefs, pushState) and stripped from it
	// (CurrentPath, popstate). Set by BindHistory from the page's <base>.
	base string

	// notFound is set by the last Route/SubRoute render that matched no
	// pattern (see NotFound).
	notFound bool
}

func New(initial string) *Router {
	return &Router{
		Path:   core.NewSignal(initial),
		Query:  core.NewSignal(map[string]string{}).WithEquals(sameParams),
		params: core.NewSignal(map[string]string{}).WithEquals(sameParams),
	}
}

// NewURL creates a router positioned at u's path and query — what a server
// renders for a request: router.NewURL(req.URL).
func NewURL(u *url.URL) *Router {
	r := New(u.Path)
	r.Query.Set(parseQuery(u.RawQuery))
	return r
}

// NotFound reports whether the most recent Route render matched no pattern
// and fell through to the "/404" route (or the built-in not-found text), or a
// SubRoute under a matched prefix matched nothing. Servers use it after
// rendering to answer with HTTP 404 (see ssr.RoutedPage).
func (r *Router) NotFound() bool { return r.notFound }

// SetNavFn sets the browser-history callback for Navigate. Kept for backward
// compatibility; prefer BindHistory which sets all callbacks at once.
func (r *Router) SetNavFn(fn func(string)) {
	r.navFn = fn
}

func (r *Router) Navigate(path string) {
	r.Path.Set(path)
	if r.navFn != nil {
		r.navFn(r.base + path) // route matching is base-relative; the URL carries the base
	}
}

// NavigateReplace updates the path without pushing a new history entry
// (calls replaceState instead of pushState). Use it for non-destructive
// transitions — tab switches, search params, wizard steps where back
// should skip the intermediate step.
func (r *Router) NavigateReplace(path string) {
	r.Path.Set(path)
	if r.replaceFn != nil {
		r.replaceFn(r.base + path)
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

// Link renders an <a> that navigates in-app on a plain click. A click with a
// modifier key (cmd/ctrl/shift/alt) or a non-primary button is left to the
// browser, so "open in new tab/window" keeps working. items add attributes
// (Class, AriaCurrent, …) or children after the text.
func (r *Router) Link(to, text string, items ...core.Item) *core.ElementNode {
	return r.LinkTo(to, append([]core.Item{h.Text(text)}, items...)...)
}

// LinkTo is Link with arbitrary content: items are the anchor's attributes and
// children (an icon plus text, several spans, …). The href carries the base
// path, so copying the link or opening it in a new tab lands on the right URL.
//
//	r.LinkTo("/", h.Class("brand"), logo(), h.Text("goowee"))
func (r *Router) LinkTo(to string, items ...core.Item) *core.ElementNode {
	all := []core.Item{
		h.Href(r.base + to), // real URL carries the base; internal nav is base-relative
		h.OnClickE(func(e core.EventData) {
			if !InAppClick(e) {
				return
			}
			e.PreventDefault()
			r.Navigate(to)
		}),
	}
	return h.A(append(all, items...)...)
}

// InAppClick reports whether a click on a link should be handled as in-app
// navigation: the primary button, with no modifier key. Other clicks mean
// "open in a new tab/window" (cmd/ctrl/middle-click) or "download" (alt) and
// belong to the browser. Use it in custom link handlers.
func InAppClick(e core.EventData) bool {
	return e.Button() == 0 && !e.CtrlKey() && !e.MetaKey() && !e.ShiftKey() && !e.AltKey()
}

// BasePath returns the URL prefix the app is served under ("" at the root).
func (r *Router) BasePath() string { return r.base }

// SetBasePath sets the URL prefix the app is served under ("/goowee", or "" at
// the domain root; a trailing slash is ignored). In the browser BindHistory
// reads it from the page's <base href>; a server rendering under a prefix, or
// a test, sets it here so links carry it.
func (r *Router) SetBasePath(base string) { r.base = strings.TrimSuffix(base, "/") }

// Param returns the value of a URL param captured by the currently matched
// route ("" if absent). This is a snapshot — good for event handlers and
// one-shot reads. To update a still-mounted component when only the param
// changes (e.g. /todos/1 -> /todos/2, where the same component is preserved),
// bind reactively with ParamSignal instead.
func (r *Router) Param(name string) string {
	return r.params.Peek()[name]
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

// QueryParam returns the value of a URL query parameter ("" if absent).
// This is a snapshot — good for event handlers and one-shot reads.
func (r *Router) QueryParam(name string) string {
	return r.Query.Peek()[name]
}

// QueryParamSignal returns a derived signal of one query parameter's value that
// updates when the URL changes. Bind it reactively, e.g. h.TextS(r.QueryParamSignal("tag")).
func (r *Router) QueryParamSignal(name string) *core.Signal[string] {
	return core.Computed([]core.SignalAccessor{r.Query}, func() string {
		return r.Query.Get()[name]
	})
}

// SetQueryParam updates a query parameter without navigating (replaceState).
func (r *Router) SetQueryParam(name, value string) {
	q := copyParams(r.Query.Get())
	if value == "" {
		delete(q, name)
	} else {
		q[name] = value
	}
	r.Query.Set(q)
	if r.replaceFn != nil {
		r.replaceFn(r.base + r.Path.Get() + "?" + encodeQuery(q))
	}
}

// SetQueryParamPush is like SetQueryParam but pushes a history entry.
func (r *Router) SetQueryParamPush(name, value string) {
	q := copyParams(r.Query.Get())
	if value == "" {
		delete(q, name)
	} else {
		q[name] = value
	}
	r.Query.Set(q)
	if r.navFn != nil {
		r.navFn(r.base + r.Path.Get() + "?" + encodeQuery(q))
	}
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
	matchers := compileRoutes(routes)
	return &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			path := r.Path.Get()
			r.notFound = false
			if fn, ok := routes[path]; ok {
				r.setParams(nil)
				return fn()
			}
			if fn, params, ok := matchCompiled(matchers, path); ok {
				r.setParams(params)
				return fn()
			}
			r.setParams(nil)
			r.notFound = true
			if fn, ok := routes["/404"]; ok {
				return fn()
			}
			return h.P(h.Text("404 — page not found"))
		},
	}
}

// SubRoute creates a nested route group under prefix. When the current path
// starts with prefix, the prefix is stripped and routing continues against
// the sub-routes, with the same deterministic most-specific-first matching as
// Route. Returns nothing (empty FragmentNode) when the prefix doesn't match, so
// it can be placed inside a parent route handler that provides the surrounding
// layout.
//
// Params captured by a sub-route are merged into the parent route's params, so
// under "/org/:org/*" a sub-route "/:repo" exposes both Param("org") and
// Param("repo").
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
	matchers := compileRoutes(routes)
	// Keys this sub-route contributed on its last render, so a later render
	// can drop them before merging its own (no stale sub-params).
	var lastKeys []string
	merge := func(sub map[string]string) {
		if len(sub) == 0 && len(lastKeys) == 0 {
			return
		}
		merged := copyParams(r.params.Peek())
		for _, k := range lastKeys {
			delete(merged, k)
		}
		lastKeys = lastKeys[:0]
		for k, v := range sub {
			merged[k] = v
			lastKeys = append(lastKeys, k)
		}
		r.setParams(merged)
	}
	// A scope that re-matches the sub-routes whenever the path changes. It runs
	// on the render loop, so no locking is needed (ADR-010).
	return &core.ScopeNode{
		Deps: []core.SignalAccessor{r.Path},
		Render: func() core.Node {
			path := r.Path.Get()
			var sub string
			switch {
			case path == prefix:
				sub = "/"
			case strings.HasPrefix(path, prefix+"/"):
				sub = path[len(prefix):]
			default:
				merge(nil)
				return &core.FragmentNode{}
			}
			if fn, ok := routes[sub]; ok {
				merge(nil)
				return fn()
			}
			if fn, params, ok := matchCompiled(matchers, sub); ok {
				merge(params)
				return fn()
			}
			merge(nil)
			r.notFound = true
			if fn, ok := routes["/404"]; ok {
				return fn()
			}
			return &core.FragmentNode{}
		},
	}
}

// compileRoutes compiles the non-exact patterns (params or prefix wildcards)
// once, ordered most-specific-first: more literal segments first, then
// non-wildcard over wildcard, then longer patterns, then the pattern text as a
// final tie-break — so matching never depends on Go's randomized map order.
func compileRoutes(routes map[string]func() core.Node) []routeMatcher {
	var matchers []routeMatcher
	for pattern, fn := range routes {
		if strings.Contains(pattern, ":") || strings.HasSuffix(pattern, "/*") {
			matchers = append(matchers, compileMatcher(pattern, fn))
		}
	}
	sort.Slice(matchers, func(i, j int) bool {
		a, b := matchers[i], matchers[j]
		if a.literals != b.literals {
			return a.literals > b.literals
		}
		if a.wildcard != b.wildcard {
			return !a.wildcard // non-wildcard is more specific
		}
		if len(a.segs) != len(b.segs) {
			return len(a.segs) > len(b.segs)
		}
		return a.pattern < b.pattern
	})
	return matchers
}

// matchCompiled returns the handler and params of the first (most specific)
// matcher that accepts path.
func matchCompiled(matchers []routeMatcher, path string) (func() core.Node, map[string]string, bool) {
	pathSegs := segsOf(path)
	for _, m := range matchers {
		if params, ok := m.match(pathSegs); ok {
			return m.fn, params, true
		}
	}
	return nil, nil, false
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

// Lazy defers a route handler's initialization: load runs once, the first time
// the route is matched, and its result is cached for later matches. Everything
// ships in the one WASM binary, so this defers setup work (building the handler
// closure, one-time state), not code loading — there is no dynamic import.
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
	pattern  string
	segs     []string // pattern segments; a trailing "*" matches the rest
	wildcard bool     // last segment is "*"
	literals int      // count of fixed (non-param, non-wildcard) segments
	fn       func() core.Node
}

func compileMatcher(pattern string, fn func() core.Node) routeMatcher {
	m := routeMatcher{pattern: pattern, segs: segsOf(pattern), fn: fn}
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

// copyParams returns a shallow copy of the map (or a fresh map if nil).
func copyParams(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// parseQuery parses a query string like "?a=1&b=2" into a map.
func parseQuery(raw string) map[string]string {
	q := map[string]string{}
	if raw == "" {
		return q
	}
	raw = strings.TrimPrefix(raw, "?")
	if raw == "" {
		return q
	}
	for _, pair := range strings.Split(raw, "&") {
		k, v, _ := strings.Cut(pair, "=")
		if k != "" {
			q[unescapeQuery(k)] = unescapeQuery(v)
		}
	}
	return q
}

// encodeQuery encodes a map into a query string like "a=1&b=2".
func encodeQuery(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var buf strings.Builder
	first := true
	for k, v := range m {
		if !first {
			buf.WriteByte('&')
		}
		first = false
		buf.WriteString(escapeQuery(k))
		buf.WriteByte('=')
		buf.WriteString(escapeQuery(v))
	}
	return buf.String()
}

func escapeQuery(s string) string { return url.QueryEscape(s) }

// unescapeQuery decodes a query component (%XX escapes and '+' as space); a
// malformed escape falls back to the raw text.
func unescapeQuery(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}
