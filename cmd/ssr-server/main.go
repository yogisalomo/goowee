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

// accepts reports whether the client's Accept-Encoding lists coding (gzip,
// br) without refusing it with q=0.
func accepts(r *http.Request, coding string) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(name), coding) {
			continue
		}
		if q, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// compressible reports whether a content type is worth compressing. The big
// win is the ~4.7 MB WASM binary (gzip -9 is ~3.8×, brotli -q 11 ~5.2×); the
// text assets compress well too.
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

// brotliSibling opens path+".br", a brotli-compressed copy made at build time
// (`make brotli`: brotli -q 11), when it exists and is at least as new as
// path — a stale copy from an older build is ignored. The comparison is to
// the second: the brotli CLI gives its output the input's mtime truncated to
// whole seconds. Brotli isn't in the
// standard library, so the server serves pre-built copies (like nginx's
// brotli_static) instead of compressing on the fly. For the wasm it is ~27%
// smaller than gzip -9.
func brotliSibling(path string, fi os.FileInfo) (*os.File, os.FileInfo, bool) {
	f, err := os.Open(path + ".br")
	if err != nil {
		return nil, nil, false
	}
	bfi, err := f.Stat()
	if err != nil || bfi.IsDir() || bfi.ModTime().Before(fi.ModTime().Truncate(time.Second)) {
		f.Close()
		return nil, nil, false
	}
	return f, bfi, true
}

// serveStatic serves a static file: its pre-built brotli copy when the client
// accepts br and one is present, else gzip-compressed from cache when the
// client accepts gzip and the type is worth compressing, else via
// http.ServeFile.
func serveStatic(w http.ResponseWriter, r *http.Request, path string, fi os.FileInfo) {
	ct := contentType(path)
	if compressible(ct) && accepts(r, "br") {
		if f, bfi, ok := brotliSibling(path, fi); ok {
			defer f.Close()
			h := w.Header()
			h.Set("Content-Type", ct)
			h.Set("Content-Encoding", "br")
			h.Set("Vary", "Accept-Encoding")
			http.ServeContent(w, r, path, bfi.ModTime(), f)
			return
		}
	}
	if compressible(ct) && accepts(r, "gzip") {
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
