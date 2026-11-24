//go:build js && wasm

package data

import (
	"encoding/json"
	"syscall/js"
)

// loadPage reads the page's data for the client store, the first time a Load
// asks: from the script the server wrote, if the page is the one the server
// rendered for this URL (goowee.serverRendered). It is a plain function that
// only Take calls, not a hook set in init, so an app without hooks.Load
// doesn't link encoding/json for it (#126).
func loadPage() map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	g := js.Global().Get("goowee")
	if fn := g.Get("serverRendered"); fn.Type() == js.TypeFunction && !fn.Invoke().Bool() {
		return out
	}
	el := js.Global().Get("document").Call("getElementById", ScriptID)
	if !el.Truthy() {
		return out
	}
	_ = json.Unmarshal([]byte(el.Get("textContent").String()), &out)
	return out
}
