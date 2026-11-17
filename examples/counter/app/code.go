package app

import (
	"path"
	"strings"
	"time"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/highlight"
)

// writeClipboard copies text to the clipboard and reports the outcome to done,
// possibly from another goroutine. The browser build sets it
// (clipboard_js.go); elsewhere copying fails, and tests replace it.
var writeClipboard = func(text string, done func(ok bool)) { done(false) }

// copyFeedback is how long the button says "Copied" before it resets.
var copyFeedback = 1500 * time.Millisecond

// copyButton copies text when clicked and says whether it worked.
func copyButton(text string) core.Node {
	return core.Component("CopyButton", func() core.Node {
		label, setLabel := hooks.UseState("Copy")
		gen := 0 // the latest click; an older click's reset is ignored
		return Button(Class("copy"), Type("button"), Attr("aria-live", "polite"),
			OnClick(func() {
				gen++
				mine := gen
				writeClipboard(text, func(ok bool) {
					core.Schedule(func() {
						if mine != gen {
							return
						}
						if ok {
							setLabel("Copied")
						} else {
							setLabel("Copy failed")
						}
					})
					go func() {
						time.Sleep(copyFeedback)
						core.Schedule(func() {
							if mine == gen {
								setLabel("Copy")
							}
						})
					}()
				})
			}),
			TextS(label),
		)
	})
}

// highlighted renders code as highlighted tokens for lang ("go", "html", "sh";
// anything else is plain text).
func highlighted(lang, code string) []core.Item {
	toks := highlight.For(lang, code)
	out := make([]core.Item, len(toks))
	for i, t := range toks {
		if t.Kind == highlight.Plain {
			out[i] = Text(t.Text)
		} else {
			out[i] = Span(Class("tok-"+t.Kind.String()), Text(t.Text))
		}
	}
	return out
}

// codePanel is a titled code panel: the file name and a copy button in its
// bar, highlighted by the file's extension.
func codePanel(filename, code string) core.Node {
	return Div(Class("panel"),
		Div(Class("panel-bar"),
			Span(Class("dot")), Span(Class("dot")), Span(Class("dot")),
			Span(Class("panel-file"), Text(filename)),
			copyButton(code),
		),
		Pre(Class("code"), Code(highlighted(path.Ext(filename), code)...)),
	)
}

// snippet is an untitled code block (commands, a page shell) with a copy
// button in its corner.
func snippet(lang, code string) core.Node {
	return Div(Class("snippet"),
		Pre(Class("code"), Code(highlighted(lang, code)...)),
		copyButton(code),
	)
}

// rich renders prose that marks identifiers with backticks: each `span`
// becomes a <code> element. An unmatched backtick is kept as text.
func rich(s string) core.Item {
	parts := strings.Split(s, "`")
	if len(parts)%2 == 0 { // an odd number of backticks: the last one is literal
		n := len(parts)
		parts[n-2] += "`" + parts[n-1]
		parts = parts[:n-1]
	}
	items := make([]core.Item, 0, len(parts))
	for i, p := range parts {
		switch {
		case p == "":
		case i%2 == 1:
			items = append(items, Code(Text(p)))
		default:
			items = append(items, Text(p))
		}
	}
	return Group(items...)
}
