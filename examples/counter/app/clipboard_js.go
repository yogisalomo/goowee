//go:build js && wasm

package app

import "syscall/js"

func init() { writeClipboard = browserClipboard }

// browserClipboard writes text with the async Clipboard API, falling back to
// a hidden textarea and execCommand("copy") where that API is missing (an
// insecure origin, an older browser). done runs once, off the render loop.
func browserClipboard(text string, done func(ok bool)) {
	clip := js.Global().Get("navigator").Get("clipboard")
	if clip.Truthy() && clip.Get("writeText").Type() == js.TypeFunction {
		var ok, fail js.Func
		finish := func(result bool) {
			ok.Release()
			fail.Release()
			done(result)
		}
		ok = js.FuncOf(func(js.Value, []js.Value) any { finish(true); return nil })
		fail = js.FuncOf(func(js.Value, []js.Value) any { finish(execCopy(text)); return nil })
		clip.Call("writeText", text).Call("then", ok, fail)
		return
	}
	done(execCopy(text))
}

func execCopy(text string) bool {
	doc := js.Global().Get("document")
	ta := doc.Call("createElement", "textarea")
	ta.Set("value", text)
	ta.Call("setAttribute", "readonly", "")
	ta.Get("style").Set("cssText", "position:fixed;top:0;left:0;opacity:0")
	doc.Get("body").Call("appendChild", ta)
	ta.Call("select")
	ok := doc.Call("execCommand", "copy").Truthy()
	ta.Call("remove")
	return ok
}
