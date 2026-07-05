//go:build js && wasm

package router

import "syscall/js"

// CurrentPath returns the browser's current pathname — pass it to New.
func CurrentPath() string {
	return js.Global().Get("location").Get("pathname").String()
}

// BindHistory wires the router to the browser's History API: Navigate pushes a
// new entry, NavigateReplace replaces the current, Back/Forward follow the
// browser stack, and popstate syncs Path without pushing another entry.
// Call once at startup so apps don't reimplement this.
func (r *Router) BindHistory() {
	r.navFn = func(path string) {
		js.Global().Get("history").Call("pushState", nil, "", path)
	}
	r.replaceFn = func(path string) {
		js.Global().Get("history").Call("replaceState", nil, "", path)
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
		r.Path.Set(js.Global().Get("location").Get("pathname").String())
		return nil
	}))
}
