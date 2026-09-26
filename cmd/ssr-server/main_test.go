package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return out
}

func TestServeStaticGzipsWhenAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.wasm")
	// Repetitive payload so gzip clearly shrinks it.
	raw := bytes.Repeat([]byte("goowee-wasm-payload-"), 4096)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)

	req := httptest.NewRequest(http.MethodGet, "/main.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	serveStatic(rec, req, path, fi)

	res := rec.Result()
	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := res.Header.Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm", got)
	}
	if got := res.Header.Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	body := rec.Body.Bytes()
	if len(body) >= len(raw) {
		t.Fatalf("gzip body (%d) not smaller than raw (%d)", len(body), len(raw))
	}
	if !bytes.Equal(gunzip(t, body), raw) {
		t.Fatal("decompressed body does not match original")
	}
}

func TestServeStaticIdentityWhenNotAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.wasm")
	raw := bytes.Repeat([]byte("x"), 1000)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)

	req := httptest.NewRequest(http.MethodGet, "/main.wasm", nil) // no Accept-Encoding
	rec := httptest.NewRecorder()
	serveStatic(rec, req, path, fi)

	if got := rec.Result().Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (identity)", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatal("identity body does not match original")
	}
}

func TestGzipStaticCachesAndInvalidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.js")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	b1, err := gzipStatic(path, fi.ModTime())
	if err != nil {
		t.Fatal(err)
	}
	// Same modtime -> cached identical slice.
	b2, _ := gzipStatic(path, fi.ModTime())
	if &b1[0] != &b2[0] {
		t.Fatal("expected cached body to be reused for identical mod time")
	}
	// Rewrite with a newer mod time -> re-compressed to new content.
	newTime := fi.ModTime().Add(2 * 1e9) // +2s
	if err := os.WriteFile(path, []byte("v2-different"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, newTime, newTime)
	fi2, _ := os.Stat(path)
	b3, _ := gzipStatic(path, fi2.ModTime())
	if bytes.Equal(gunzip(t, b1), gunzip(t, b3)) {
		t.Fatal("expected re-compression after content+modtime change")
	}
	if string(gunzip(t, b3)) != "v2-different" {
		t.Fatalf("stale cache: got %q", gunzip(t, b3))
	}
}

func TestCompressibleAndContentType(t *testing.T) {
	cases := map[string]bool{
		"application/wasm":               true,
		"text/html; charset=utf-8":       true,
		"text/javascript; charset=utf-8": true,
		"image/png":                      false,
		"font/woff2":                     false,
	}
	for ct, want := range cases {
		if got := compressible(ct); got != want {
			t.Errorf("compressible(%q) = %v, want %v", ct, got, want)
		}
	}
	if got := contentType("x/main.wasm"); got != "application/wasm" {
		t.Errorf("contentType(.wasm) = %q", got)
	}
}

// #77: the reference server renders every route (params included) through
// ssr.Handler — no hard-coded list — with real 404s and a client fallback.
func TestServerRoutes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>client shell"), 0o644)
	os.WriteFile(filepath.Join(dir, "site.css"), []byte("body{}"), 0o644)
	srv := newServer(dir)
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/"); code != 200 || !strings.Contains(body, "data-node-id") {
		t.Fatalf("/ should be server-rendered: %d", code)
	}
	if code, body := get("/greet/bob"); code != 200 || !strings.Contains(body, "Hello, bob!") {
		t.Fatalf("param routes are SSR'd now: %d", code)
	}
	if code, body := get("/definitely-not-a-page"); code != 404 || !strings.Contains(body, "page not found") {
		t.Fatalf("unknown path must be a 404 page, got %d", code)
	}
	if code, body := get("/error"); code != 200 || !strings.Contains(body, "client shell") {
		t.Fatalf("a panicking page falls back to the client shell: %d %s", code, body)
	}
	if code, body := get("/ai"); code != 200 || !strings.Contains(body, "client shell") {
		t.Fatalf("/ai is client-only: %d", code)
	}
	if code, body := get("/site.css"); code != 200 || body != "body{}" {
		t.Fatalf("static files: %d %s", code, body)
	}
	if code, body := get("/../main.go"); code == 200 && strings.Contains(body, "package main") {
		t.Fatal("path traversal escaped the static dir")
	}
}
