package ssr

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/data"
	"github.com/yogisalomo/goowee/router"
	"github.com/yogisalomo/goowee/web"
)

// Page is what a Handler renders for one request.
type Page struct {
	// Node is the tree to render.
	Node core.Node
	// Status, if set, is called after the render — so it can consult state the
	// render produced, like router.NotFound — and returns the HTTP status.
	// 0 (or a nil Status) means 200.
	Status func() int
}

// HandlerOptions configures Handler.
type HandlerOptions struct {
	// Page builds the page for a request (required). See RoutedPage for apps
	// built on the router.
	Page func(r *http.Request) Page

	// Document writes the full HTML document around the rendered <head>
	// content (from h.Metadata) and <body> markup (required). The body must
	// end up inside the element the client mounts into (<div id="root">).
	Document func(w io.Writer, head, body string)

	// Fallback serves requests that shouldn't or couldn't be server-rendered:
	// those ClientOnly selects, and any whose render panicked (logged). It is
	// typically the client-only shell (index.html), so the browser renders the
	// page instead. nil answers a panicked render with 500.
	Fallback http.Handler

	// DataTimeout bounds how long a request waits for the page's keyed data
	// (hooks.Load) before rendering without it; 0 means DefaultDataTimeout.
	DataTimeout time.Duration

	// ClientOnly, if set, selects requests to hand straight to Fallback — pages
	// that can't be server-rendered faithfully (e.g. ones whose content depends
	// on the browser).
	ClientOnly func(r *http.Request) bool
}

// Handler serves server-rendered HTML documents. Each request renders
// independently and concurrently (no shared lock); the response is
// gzip-compressed when the client accepts it. It also serves goowee.js and
// wasm_exec.js from web.Handler, so the page cannot load a stale runtime.
//
//	http.Handle("/", ssr.Handler(ssr.HandlerOptions{
//	    Page:     ssr.RoutedPage(app.App),
//	    Document: shell,             // writes <html>…<div id="root">body</div>…
//	    Fallback: indexHTML,         // client-rendered shell
//	}))
func Handler(opts HandlerOptions) http.Handler {
	if opts.Page == nil || opts.Document == nil {
		panic("ssr.Handler: Page and Document are required")
	}
	assets := web.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if web.AssetName(r.URL.Path) != "" {
			assets.ServeHTTP(w, r)
			return
		}
		if opts.ClientOnly != nil && opts.ClientOnly(r) && opts.Fallback != nil {
			opts.Fallback.ServeHTTP(w, r)
			return
		}
		page, body, head, rec := render(opts.Page, r, opts.DataTimeout)
		if rec != nil {
			core.Log(core.LogRecoverRender, "server render panicked; serving the fallback", map[string]any{
				"path":  r.URL.Path,
				"panic": rec,
			})
			if opts.Fallback != nil {
				opts.Fallback.ServeHTTP(w, r)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		status := http.StatusOK
		if page.Status != nil {
			if s := page.Status(); s != 0 {
				status = s
			}
		}
		var doc bytes.Buffer
		opts.Document(&doc, head, body)
		writeHTML(w, r, status, doc.Bytes())
	})
}

// render renders the page for r. A page that loads data (hooks.Load) is
// rendered twice: the first pass collects the keyed fetches, the server runs
// them concurrently (with r's context and dataTimeout), and the second pass
// renders with the results, which are added to head as JSON for the client
// (#152). A page without keyed data renders once, as before.
func render(build func(*http.Request) Page, r *http.Request, dataTimeout time.Duration) (page Page, body, head string, rec any) {
	defer func() { rec = recover() }()
	store := data.NewServer()
	pass := func() {
		page = build(r)
		body, head = New().renderWith(page.Node, data.Provided(store))
	}
	pass()
	if store.Pending() {
		if dataTimeout <= 0 {
			dataTimeout = DefaultDataTimeout
		}
		store.Run(r.Context(), dataTimeout)
		pass()
	}
	if script := store.Script(); script != "" {
		head += script
	}
	return
}

// DefaultDataTimeout bounds how long a server render waits for its keyed
// data (hooks.Load) when HandlerOptions.DataTimeout is 0. A fetch that takes
// longer is left to the client.
const DefaultDataTimeout = 5 * time.Second

// RoutedPage adapts a router-based app — app(router) returns the root node —
// into a Page func: the router starts at the request's path and query, and
// the response is 404 when the router fell through to its not-found route.
// Under Export with a Base, the router gets that base path, so its links
// carry it.
func RoutedPage(app func(*router.Router) core.Node) func(*http.Request) Page {
	return func(r *http.Request) Page {
		rtr := router.NewURL(r.URL)
		if b := BasePath(r); b != "" {
			rtr.SetBasePath(b)
		}
		return Page{
			Node: app(rtr),
			Status: func() int {
				if rtr.NotFound() {
					return http.StatusNotFound
				}
				return http.StatusOK
			},
		}
	}
}

func writeHTML(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Add("Vary", "Accept-Encoding")
	if len(body) >= 1024 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write(body)
		zw.Close()
		h.Set("Content-Encoding", "gzip")
		w.WriteHeader(status)
		w.Write(buf.Bytes())
		return
	}
	w.WriteHeader(status)
	w.Write(body)
}
