package ssr

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/internal/dom"
)

var session = core.NewContext("anonymous")

func greeting() core.Node {
	return core.Component("Greeting", func() core.Node {
		return session.Use(func(s *core.Signal[string]) core.Node { return h.P(h.Class("who"), h.TextS(s)) })
	})
}

func sessionPage(user string) core.Node {
	return h.Div(greeting(), session.Provide(core.NewSignal(user), h.Section(greeting(), h.Ul(h.Li(greeting())))))
}

// #111: concurrent server renders providing different values each see only
// their own (run under -race in CI), and outside the provider, the default.
func TestContextSSRIsolation(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user := fmt.Sprintf("user-%d", g)
			for range 20 {
				html, _ := New().Render(sessionPage(user))
				// Its own user twice (inside the provider), no one else's, and
				// the default once (outside it).
				if strings.Count(html, user+"<") != 2 || strings.Count(html, "user-") != 2 || strings.Count(html, "anonymous") != 1 {
					errs <- html
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("a render saw another request's value:\n%s", e)
	}
}

// Providers and consumers render the same ids on both sides, so the client
// hydrates the server's markup with the provided values in place.
func TestContextHydrates(t *testing.T) {
	html, _ := New().Render(sessionPage("ada"))
	r := dom.New()
	r.SetHydrating(true)
	muts, _ := r.Render(sessionPage("ada"))
	for _, m := range muts {
		if m.Type != core.MutHydrate {
			continue
		}
		if m.Value == "#comment" {
			if !strings.Contains(html, fmt.Sprintf("<!--/%d-->", m.NodeID)) {
				t.Fatalf("scope anchor %d not in the server markup: %s", m.NodeID, html)
			}
		} else if m.Value != "#text" && !strings.Contains(html, fmt.Sprintf(`data-node-id="%d"`, m.NodeID)) {
			t.Fatalf("node %d (%v) not in the server markup: %s", m.NodeID, m.Value, html)
		}
	}
	if !strings.Contains(html, ">ada<") || !strings.Contains(html, ">anonymous<") {
		t.Fatalf("server markup: %s", html)
	}
}
