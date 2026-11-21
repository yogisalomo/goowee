package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

const jsContentType = "text/javascript; charset=utf-8"

// AssetName reports whether path names the embedded runtime, "goowee.js".
// It matches the bare name and any sub-path ending in it, so an app served
// under a base path still gets the matching runtime. wasm_exec.js is not an
// embedded asset: the app serves that loader from its own static files.
func AssetName(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	if path == "goowee.js" {
		return path
	}
	return ""
}

// Handler serves this module's goowee.js with a content-hash ETag. It never
// serves a runtime copied into a static directory, so the bytes always match
// the linked binary. wasm_exec.js belongs to the app's static files: it comes
// from the app's Go toolchain, which a deployed server may not have.
func Handler() http.Handler {
	js := RuntimeJS()
	tag := etag(js)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AssetName(r.URL.Path) == "" {
			http.NotFound(w, r)
			return
		}
		serveJS(w, r, js, tag)
	})
}

func serveJS(w http.ResponseWriter, r *http.Request, body []byte, tag string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := w.Header()
	h.Set("Content-Type", jsContentType)
	h.Set("ETag", tag)
	// The URL isn't content-hashed: revalidate on every visit so an upgrade
	// picks up the runtime that matches the new binary instead of keeping an
	// old one for a year. A repeat visit is a cheap 304.
	h.Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}

func etag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func wasmExecJS() ([]byte, error) {
	root := os.Getenv("GOROOT")
	if root == "" {
		root = goruntime.GOROOT()
	}
	for _, rel := range []string{"lib/wasm/wasm_exec.js", "misc/wasm/wasm_exec.js"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if body, err := os.ReadFile(path); err == nil {
			return body, nil
		}
	}
	return nil, fmt.Errorf("web: wasm_exec.js not found under GOROOT %q", root)
}
