package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yogisalomo/goowee/examples/counter/app"
	"github.com/yogisalomo/goowee/ssr"
)

// acceptsGzip reports whether the client advertised gzip support.
func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// compressible reports whether a content type is worth gzipping. The big win is
// the ~4 MB WASM binary (gzip -9 is ~3.7×); the text assets compress well too.
// Already-compressed formats (png/woff2/…) are served as-is.
func compressible(ct string) bool {
	switch {
	case strings.HasPrefix(ct, "text/"),
		strings.Contains(ct, "javascript"),
		strings.Contains(ct, "json"),
		strings.Contains(ct, "svg"),
		strings.HasPrefix(ct, "application/wasm"):
		return true
	}
	return false
}

// contentType maps a static file's extension to a content type. mime doesn't
// register .wasm on every platform, so pin the one asset that matters.
func contentType(path string) string {
	switch filepath.Ext(path) {
	case ".wasm":
		return "application/wasm"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	}
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// gzipCache holds pre-compressed copies of static files so a large asset (the
// ~4 MB main.wasm) is gzipped once, not on every request — the same "serve a
// pre-compressed sibling" behaviour nginx's gzip_static and the GitHub Pages CDN
// provide. Entries are keyed by path and invalidated when the file's mod time
// changes (so a rebuilt wasm is re-compressed).
type gzipEntry struct {
	body    []byte
	modTime time.Time
}

var (
	gzMu    sync.Mutex
	gzStore = map[string]gzipEntry{}
)

// gzipStatic returns the gzip-compressed bytes of path, computing (and caching)
// them on first use at maximum compression.
func gzipStatic(path string, modTime time.Time) ([]byte, error) {
	gzMu.Lock()
	if e, ok := gzStore[path]; ok && e.modTime.Equal(modTime) {
		body := e.body
		gzMu.Unlock()
		return body, nil
	}
	gzMu.Unlock()

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	body := buf.Bytes()

	gzMu.Lock()
	gzStore[path] = gzipEntry{body: body, modTime: modTime}
	gzMu.Unlock()
	return body, nil
}

// serveStatic serves a static file, gzip-compressed from cache when the client
// accepts it and the type is worth compressing, otherwise via http.ServeFile.
func serveStatic(w http.ResponseWriter, r *http.Request, path string, fi os.FileInfo) {
	ct := contentType(path)
	if acceptsGzip(r) && compressible(ct) {
		if body, err := gzipStatic(path, fi.ModTime()); err == nil {
			h := w.Header()
			h.Set("Content-Type", ct)
			h.Set("Content-Encoding", "gzip")
			h.Set("Vary", "Accept-Encoding")
			h.Set("Content-Length", strconv.Itoa(len(body)))
			w.Write(body)
			return
		}
	}
	http.ServeFile(w, r, path)
}

// shell writes the HTML document around the server-rendered head and body.
// site.css and the scripts are app-shell infrastructure, not page metadata, so
// they live here unconditionally — keeping them out of the app's Metadata
// avoids double-emitting them on the pure-client route (which loads its own
// index.html shell).
func shell(w io.Writer, head, body string) {
	if head == "" {
		head = `    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>goowee — reactive Go UIs in WebAssembly</title>`
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
%s
    <link rel="stylesheet" href="site.css">
    <script src="wasm_exec.js"></script>
    <script src="goowee.js"></script>
    <script src="counter.js"></script>
</head>
<body>
    <div id="root">%s</div>
    <script>goowee.boot();</script>
</body>
</html>`, head, body)
}

// newServer serves static assets from staticDir and server-renders every
// other path through ssr.Handler: the router decides the page (and a 404 for
// unknown paths), a render that panics falls back to the client-rendered
// shell, and /ai — whose <textarea> text can't carry SSR hydration markers
// (ADR-009) — is always client-rendered.
func newServer(staticDir string) http.Handler {
	indexHTML := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := filepath.Join(staticDir, "index.html")
		if fi, err := os.Stat(idx); err == nil && !fi.IsDir() {
			serveStatic(w, r, idx, fi)
			return
		}
		http.NotFound(w, r)
	})
	pages := ssr.Handler(ssr.HandlerOptions{
		Page:       ssr.RoutedPage(app.App),
		Document:   shell,
		Fallback:   indexHTML,
		ClientOnly: func(r *http.Request) bool { return r.URL.Path == "/ai" },
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			staticPath := filepath.Join(staticDir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
			if fi, err := os.Stat(staticPath); err == nil && !fi.IsDir() {
				serveStatic(w, r, staticPath, fi)
				return
			}
		}
		pages.ServeHTTP(w, r)
	})
}

func main() {
	staticDir := "examples/counter"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = filepath.Join("..", "..", "examples", "counter")
	}
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Fatalf("static directory not found at examples/counter/")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := ":" + port
	log.Printf("SSR server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newServer(staticDir)))
}
