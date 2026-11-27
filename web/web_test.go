package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeJSHasProtocolMarker(t *testing.T) {
	if !strings.Contains(string(RuntimeJS()), "goowee.protocolVersion = 2;") {
		t.Fatal("embedded goowee.js has no protocolVersion marker")
	}
}

func TestAssetName(t *testing.T) {
	for path, want := range map[string]string{
		"goowee.js":              "goowee.js",
		"/goowee.js":             "goowee.js",
		"/base/sub/goowee.js":    "goowee.js",
		"/wasm_exec.js":          "",
		"/base/sub/wasm_exec.js": "",
		"/goowee.js.map":         "",
		"/":                      "",
		"/main.wasm":             "",
	} {
		if got := AssetName(path); got != want {
			t.Errorf("AssetName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHandlerGooweeJS(t *testing.T) {
	h := Handler()
	r := httptest.NewRequest(http.MethodGet, "/goowee.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != jsContentType {
		t.Errorf("Content-Type = %q, want %q", got, jsContentType)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	tag := res.Header.Get("ETag")
	if tag == "" || !strings.HasPrefix(tag, `"`) {
		t.Fatalf("ETag = %q, want a quoted hash", tag)
	}
	if got := w.Body.String(); got != string(RuntimeJS()) {
		t.Error("goowee.js body isn't the embedded runtime")
	}

	r = httptest.NewRequest(http.MethodGet, "/goowee.js", nil)
	r.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status %d, want 304", w.Code)
	}
}

func TestHandlerLeavesWasmExecToStaticFiles(t *testing.T) {
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/base/wasm_exec.js", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", w.Code)
	}
}

func TestHandlerNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/main.wasm", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", w.Code)
	}
}
