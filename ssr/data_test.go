package ssr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func dataDoc(w io.Writer, head, body string) {
	fmt.Fprintf(w, "<html><head>%s</head><body><div id=root>%s</div></body></html>", head, body)
}

// view renders a Load of key whose data is what fetch returns.
func loadPage(key string, fetch func(ctx context.Context) (string, error)) core.Node {
	return h.Div(hooks.Load(key, nil, fetch, func(res *hooks.Resource[string]) core.Node {
		return h.ShowElse(res.Loading,
			func() core.Node { return h.P(h.Text("loading")) },
			func() core.Node { return h.P(h.Class("data"), h.TextS(res.Data)) })
	}))
}

func getBody(t *testing.T, hd http.Handler, path string) string {
	t.Helper()
	w := httptest.NewRecorder()
	hd.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w.Body.String()
}

// #152: the server runs a Load's fetch, renders with its data, and embeds the
// data for the client, once per request.
func TestHandlerLoadsData(t *testing.T) {
	var calls atomic.Int32
	hd := Handler(HandlerOptions{Document: dataDoc, Page: func(r *http.Request) Page {
		return Page{Node: loadPage("greeting", func(ctx context.Context) (string, error) {
			calls.Add(1)
			return "hello </script><b>", nil
		})}
	}})
	body := getBody(t, hd, "/")
	if !strings.Contains(body, `class="data"`) || !strings.Contains(body, "hello &lt;/script&gt;&lt;b&gt;") {
		t.Fatalf("the page wasn't rendered with its data:\n%s", body)
	}
	want := `<script type="application/json" id="goowee-data">{"greeting":"hello \u003c/script\u003e\u003cb\u003e"}</script>`
	if !strings.Contains(body, want) {
		t.Fatalf("data script missing or unescaped:\n%s", body)
	}
	if strings.Count(body, "</script>") != 1 {
		t.Fatalf("the data closed its script early:\n%s", body)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetch ran %d times, want 1", calls.Load())
	}
}

// A failed or late fetch leaves the loading state, and no data, for the
// client to fetch.
func TestHandlerDataFailureAndTimeout(t *testing.T) {
	for name, fetch := range map[string]func(context.Context) (string, error){
		"error": func(context.Context) (string, error) { return "", errors.New("down") },
		"slow": func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		"panic": func(context.Context) (string, error) { panic("boom") },
	} {
		hd := Handler(HandlerOptions{Document: dataDoc, DataTimeout: 50 * time.Millisecond,
			Page: func(r *http.Request) Page { return Page{Node: loadPage("k", fetch)} }})
		body := getBody(t, hd, "/")
		if !strings.Contains(body, "loading") || strings.Contains(body, "goowee-data") {
			t.Errorf("%s: %s", name, body)
		}
	}
}

// A page without keyed data renders once and carries no script.
func TestHandlerWithoutDataRendersOnce(t *testing.T) {
	builds := 0
	hd := Handler(HandlerOptions{Document: dataDoc, Page: func(r *http.Request) Page {
		builds++
		return Page{Node: h.P(h.Text("static"))}
	}})
	body := getBody(t, hd, "/")
	if builds != 1 || strings.Contains(body, "goowee-data") {
		t.Fatalf("builds %d, body %s", builds, body)
	}
}

// Concurrent requests that overlap keep their data apart: each response has
// its own request's value and none of the others'.
func TestConcurrentRequestsKeepTheirData(t *testing.T) {
	const n = 16
	var started sync.WaitGroup
	started.Add(n)
	release := make(chan struct{})
	hd := Handler(HandlerOptions{Document: dataDoc, Page: func(r *http.Request) Page {
		v := r.URL.Query().Get("v")
		return Page{Node: loadPage("value", func(ctx context.Context) (string, error) {
			started.Done()
			<-release // every request is fetching at once
			return "value-" + v, nil
		})}
	}})
	go func() { started.Wait(); close(release) }()
	bodies := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bodies[i] = getBody(t, hd, fmt.Sprintf("/?v=%d", i))
		}(i)
	}
	wg.Wait()
	for i, body := range bodies {
		mine := fmt.Sprintf("value-%d<", i)
		if !strings.Contains(body, mine) {
			t.Fatalf("request %d didn't get its data: %s", i, body)
		}
		for j := 0; j < n; j++ {
			if j != i && strings.Contains(body, fmt.Sprintf("value-%d<", j)) || j != i && strings.Contains(body, fmt.Sprintf(`"value-%d"`, j)) {
				t.Fatalf("request %d's page has request %d's data: %s", i, j, body)
			}
		}
	}
}

// The data store is provided without a node, so the page's markup (and its
// node ids) is exactly what a plain render gives: a client rendering the app
// alone hydrates it.
func TestDataDoesNotShiftNodeIDs(t *testing.T) {
	page := func() core.Node {
		return h.Div(h.P(h.Text("a")), h.Show(core.NewSignal(true), func() core.Node { return h.Span(h.Text("b")) }))
	}
	plain, _ := New().Render(page())
	hd := Handler(HandlerOptions{Document: dataDoc, Page: func(r *http.Request) Page { return Page{Node: page()} }})
	if body := getBody(t, hd, "/"); !strings.Contains(body, "<div id=root>"+plain+"</div>") {
		t.Fatalf("the handler's markup differs from a plain render:\n%s\n%s", body, plain)
	}
}
