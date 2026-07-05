//go:build !(js && wasm)

package router

// CurrentPath and BindHistory are no-ops off the browser (server render, tests)
// so the same app code compiles for both targets.
func CurrentPath() string { return "/" }

func (r *Router) BindHistory() {}
