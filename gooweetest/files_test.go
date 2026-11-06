package gooweetest_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// composer takes pasted images as attachments (reading their bytes off the
// render loop), lets text pastes through, and completes "@a" to "@ada " with
// the caret after the completion.
func composer() core.Node {
	return core.Component("Composer", func() core.Node {
		draft, setDraft := hooks.UseState("")
		attached, setAttached := hooks.UseState([]string{})
		lastText, setLastText := hooks.UseState("")
		copied, setCopied := hooks.UseState("")
		ref := Ref()
		return Div(
			Textarea(RefTo(ref), BindValue(draft),
				OnPasteE(func(e core.EventData) {
					files := e.Files()
					if len(files) == 0 {
						return // text: let the browser insert it
					}
					e.PreventDefault()
					for _, f := range files {
						go func() {
							data, err := f.Bytes()
							core.Schedule(func() {
								setAttached(append(attached.Get(), fmt.Sprintf("%s %s %d %v", f.Name, f.Type, len(data), err)))
							})
						}()
					}
				}),
				OnCopy(setCopied),
			),
			Input(Class("plain"), OnPaste(setLastText)),
			Button(Text("Complete"), OnClick(func() {
				// The caret call comes first: it still lands after the value.
				before, after, _ := strings.Cut(draft.Get(), "@a")
				at := len(utf16.Encode([]rune(before + "@ada ")))
				ref.Call("setSelectionRange", at, at)
				setDraft(before + "@ada " + after)
			})),
			Ul(Class("attached"), For(attached, func(s string) string { return s }, func(s string) core.Node { return Li(Text(s)) })),
			P(Class("pasted"), TextS(lastText)),
			P(Class("copied"), TextS(copied)),
		)
	})
}

func TestPasteFilesAndText(t *testing.T) {
	s := gooweetest.Render(t, composer())
	box := s.Find("textarea")
	png := []byte("\x89PNG\r\n\x1a\nfake")
	if !s.Paste(box, "", gooweetest.File{Name: "image.png", Type: "image/png", Data: png}) {
		t.Fatal("an image paste wasn't prevented")
	}
	s.WaitForText("image.png image/png 12 <nil>")
	if s.Paste(box, "just text") {
		t.Fatal("a text paste was prevented")
	}
	if s.Paste(s.Find("input.plain"), "pasted words"); s.Find("p.pasted").Text() != "pasted words" {
		t.Fatalf("OnPaste got %q", s.Find("p.pasted").Text())
	}
	// Tests written against the old payload still pass "value".
	s.Dispatch(s.Find("input.plain"), "paste", map[string]any{"value": "old style"})
	if got := s.Find("p.pasted").Text(); got != "old style" {
		t.Fatalf("OnPaste with a value-only payload got %q", got)
	}
	s.Dispatch(box, "copy", map[string]any{"text": "selection"})
	if got := s.Find("p.copied").Text(); got != "selection" {
		t.Fatalf("OnCopy got %q", got)
	}
}

func TestRefCallPlacesCaretAfterUpdate(t *testing.T) {
	s := gooweetest.Render(t, composer())
	box := s.Find("textarea")
	s.Input(box, "hi 🎉 @a there")
	s.Click(s.FindByText("Complete"))
	if got := box.Value(); got != "hi 🎉 @ada  there" {
		t.Fatalf("value %q", got)
	}
	// "hi 🎉 @ada " is 11 UTF-16 units (the emoji is 2), 14 bytes.
	if start, end := box.Prop("selectionStart"), box.Prop("selectionEnd"); start != 11.0 || end != 11.0 {
		t.Fatalf("caret at %v..%v, want 11 (after the completion)", start, end)
	}
	calls := box.Calls()
	if len(calls) != 1 || calls[0].Method != "setSelectionRange" || fmt.Sprint(calls[0].Args) != "[11 11]" {
		t.Fatalf("calls %+v", calls)
	}
}

func TestDropZoneAndFileInput(t *testing.T) {
	s := gooweetest.Render(t, core.Component("Uploads", func() core.Node {
		got, setGot := hooks.UseState("")
		over, setOver := hooks.UseState(false)
		show := func(files []core.File) {
			for _, f := range files {
				go func() {
					data, _ := f.Bytes()
					core.Schedule(func() { setGot(got.Get() + f.Name + "=" + string(data) + " ") })
				}()
			}
		}
		return Div(
			Div(Class("zone"),
				OnDragOver(func(e core.EventData) {
					if slices.Contains(e.DataTypes(), "Files") {
						e.PreventDefault()
						setOver(true)
					}
				}),
				OnDrop(func(e core.EventData) {
					e.PreventDefault()
					show(e.Files())
				}),
				Span(Class("inner"), Text("drop here")),
			),
			Input(Type("file"), OnChangeE(func(e core.EventData) { show(e.Files()) })),
			P(Class("got"), TextS(got)),
			P(Class("over"), Textf("%v", over)),
		)
	}))
	if !s.Dispatch(s.Find("span.inner"), "dragover", gooweetest.DragOverData()) || s.Find("p.over").Text() != "true" {
		t.Fatal("dragover with files wasn't accepted")
	}
	if !s.Drop(s.Find("span.inner"), gooweetest.File{Name: "a.txt", Data: []byte("A")}) {
		t.Fatal("drop wasn't prevented")
	}
	s.WaitForText("a.txt=A")
	s.SelectFiles(s.Find("input"), gooweetest.File{Name: "b.txt", Data: []byte("B")})
	s.WaitForText("b.txt=B")
}
