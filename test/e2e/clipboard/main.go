//go:build js && wasm

// Command clipboard is the app test/e2e/clipboard.mjs drives: paste and drop
// with files and text (EventData.Files, DataText, DataTypes, File.Bytes), the
// OnPaste/OnCopy helpers, and ref.Call placing the caret after an update
// (#106).
package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func main() { bridge.Run(app()) }

func app() core.Node {
	return core.Component("ClipboardE2E", func() core.Node {
		draft, setDraft := hooks.UseState("")
		files, setFiles := hooks.UseState([]string{})
		paste, setPaste := hooks.UseState("")
		plain, setPlain := hooks.UseState("")
		copied, setCopied := hooks.UseState("")
		over, setOver := hooks.UseState("")
		dropped, setDropped := hooks.UseState("")
		status, setStatus := hooks.UseState("")
		box, email := Ref(), Ref()

		// read reports each file's metadata and the sum of its bytes, read off
		// the render loop as File.Bytes requires.
		read := func(fs []core.File) {
			for _, f := range fs {
				go func() {
					data, err := f.Bytes()
					sum := 0
					for _, b := range data {
						sum += int(b)
					}
					core.Schedule(func() {
						setFiles(append(files.Get(), fmt.Sprintf("%s %s %d %d %v", f.Name, f.Type, len(data), sum, err)))
					})
				}()
			}
		}
		return Div(
			Textarea(ID("box"), RefTo(box), BindValue(draft),
				OnPasteE(func(e core.EventData) {
					setPaste(fmt.Sprintf("text:%s types:%s files:%d", e.DataText(), strings.Join(e.DataTypes(), ","), len(e.Files())))
					if fs := e.Files(); len(fs) > 0 {
						e.PreventDefault()
						read(fs)
					}
				}),
				OnCopy(setCopied),
			),
			Input(ID("plain"), OnPaste(setPlain)),
			Input(ID("email"), Type("email"), RefTo(email)),
			Button(ID("complete"), OnClick(func() {
				before, after, _ := strings.Cut(draft.Get(), "@a")
				at := len(utf16.Encode([]rune(before + "@ada ")))
				box.Call("setSelectionRange", at, at) // before the write; lands after it
				setDraft(before + "@ada " + after)
			}), Text("complete")),
			Button(ID("bad"), OnClick(func() {
				email.Call("setSelectionRange", 1, 1) // throws on an email input
				setStatus("after bad call")
			}), Text("bad")),
			Div(ID("zone"),
				OnDragOver(func(e core.EventData) {
					setOver(fmt.Sprintf("%s files:%d", strings.Join(e.DataTypes(), ","), len(e.Files())))
					if slices.Contains(e.DataTypes(), "Files") {
						e.PreventDefault()
					}
				}),
				OnDrop(func(e core.EventData) {
					e.PreventDefault()
					setDropped(fmt.Sprintf("text:%s at:%v,%v", e.DataText(), e.ClientX() > 0, e.ClientY() > 0))
					read(e.Files())
				}),
				Span(ID("target"), Text("drop here")),
			),
			P(ID("files"), TextS(core.Computed([]core.SignalAccessor{files}, func() string { return strings.Join(files.Get(), "|") }))),
			P(ID("paste"), TextS(paste)),
			P(ID("plain-out"), TextS(plain)),
			P(ID("copied"), TextS(copied)),
			P(ID("over"), TextS(over)),
			P(ID("dropped"), TextS(dropped)),
			P(ID("status"), TextS(status)),
		)
	})
}
