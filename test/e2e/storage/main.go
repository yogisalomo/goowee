//go:build js && wasm

// Command storage is the app test/e2e/storage.mjs drives: goowee/storage
// against the browser's localStorage and sessionStorage, across a reload and
// with storage disabled (#100).
package main

import (
	"fmt"
	"strings"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/storage"
)

func main() {
	bridge.Run(core.Component("StorageE2E", func() core.Node {
		draft, setDraft := hooks.UseState("")
		restored, setRestored := hooks.UseState("")
		saveErr, setSaveErr := hooks.UseState("")
		hooks.OnMount(func() func() {
			d, ok := storage.Local.Get("draft")
			visits, _ := storage.Session.Get("visits")
			storage.Session.Set("visits", visits+"x")
			setRestored(fmt.Sprintf("draft=%q found=%v visits=%q keys=%s", d, ok, visits, strings.Join(storage.Local.Keys(), ",")))
			if ok {
				setDraft(d)
			}
			return nil
		})
		hooks.Watch([]core.SignalAccessor{draft}, func() {
			err := storage.Local.Set("draft", draft.Get())
			setSaveErr(fmt.Sprint(err))
		})
		return Div(
			Textarea(ID("draft"), BindValue(draft)),
			P(ID("restored"), TextS(restored)),
			P(ID("save"), TextS(saveErr)),
			Button(ID("clear"), OnClick(func() { storage.Local.Remove("draft"); setRestored("removed") }), Text("clear")),
		)
	}))
}
