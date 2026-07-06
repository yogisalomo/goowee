//go:build js && wasm

package router

import (
	"strings"
	"syscall/js"
)

// basePath reads the page's <base href> and returns it as a prefix without a
// trailing slash (e.g. "/goowee"), or "" when there is no <base> (served at the
// domain root, as the SSR server and local dev do). It's how the same WASM
// binary works both at "/" and under a GitHub Pages project path.
func basePath() string {
	b := js.Global().Get("document").Call("querySelector", "base")
	if !b.Truthy() {
		return ""
	}
	href := b.Call("getAttribute", "href").String()
	href = strings.TrimSuffix(href, "/")
	if href != "" && !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return href
}

// CurrentPath returns the browser's current pathname, base-relative — pass it
// to New.
func CurrentPath() string {
	p := js.Global().Get("location").Get("pathname").String()
	return stripBase(p, basePath())
}

// BindHistory wires the router to the browser's History API: Navigate pushes a
// new entry, NavigateReplace replaces the current, Back/Forward follow the
// browser stack, and popstate syncs Path without pushing another entry. It also
// captures the page's base path, so links and pushState carry it while route
// matching stays base-relative. Call once at startup.
func (r *Router) BindHistory() {
	r.base = basePath()
	r.navFn = func(url string) {
		js.Global().Get("history").Call("pushState", nil, "", url) // url already base-prefixed
	}
	r.replaceFn = func(url string) {
		js.Global().Get("history").Call("replaceState", nil, "", url)
	}
	r.backFn = func() {
		js.Global().Get("history").Call("back")
	}
	r.forwardFn = func() {
		js.Global().Get("history").Call("forward")
	}
	js.Global().Call("addEventListener", "popstate", js.FuncOf(func(this js.Value, args []js.Value) any {
		// Sync Path directly (not via Navigate) so we don't push a new entry
		// on top of the one the browser just popped.
		p := js.Global().Get("location").Get("pathname").String()
		r.Path.Set(stripBase(p, r.base))
		return nil
	}))
}
