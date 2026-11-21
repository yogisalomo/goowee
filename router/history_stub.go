//go:build !(js && wasm)

package router

// CurrentPath and BindHistory are no-ops off the browser (server render, tests)
// so the same app code compiles for both targets. The browser-behaviour hooks
// are no-ops too: an SSR render has no history or scroll position.
func CurrentPath() string { return "/" }

func (r *Router) BindHistory() {}

func (r *Router) saveScroll() {}

func (r *Router) scrollState() any { return nil }

func (r *Router) freshScrollState() any { return nil }

func (r *Router) applyNav(kind navKind, fragment string, saved any, opt ScrollOptions) {}
