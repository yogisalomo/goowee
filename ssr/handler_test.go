package ssr

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
)

func testApp(r *router.Router) core.Node {
	return core.Component("App", func() core.Node {
		return h.Main(
			h.Metadata(h.Title("t")),
			r.Route(map[string]func() core.Node{
				"/":          func() core.Node { return h.P(h.Text("home")) },
				"/boom":      func() core.Node { panic("render failed") },
				"/users/:id": func() core.Node { return h.P(h.Textf("user %s", r.ParamSignal("id"))) },
				"/search":    func() core.Node { return h.P(h.Textf("q=%s", r.QueryParamSignal("q"))) },
				"/docs/*": func() core.Node {
					return r.SubRoute("/docs", map[string]func() core.Node{"/intro": func() core.Node { return h.P(h.Text("intro")) }})
				},
			}),
		)
	})
}

func doc(w io.Writer, head, body string) {
	fmt.Fprintf(w, `<html><head>%s</head><body><div id="root">%s</div></body></html>`, head, body)
}

func get(t *testing.T, hd http.Handler, path string, gz bool) (int, string, http.Header) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if gz {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	hd.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Header().Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(zr)
		body = string(b)
	}
	return rec.Code, body, rec.Header()
}

// #77: the handler renders per request, reports 404 from the router, falls
// back on a panicking render, and serves client-only paths from the fallback.
func TestHandler(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "SHELL") })
	hd := Handler(HandlerOptions{
		Page:       RoutedPage(testApp),
		Document:   doc,
		Fallback:   fallback,
		ClientOnly: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/client") },
	})

	code, body, hdr := get(t, hd, "/", false)
	if code != 200 || !strings.Contains(body, "home") || !strings.Contains(body, "<title>") {
		t.Fatalf("/: %d %s", code, body)
	}
	if hdr.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("content type %q", hdr.Get("Content-Type"))
	}
	if code, body, _ := get(t, hd, "/users/42", false); code != 200 || !strings.Contains(body, "user 42") {
		t.Fatalf("param route: %d %s", code, body)
	}
	if code, body, _ := get(t, hd, "/search?q=go%20wasm", false); code != 200 || !strings.Contains(body, "q=go wasm") {
		t.Fatalf("query: %d %s", code, body)
	}
	if code, body, _ := get(t, hd, "/nope", false); code != 404 || !strings.Contains(body, "404") {
		t.Fatalf("unknown path must be a 404 page: %d %s", code, body)
	}
	if code, _, _ := get(t, hd, "/docs/intro", false); code != 200 {
		t.Fatalf("sub-route: %d", code)
	}
	if code, _, _ := get(t, hd, "/docs/missing", false); code != 404 {
		t.Fatalf("unmatched sub-route must be 404, got %d", code)
	}
	if code, body, _ := get(t, hd, "/boom", false); code != 200 || body != "SHELL" {
		t.Fatalf("a panicking render must fall back: %d %s", code, body)
	}
	if _, body, _ := get(t, hd, "/client/x", false); body != "SHELL" {
		t.Fatalf("client-only path must use the fallback: %s", body)
	}

	// Large enough to compress.
	big := Handler(HandlerOptions{
		Page: func(*http.Request) Page {
			return Page{Node: h.P(h.Text(strings.Repeat("goowee ", 400)))}
		},
		Document: doc,
	})
	code, body, hdr = get(t, big, "/", true)
	if code != 200 || hdr.Get("Content-Encoding") != "gzip" || !strings.Contains(body, "goowee goowee") {
		t.Fatalf("gzip: %d %q", code, hdr.Get("Content-Encoding"))
	}

	// No fallback: a panic is a 500, not a crash.
	bare := Handler(HandlerOptions{Page: RoutedPage(testApp), Document: doc})
	if code, _, _ := get(t, bare, "/boom", false); code != 500 {
		t.Fatalf("want 500 without a fallback, got %d", code)
	}
}
