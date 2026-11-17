package app

import (
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
)

func TestRichRendersBackticksAsCode(t *testing.T) {
	s := gooweetest.Render(t, core.Component("R", func() core.Node {
		return Div(P(Class("a"), rich("call `f(x)` then `g` and 1`2")), P(Class("b"), rich("`only`")))
	}))
	if got := s.Find("p.a").HTML(); !strings.Contains(got, "call <code>f(x)</code> then <code>g</code> and 1`2") {
		t.Fatalf("got %s", got)
	}
	if got := s.Find("p.b").HTML(); !strings.Contains(got, "<code>only</code>") {
		t.Fatalf("got %s", got)
	}
}

func TestNoLiteralBackticksOnThePages(t *testing.T) {
	r := router.New("/")
	s := gooweetest.Render(t, App(r))
	pages := []string{"/", "/tutorial"}
	for _, st := range tutorialSteps {
		pages = append(pages, st.path)
	}
	for _, p := range pages {
		r.Navigate(p)
		s.Flush()
		// Code blocks and the AI rules may contain backticks; prose may not.
		for _, sel := range []string{"p", "li", "h1", "h2", "h3"} {
			for _, el := range s.FindAll(sel) {
				if strings.Contains(el.Text(), "`") && el.Find("code") == nil {
					t.Errorf("on %s: literal backtick in <%s>: %q", p, sel, el.Text())
				}
			}
		}
	}
}

func TestCodePanelIsHighlightedAndCopies(t *testing.T) {
	var copied []string
	ok := true
	writeClipboard = func(text string, done func(bool)) { copied = append(copied, text); done(ok) }
	copyFeedback = 20 * time.Millisecond
	t.Cleanup(func() {
		writeClipboard = func(_ string, done func(bool)) { done(false) }
		copyFeedback = 1500 * time.Millisecond
	})

	s := gooweetest.Render(t, counterPage(router.New("/counter")))
	panel := s.Find(".panel")
	if panel.Find(".tok-func") == nil || panel.Find(".tok-comment") == nil || panel.Find(".tok-number") == nil {
		t.Fatalf("panel isn't highlighted:\n%s", panel.HTML())
	}
	if panel.Find("pre").Text() != counterCode {
		t.Fatal("highlighting changed the code's text")
	}
	btn := panel.Find("button.copy")
	s.Click(btn)
	if len(copied) != 1 || copied[0] != counterCode {
		t.Fatalf("copied %q", copied)
	}
	s.WaitForText("Copied")
	s.WaitFor(func() bool { return btn.Text() == "Copy" })

	ok = false
	s.Click(btn)
	s.WaitForText("Copy failed")
}

func TestSetupSnippetsHaveCopyButtons(t *testing.T) {
	s := gooweetest.Render(t, gettingStartedPage(router.New("/getting-started")))
	snips := s.FindAll(".snippet")
	if len(snips) != 7 {
		t.Fatalf("%d snippets, want 7", len(snips))
	}
	for _, sn := range snips {
		if sn.Find("button.copy") == nil {
			t.Fatalf("snippet without a copy button:\n%s", sn.HTML())
		}
	}
	if s.Find(".snippet .tok-attr") == nil || s.Find(".snippet .tok-keyword") == nil {
		t.Fatal("shell/HTML snippets aren't highlighted")
	}
}
