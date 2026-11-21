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

// AssetName reports which browser asset path names: "goowee.js" or
// "wasm_exec.js". It matches the bare name and any sub-path ending in it, so
// an app served under a base path still gets the matching runtime.
func AssetName(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	switch path {
	case "goowee.js", "wasm_exec.js":
		return path
	}
	return ""
}

// Handler serves goowee.js from this module and wasm_exec.js from the Go
// toolchain, with content hashes for ETags. It never serves a runtime copied
// into a static directory, so the bytes always match the linked binary.
func Handler() http.Handler {
	js := RuntimeJS()
	jsTag := etag(js)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := AssetName(r.URL.Path)
		if name == "" {
			http.NotFound(w, r)
			return
		}
		body, tag := js, jsTag
		if name == "wasm_exec.js" {
			b, err := wasmExecJS()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			body, tag = b, etag(b)
		}
		serveJS(w, r, body, tag)
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
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
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
