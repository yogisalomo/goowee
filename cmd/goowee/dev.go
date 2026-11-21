package main

import (
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yogisalomo/goowee/web"
)

// devCmd is `goowee dev`: build, serve, and on every change rebuild and
// reload the page.
func devCmd(args []string) error {
	fset := flag.NewFlagSet("dev", flag.ContinueOnError)
	addr := fset.String("addr", "localhost:8080", "listen address")
	var proxies proxyFlags
	fset.Var(&proxies, "proxy", "forward a path prefix to another server, e.g. /api=http://localhost:9000 (repeatable)")
	if err := fset.Parse(reorder(args)); err != nil {
		return err
	}
	dir := "."
	if fset.NArg() > 0 {
		dir = fset.Arg(0)
	}
	p, err := loadProject(dir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	d := newDevServer(p, proxies)
	go d.watch(ctx, 250*time.Millisecond)
	fmt.Fprintf(os.Stderr, "goowee: serving %s on http://%s (dev mode; edits rebuild and reload)\n", p.dir, ln.Addr())
	srv := &http.Server{Handler: d}
	go func() { <-ctx.Done(); srv.Close(); d.stopServer() }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// proxyFlags collects --proxy /prefix=URL.
type proxyFlags []string

func (p *proxyFlags) String() string     { return strings.Join(*p, ",") }
func (p *proxyFlags) Set(v string) error { *p = append(*p, v); return nil }

// devServer builds the project into stage (.goowee/dev), serves it, and
// tells pages to reload over server-sent events.
type devServer struct {
	p       *project
	stage   string
	proxies []prefixProxy

	mu      sync.Mutex
	builds  int    // completed builds, successful or not
	failed  string // the last build's error output, "" if it succeeded
	clients map[chan string]bool
	server  *exec.Cmd // the SSR server, for projects with server/
	backend *httputil.ReverseProxy
}

type prefixProxy struct {
	prefix string
	proxy  *httputil.ReverseProxy
}

func newDevServer(p *project, proxies []string) *devServer {
	d := &devServer{p: p, stage: filepath.Join(p.dir, ".goowee", "dev"), clients: map[chan string]bool{}}
	for _, spec := range proxies {
		prefix, target, ok := strings.Cut(spec, "=")
		u, err := url.Parse(target)
		if !ok || err != nil || u.Host == "" {
			fmt.Fprintf(os.Stderr, "goowee: ignoring --proxy %q: want /prefix=http://host:port\n", spec)
			continue
		}
		d.proxies = append(d.proxies, prefixProxy{prefix, httputil.NewSingleHostReverseProxy(u)})
	}
	return d
}

// watch rebuilds whenever the project's Go files or static files change,
// polling their sizes and modification times (no dependency on a file
// notification library), and once at the start.
func (d *devServer) watch(ctx context.Context, every time.Duration) {
	last := ""
	for {
		if s := d.snapshot(); s != last {
			if last != "" {
				time.Sleep(every / 2) // let an editor finish writing
				s = d.snapshot()
			}
			last = s
			d.build()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// snapshot summarizes what a build depends on: every .go file, go.mod,
// go.sum, goowee.json, and the files under web/.
func (d *devServer) snapshot() string {
	var b strings.Builder
	filepath.WalkDir(d.p.dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := e.Name()
		if e.IsDir() {
			if p != d.p.dir && (strings.HasPrefix(name, ".") || name == "dist" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		inWeb := d.p.web != "" && strings.HasPrefix(p, d.p.web+string(filepath.Separator))
		if !inWeb && !strings.HasSuffix(name, ".go") && name != "go.mod" && name != "go.sum" && name != configFile {
			return nil
		}
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return b.String()
}

// build stages the static files, builds the wasm (and the server), and
// tells every page to reload, or what failed.
func (d *devServer) build() {
	start := time.Now()
	failed := d.stageFiles()
	if failed == "" {
		tmp := filepath.Join(d.stage, "main.wasm.tmp")
		if out, err := d.p.buildWasm(tmp, wasmDev); err != nil {
			failed = "building the WASM failed:\n" + string(out)
		} else if err := os.Rename(tmp, filepath.Join(d.stage, "main.wasm")); err != nil {
			failed = err.Error()
		}
	}
	if failed == "" && d.p.server {
		failed = d.restartServer()
	}
	d.mu.Lock()
	d.builds++
	d.failed = failed
	clients := make([]chan string, 0, len(d.clients))
	for c := range d.clients {
		clients = append(clients, c)
	}
	d.mu.Unlock()
	event := "reload"
	if failed != "" {
		event = "error"
		fmt.Fprintf(os.Stderr, "goowee: %s\n", strings.TrimSpace(failed))
	} else {
		fmt.Fprintf(os.Stderr, "goowee: built in %s\n", time.Since(start).Round(10*time.Millisecond))
	}
	for _, c := range clients {
		select {
		case c <- event:
		default:
		}
	}
}

// stageFiles copies web/ and the runtime assets into the stage directory.
func (d *devServer) stageFiles() string {
	if err := os.MkdirAll(d.stage, 0o755); err != nil {
		return err.Error()
	}
	if d.p.web != "" {
		if err := copyDir(d.p.web, d.stage); err != nil {
			return "copying web/: " + err.Error()
		}
	}
	loader, err := d.p.loader()
	if err != nil {
		return err.Error()
	}
	if err := os.WriteFile(filepath.Join(d.stage, "wasm_exec.js"), loader, 0o644); err != nil {
		return err.Error()
	}
	if err := os.WriteFile(filepath.Join(d.stage, "goowee.js"), web.RuntimeJS(), 0o644); err != nil {
		return err.Error()
	}
	return ""
}

// restartServer builds server/ and runs it on a free local port, serving the
// stage directory; requests that aren't the dev server's own go to it.
func (d *devServer) restartServer() string {
	bin := filepath.Join(d.p.dir, ".goowee", "server")
	if out, err := d.p.buildServer(bin, false); err != nil {
		return "building server/ failed:\n" + string(out)
	}
	d.stopServer()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err.Error()
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(bin, "-addr", addr, "-static", d.stage)
	cmd.Dir = d.p.dir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return "starting server/: " + err.Error()
	}
	for i := 0; i < 100; i++ { // up to 5 s for it to listen
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	u, _ := url.Parse("http://" + addr)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ModifyResponse = injectResponse
	d.mu.Lock()
	d.server, d.backend = cmd, proxy
	d.mu.Unlock()
	return ""
}

func (d *devServer) stopServer() {
	d.mu.Lock()
	cmd := d.server
	d.server = nil
	d.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
}

func (d *devServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/_goowee/events":
		d.events(w, r)
		return
	case "/_goowee/dev.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		io.WriteString(w, devJS)
		return
	}
	for _, p := range d.proxies {
		if strings.HasPrefix(r.URL.Path, p.prefix) {
			p.proxy.ServeHTTP(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	d.mu.Lock()
	backend := d.backend
	d.mu.Unlock()
	if file := d.staged(r.URL.Path); file != "" {
		serveStaged(w, r, file)
		return
	}
	if backend != nil {
		r.Header.Del("Accept-Encoding") // the reload script goes into its HTML
		backend.ServeHTTP(w, r)
		return
	}
	// A client-only app: any path without an extension is a page, served by
	// the shell. A missing file with an extension (main.wasm before the
	// first build) is a 404, not the shell.
	if path.Ext(r.URL.Path) == "" {
		if file := d.staged("/index.html"); file != "" {
			serveStaged(w, r, file)
			return
		}
	}
	d.mu.Lock()
	failed := d.failed
	d.mu.Unlock()
	if failed != "" && path.Ext(r.URL.Path) == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, inject([]byte("<!doctype html><title>build failed</title><body></body>")))
		return
	}
	http.NotFound(w, r)
}

// staged is the staged file for urlPath, or "" if there is none.
func (d *devServer) staged(urlPath string) string {
	file := filepath.Join(d.stage, filepath.FromSlash(path.Clean("/"+urlPath)))
	if fi, err := os.Stat(file); err == nil && !fi.IsDir() {
		return file
	}
	return ""
}

// serveStaged serves a file with the type the browser needs (application/wasm
// for streaming compilation), gzip-compressed when the browser accepts it,
// and the reload script in HTML.
func serveStaged(w http.ResponseWriter, r *http.Request, file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctype := contentType(file)
	if strings.HasPrefix(ctype, "text/html") {
		data = []byte(inject(data))
	}
	w.Header().Set("Content-Type", ctype)
	if compressible(ctype) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		zw, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		zw.Write(data)
		zw.Close()
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func contentType(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".wasm":
		return "application/wasm"
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	}
	if t := mimeByExt(filepath.Ext(file)); t != "" {
		return t
	}
	return "application/octet-stream"
}

func compressible(ctype string) bool {
	return strings.HasPrefix(ctype, "text/") || ctype == "application/wasm" || ctype == "application/json" || ctype == "image/svg+xml"
}

// events streams "reload" and "error" events to a page. A page that connects
// after a failed build gets the error at once.
func (d *devServer) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan string, 4)
	d.mu.Lock()
	d.clients[c] = true
	failed := d.failed
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.clients, c)
		d.mu.Unlock()
	}()
	io.WriteString(w, ": connected\n\n")
	if failed != "" {
		writeEvent(w, "error", failed)
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-c:
			data := ""
			if ev == "error" {
				d.mu.Lock()
				data = d.failed
				d.mu.Unlock()
			}
			writeEvent(w, ev, data)
			flusher.Flush()
		case <-time.After(20 * time.Second):
			io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func writeEvent(w io.Writer, event, data string) {
	fmt.Fprintf(w, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	io.WriteString(w, "\n")
}

// injectResponse adds the reload script to a proxied HTML page.
func injectResponse(resp *http.Response) error {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || resp.Header.Get("Content-Encoding") != "" {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	out := inject(body)
	resp.Body = io.NopCloser(strings.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	resp.Header.Set("Cache-Control", "no-cache")
	return nil
}

const devScript = `<script src="/_goowee/dev.js"></script>`

// inject puts the dev script in a page: before the script that boots goowee,
// so dev mode is on before the app starts, or at the end of the body.
func inject(page []byte) string {
	s := string(page)
	if i := strings.Index(s, "goowee.boot("); i >= 0 {
		if j := strings.LastIndex(s[:i], "<script"); j >= 0 {
			return s[:j] + devScript + s[j:]
		}
	}
	if i := strings.LastIndex(s, "</body>"); i >= 0 {
		return s[:i] + devScript + s[i:]
	}
	return s + devScript
}

// devJS turns dev mode on (the inspector and dependency checks, unless the
// URL says ?goowee-dev=0), reloads the page after a successful build, and
// shows a failed build's errors over the page.
const devJS = `(function () {
  if (window.goowee && !/[?&]goowee-dev=0\b/.test(location.search)) goowee._dev = true;
  function overlay(text) {
    var el = document.getElementById("goowee-build-error");
    if (!el) {
      el = document.createElement("pre");
      el.id = "goowee-build-error";
      el.style.cssText = "position:fixed;inset:0;margin:0;padding:24px;overflow:auto;z-index:2147483647;" +
        "background:rgba(24,24,27,.96);color:#fecaca;font:13px/1.5 ui-monospace,monospace;white-space:pre-wrap";
      (document.body || document.documentElement).appendChild(el);
    }
    el.textContent = "goowee: the build failed\n\n" + text;
  }
  var es = new EventSource("/_goowee/events");
  es.addEventListener("reload", function () { location.reload(); });
  es.addEventListener("error", function (e) { if (e.data !== undefined) overlay(e.data); });
})();
`

// mimeByExt is the type the system knows for other extensions.
func mimeByExt(ext string) string { return mime.TypeByExtension(ext) }
