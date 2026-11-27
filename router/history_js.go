//go:build js && wasm

package router

import (
	"strings"
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/host"
)

// Listeners registered by BindHistory must stay reachable for the life of the
// page: syscall/js releases a Func only when Go says so.
var browserListeners []js.Func

// scrollPositions is the live position of each history entry, keyed by the
// gooweeKey we put in its state. A scroll event only updates this map;
// saveScroll copies the position into history.state when the route changes.
// Keeping it outside history.state matters on back: the browser can scroll and
// fire a scroll event before popstate, which would otherwise overwrite the
// very position we came back to restore.
var (
	scrollPositions = make(map[int]map[string]any)
	nextScrollKey   = 1
)

// basePath reads the page's <base href> and returns it as a prefix without a
// trailing slash (e.g. "/goowee"), or "" when there is no <base> (served at the
// domain root, as the SSR server and local dev do). It's how the same WASM
// binary works both at "/" and under a GitHub Pages project path.
func basePath() string {
	href := strings.TrimSuffix(host.Call("baseHref").String(), "/")
	if href != "" && !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return href
}

// CurrentPath returns the browser's current pathname, base-relative — pass it
// to New.
func CurrentPath() string {
	p := host.Call("location").Get("pathname").String()
	return trimSlash(stripBase(p, basePath()))
}

// historyState is history.state, through the host.
func historyState() js.Value { return host.Call("historyState") }

// currentHref is location.href, through the host.
func currentHref() string { return host.Call("location").Get("href").String() }

// stateCopy copies a history.state object into a Go map so it can be passed to
// pushState/replaceState again without dropping the app's own keys. clearScroll
// starts the new entry at the top (nil means "no saved position yet").
func stateCopy(src js.Value, clearScroll bool) map[string]any {
	m := make(map[string]any)
	if src.Truthy() && src.Type() == js.TypeObject {
		keys := js.Global().Get("Object").Call("keys", src)
		for i := 0; i < keys.Length(); i++ {
			k := keys.Index(i).String()
			m[k] = src.Get(k)
		}
	}
	if clearScroll {
		m["gooweeScroll"] = nil
	}
	return m
}

func stateKey(m map[string]any) int {
	switch k := m["gooweeKey"].(type) {
	case js.Value:
		if k.Truthy() {
			return int(k.Float())
		}
	case int:
		return k
	case float64:
		return int(k)
	}
	return 0
}

// ensureScrollKey gives an entry a stable id so scroll events can update its
// position independently of history.state.
func ensureScrollKey(m map[string]any) map[string]any {
	if stateKey(m) == 0 {
		m["gooweeKey"] = nextScrollKey
		nextScrollKey++
	}
	return m
}

// scrollState is the state for a replaceState that is not a route navigation
// (a query-parameter update): it keeps the entry's key and scroll position.
func (r *Router) scrollState() any {
	return ensureScrollKey(stateCopy(historyState(), false))
}

// freshScrollState is the state for a new history entry. Its scroll position
// is filled in by the scroll listener; the key is new so it gets its own one.
func (r *Router) freshScrollState() any {
	m := stateCopy(historyState(), true)
	delete(m, "gooweeKey")
	return ensureScrollKey(m)
}

func (r *Router) currentScroll() map[string]any {
	pos := host.Call("scrollPosition", r.navOptions.Container)
	// map[string]any, not map[string]float64: syscall/js only converts the
	// former to a JavaScript object.
	return map[string]any{"x": pos.Get("x").Float(), "y": pos.Get("y").Float()}
}

// saveScroll records the current entry's position in history.state before a
// history change. Scroll events keep scrollPositions up to date in between.
func (r *Router) saveScroll() {
	state := ensureScrollKey(stateCopy(historyState(), false))
	key := stateKey(state)
	pos := scrollPositions[key]
	if pos == nil {
		pos = r.currentScroll()
		scrollPositions[key] = pos
	}
	state["gooweeScroll"] = pos
	host.Call("replaceState", state, currentHref())
}

// savedForPop prefers the live position for the entry the browser just popped
// to, falling back to what history.state has (e.g. after a page reload).
func savedForPop(state js.Value) any {
	m := stateCopy(state, false)
	key := stateKey(m)
	if pos := scrollPositions[key]; pos != nil {
		return map[string]any{"gooweeScroll": pos}
	}
	return state
}

// number reads a position component from either a JS history.state object or
// a Go map handed to applyNav.
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case js.Value:
		if x.Truthy() {
			return x.Float(), true
		}
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func savedScroll(v any) (x, y float64, ok bool) {
	var src any
	switch s := v.(type) {
	case js.Value:
		if !s.Truthy() || s.Type() != js.TypeObject {
			return 0, 0, false
		}
		pos := s.Get("gooweeScroll")
		if !pos.Truthy() || pos.Type() != js.TypeObject {
			return 0, 0, false
		}
		x, xOK := number(pos.Get("x"))
		y, yOK := number(pos.Get("y"))
		return x, y, xOK && yOK
	case map[string]any:
		src = s["gooweeScroll"]
	default:
		return 0, 0, false
	}
	pos, isMap := src.(map[string]any)
	if !isMap {
		return 0, 0, false
	}
	x, xOK := number(pos["x"])
	y, yOK := number(pos["y"])
	return x, y, xOK && yOK
}

func (r *Router) applyScroll(kind navKind, fragment string, saved any, container string) {
	if host.Call("scrollToFragment", fragment, container).Bool() {
		return
	}
	if kind == navFragment {
		// A same-page fragment change with no target leaves the scroll where
		// it is; it must not fall through to the "start at the top" rule.
		return
	}
	if kind == navPop {
		if x, y, ok := savedScroll(saved); ok {
			host.Call("scrollTo", container, x, y)
			return
		}
	}
	// A push (or a popstate entry with no saved position) starts at the top.
	host.Call("scrollTo", container, 0, 0)
}

func focusRoute()    { host.Call("focusRoute") }
func announceRoute() { host.Call("announceRoute") }

// applyNav is the browser half of a navigation: it runs after the route's
// frame is in the DOM, so fragments, headings and scroll containers exist.
// scope keeps query-only updates inert and limits fragment-only changes to a
// scroll to the target.
func (r *Router) applyNav(kind navKind, fragment string, saved any, opt ScrollOptions, scope navScope) {
	if opt.Disabled || scope == navScopeNone {
		return
	}
	if scope == navScopeFragment {
		r.applyScroll(navFragment, fragment, nil, opt.Container)
		return
	}
	r.applyScroll(kind, fragment, saved, opt.Container)
	if !opt.FocusDisabled {
		focusRoute()
	}
	if !opt.AnnounceDisabled {
		announceRoute()
	}
}

// BindHistory wires the router to the browser's History API: Navigate pushes a
// new entry, NavigateReplace replaces the current, Back/Forward follow the
// browser stack, and popstate syncs Path without pushing another entry. It also
// captures the page's base path, so links and pushState carry it while route
// matching stays base-relative. Call once at startup.
func (r *Router) BindHistory() {
	r.base = basePath()
	loc := host.Call("location")
	// We restore positions ourselves, after the route's frame is applied;
	// the browser's automatic restoration happens too early (and is unreliable
	// for content rendered from WASM).
	host.Call("manualScrollRestoration")

	// The page's query and fragment; the path was given to New.
	r.syncURL(r.Path.Peek(), loc.Get("search").String(), loc.Get("hash").String())
	// Give the first entry a state object too, so the scroll listener can
	// record a position without replacing an app-owned state object.
	host.Call("replaceState", r.scrollState(), loc.Get("href").String())

	r.navFn = func(url string) {
		host.Call("pushState", r.scrollState(), url) // url already base-prefixed
	}
	r.replaceFn = func(url string) {
		host.Call("replaceState", r.scrollState(), url)
	}
	r.pushStateFn = func(url, prevPath, prevFragment string) {
		scope := changedNavScope(prevPath, prevFragment, r.Path.Peek(), r.Fragment.Peek())
		r.saveScroll()
		state := r.freshScrollState()
		if scope == navScopeNone {
			// A query-only push stays on the same page: keep this entry's
			// scroll key instead of starting a fresh position map entry.
			state = r.scrollState()
		}
		host.Call("pushState", state, url)
		core.AfterFrame(func() {
			r.applyNav(navPush, r.Fragment.Peek(), nil, r.navOptions, scope)
		})
	}
	r.replaceStateFn = func(url, prevPath, prevFragment string) {
		scope := changedNavScope(prevPath, prevFragment, r.Path.Peek(), r.Fragment.Peek())
		r.saveScroll()
		state := r.freshScrollState()
		if scope == navScopeNone {
			state = r.scrollState()
		}
		host.Call("replaceState", state, url)
		core.AfterFrame(func() {
			r.applyNav(navReplace, r.Fragment.Peek(), nil, r.navOptions, scope)
		})
	}
	r.backFn = func() {
		r.saveScroll()
		host.Call("back")
	}
	r.forwardFn = func() {
		r.saveScroll()
		host.Call("forward")
	}

	pop := js.FuncOf(func(this js.Value, args []js.Value) any {
		// Sync from the location (not via Navigate) so we don't push a new
		// entry on top of the one the browser just popped. One batch: a
		// watcher never sees the new path with the old query (#142).
		loc := host.Call("location")
		prevPath, prevFragment := r.Path.Peek(), r.Fragment.Peek()
		r.syncURL(trimSlash(stripBase(loc.Get("pathname").String(), r.base)), loc.Get("search").String(), loc.Get("hash").String())
		saved := savedForPop(historyState())
		scope := changedNavScope(prevPath, prevFragment, r.Path.Peek(), r.Fragment.Peek())
		core.AfterFrame(func() {
			r.applyNav(navPop, r.Fragment.Peek(), saved, r.navOptions, scope)
		})
		return nil
	})
	browserListeners = append(browserListeners, pop)
	host.Call("on", "popstate", pop, false)

	// Track the live position for each entry. Coalesce scroll events to one
	// update per frame; the position is copied into history.state only when a
	// route change leaves the entry (saveScroll).
	var pending bool
	scroll := js.FuncOf(func(this js.Value, args []js.Value) any {
		if pending {
			return nil
		}
		pending = true
		var frame js.Func
		frame = js.FuncOf(func(this js.Value, args []js.Value) any {
			defer frame.Release()
			pending = false
			state := stateCopy(historyState(), false)
			if stateKey(state) == 0 {
				state = ensureScrollKey(state)
				host.Call("replaceState", state, currentHref())
			}
			scrollPositions[stateKey(state)] = r.currentScroll()
			return nil
		})
		js.Global().Call("requestAnimationFrame", frame)
		return nil
	})
	browserListeners = append(browserListeners, scroll)
	host.Call("on", "scroll", scroll, true)

	beforeUnload := js.FuncOf(func(this js.Value, args []js.Value) any {
		r.saveScroll()
		if len(args) > 0 && !r.canLeave(currentHref()) {
			args[0].Call("preventDefault")
			args[0].Set("returnValue", "")
		}
		return nil
	})
	browserListeners = append(browserListeners, beforeUnload)
	host.Call("on", "beforeunload", beforeUnload, false)
}
