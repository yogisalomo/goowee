package ssr

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// shared is package-level state every request reads — the pattern that used
// to make SSR subscribe per request (and would race without care).
var shared = core.NewSignal("shared")

var mounts int // must stay 0: effects never run on the server

func page(rows int) core.Node {
	return core.Component("Page", func() core.Node {
		n, _ := hooks.UseState(rows)
		hooks.OnMount(func() func() { mounts++; return nil })
		label := core.Computed([]core.SignalAccessor{shared}, func() string { return strings.ToUpper(shared.Get()) })
		items := make([]int, rows)
		for i := range items {
			items[i] = i
		}
		list := core.NewSignal(items)
		return h.Div(
			h.P(h.TextS(label), h.Textf(" (%d rows)", n)),
			h.Show(core.NewSignal(true), func() core.Node { return h.Span(h.Text("shown")) }),
			h.Ul(h.For(list, func(i int) int { return i }, func(i int) core.Node {
				return h.Li(h.Class("row"), h.Textf("row %d of %s", i, shared))
			})),
		)
	})
}

// #72: SSR renders run concurrently without a global lock, without data races
// (run under -race in CI), and produce identical output.
func TestConcurrentSSRIsSafe(t *testing.T) {
	want, _ := New().Render(page(50))
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				got, _ := New().Render(page(50))
				if got != want {
					errs <- got
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("concurrent render differs:\n%s\nwant\n%s", e, want)
	}
	if mounts != 0 {
		t.Fatalf("effects ran on the server: %d", mounts)
	}
	if shared.SubscriberCount() != 0 {
		t.Fatalf("SSR must not subscribe to package-level signals, got %d", shared.SubscriberCount())
	}
}

// #72: parallel SSR actually scales (it was fully serialized: 1.05× on 8
// goroutines). Skipped under the race detector and on small machines.
func TestConcurrentSSRScales(t *testing.T) {
	if raceEnabled || testing.Short() || runtime.NumCPU() < 4 {
		t.Skip("timing test: needs ≥4 CPUs, no -race, no -short")
	}
	const total = 64
	render := func() { New().Render(page(400)) }
	start := time.Now()
	for i := 0; i < total; i++ {
		render()
	}
	seq := time.Since(start)
	workers := 4
	start = time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < total/workers; i++ {
				render()
			}
		}()
	}
	wg.Wait()
	par := time.Since(start)
	if speedup := float64(seq) / float64(par); speedup < 1.3 {
		t.Fatalf("4 workers gave %.2f× over sequential (want ≥1.3×; it was 1.05× when serialized) — SSR is serialized", speedup)
	} else {
		t.Logf("4 workers: %.2f× (seq %v, par %v)", speedup, seq, par)
	}
}

func BenchmarkSSRParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			New().Render(page(200))
		}
	})
}

// #78: script URLs are blocked in URL attributes, whatever the obfuscation.
func TestSSRBlocksScriptURLs(t *testing.T) {
	for _, v := range []string{
		"javascript:alert(1)", "JaVaScRiPt:alert(1)", "  javascript:alert(1)",
		"java\tscript:alert(1)", "\njavascript:alert(1)", "\x01javascript:alert(1)", "vbscript:msgbox",
	} {
		sig := core.NewSignal(v)
		html, _ := New().Render(h.Div(
			h.A(h.Href(v), h.Text("a")),
			h.Img(h.Src(v)),
			h.A(h.HrefS(sig)),
			h.Form(h.Action(v)),
		))
		if strings.Contains(strings.ToLower(html), "script:") {
			t.Fatalf("%q leaked: %s", v, html)
		}
		if strings.Count(html, "about:blank#blocked") != 4 {
			t.Fatalf("%q: want 4 blocked URLs, got %s", v, html)
		}
	}
	html, _ := New().Render(h.A(h.Href("https://example.com/?q=javascript:x"), h.Attr("title", "javascript: is fine here")))
	if !strings.Contains(html, `href="https://example.com/?q=javascript:x"`) || !strings.Contains(html, `title="javascript: is fine here"`) {
		t.Fatalf("safe values must pass through: %s", html)
	}
}

// #78: attribute names are validated the same way in body and head, and
// namespaced/underscored names are allowed.
func TestSSRAttributeNames(t *testing.T) {
	body, head := New().Render(h.Div(
		h.Metadata(h.Meta(h.Attr(`x" onload="alert(1)`, "1"), h.Attr("property", "og:title"))),
		h.Svg(h.Attr("xlink:href", "#icon"), h.Attr("xml:lang", "en"), h.Attr("data-a_b.c", "ok"),
			h.Attr("on click", "x"), h.Attr("@click", "x")),
	))
	for _, bad := range []string{"onload", "on click", "@click"} {
		if strings.Contains(body+head, bad) {
			t.Fatalf("invalid attribute %q rendered:\n%s\n%s", bad, head, body)
		}
	}
	for _, good := range []string{`xlink:href="#icon"`, `xml:lang="en"`, `data-a_b.c="ok"`, `property="og:title"`} {
		if !strings.Contains(body+head, good) {
			t.Fatalf("valid attribute %s missing:\n%s\n%s", good, head, body)
		}
	}
	_ = fmt.Sprint
}
