//go:build js && wasm

package data

import (
	"encoding/json"
	"syscall/js"
)

// In the browser, the default store reads the page's data the first time a
// Load asks: from the script the server wrote, if the page is the one the
// server rendered for this URL (goowee.serverRendered).
func init() {
	client.load = func() map[string]json.RawMessage {
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
}
