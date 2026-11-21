package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/net"
)

type addReq struct {
	Title string `json:"title"`
}
type todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type userKey struct{}

// serveOverHTTP routes the app's fetches to mux, as a browser would reach the
// server, and drops the in-process handlers so Call really goes over HTTP.
func serveOverHTTP(t *testing.T, mux *http.ServeMux) *[]*http.Request {
	t.Helper()
	mu.Lock()
	saved := handlers
	handlers = map[string]handler{}
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); handlers = saved; mu.Unlock() })
	var seen []*http.Request
	n := gooweetest.NewFakeNet(t)
	n.Handle("/", func(req net.Request) (net.Response, error) {
		r := httptest.NewRequest(req.Method, "http://app.test"+req.URL, bytes.NewReader(req.Body))
		for k, v := range req.Header {
			r.Header.Set(k, v)
		}
		seen = append(seen, r)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		body, _ := io.ReadAll(w.Result().Body)
		return net.Response{Status: w.Code, Body: body}, nil
	})
	return &seen
}

func newServer(t *testing.T, opts ...Option) (*http.ServeMux, *Func[addReq, todo]) {
	t.Helper()
	add := Define[addReq, todo]("test.add." + t.Name())
	mux := http.NewServeMux()
	Handle(mux, add, func(ctx context.Context, req addReq) (todo, error) {
		switch req.Title {
		case "":
			return todo{}, Invalid("check the form", map[string]string{"title": "required"})
		case "boom":
			return todo{}, errors.New("database password is hunter2")
		}
		user, _ := ctx.Value(userKey{}).(string)
		return todo{ID: 7, Title: req.Title + user}, nil
	}, opts...)
	return mux, add
}

func TestCallOverHTTP(t *testing.T) {
	mux, add := newServer(t)
	seen := serveOverHTTP(t, mux)
	got, err := add.Call(context.Background(), addReq{Title: "milk"})
	if err != nil || got != (todo{7, "milk"}) {
		t.Fatalf("Call = %+v, %v", got, err)
	}
	r := (*seen)[0]
	if r.Method != "POST" || r.URL.Path != "/_rpc/"+add.Name() || r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("request: %s %s %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
	}
}

func TestErrorsOverHTTP(t *testing.T) {
	mux, add := newServer(t)
	serveOverHTTP(t, mux)
	_, err := add.Call(context.Background(), addReq{})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeInvalid || e.Fields["title"] != "required" || e.Status != 400 {
		t.Fatalf("invalid: %#v", err)
	}
	_, err = add.Call(context.Background(), addReq{Title: "boom"})
	if !errors.As(err, &e) || e.Code != CodeInternal || e.Status != 500 || e.Message != "internal error" {
		t.Fatalf("a plain handler error must reach the client as internal, without its text: %#v", err)
	}
}

func TestSameOriginAndContentType(t *testing.T) {
	mux, add := newServer(t)
	post := func(origin, ct string) int {
		r := httptest.NewRequest("POST", "http://app.test"+add.Path(), bytes.NewReader([]byte(`{"title":"x"}`)))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	for _, tc := range []struct {
		origin, ct string
		want       int
	}{
		{"", "application/json", 200},
		{"http://app.test", "application/json; charset=utf-8", 200},
		{"https://evil.test", "application/json", 403},
		{"", "text/plain", 400}, // what a cross-site form can send
		{"", "application/x-www-form-urlencoded", 400},
	} {
		if got := post(tc.origin, tc.ct); got != tc.want {
			t.Errorf("Origin %q, Content-Type %q: %d, want %d", tc.origin, tc.ct, got, tc.want)
		}
	}
	// A GET isn't served.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "http://app.test"+add.Path(), nil))
	if w.Code != 405 {
		t.Fatalf("GET: %d, want 405", w.Code)
	}
}

func TestBeforeHook(t *testing.T) {
	mux, add := newServer(t, Before(func(r *http.Request) (context.Context, error) {
		user := r.Header.Get("X-User")
		if user == "" {
			return nil, Errorf(CodeUnauthorized, "sign in first")
		}
		return context.WithValue(r.Context(), userKey{}, "@"+user), nil
	}))
	call := func(user string) (todo, int) {
		r := httptest.NewRequest("POST", "http://app.test"+add.Path(), bytes.NewReader([]byte(`{"title":"x"}`)))
		r.Header.Set("Content-Type", "application/json")
		if user != "" {
			r.Header.Set("X-User", user)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var out todo
		_ = jsonUnmarshal(w.Body.Bytes(), &out)
		return out, w.Code
	}
	if _, code := call(""); code != 401 {
		t.Fatalf("no user: %d, want 401", code)
	}
	if out, code := call("ada"); code != 200 || out.Title != "x@ada" {
		t.Fatalf("with a user: %d %+v", code, out)
	}
}

// In the server's process a call runs the handler directly (server
// rendering, the server's own code).
func TestInProcessCall(t *testing.T) {
	_, add := newServer(t)
	got, err := add.Call(context.WithValue(context.Background(), userKey{}, "!"), addReq{Title: "eggs"})
	if err != nil || got.Title != "eggs!" {
		t.Fatalf("in-process: %+v %v", got, err)
	}
	var e *Error
	if _, err := add.Call(context.Background(), addReq{Title: "boom"}); !errors.As(err, &e) || e.Code != CodeInternal {
		t.Fatalf("in-process plain error: %#v", err)
	}
}

func TestFake(t *testing.T) {
	add := Define[addReq, todo]("test.fake")
	t.Run("stubbed", func(t *testing.T) {
		Fake(t, add, func(ctx context.Context, req addReq) (todo, error) { return todo{1, "fake " + req.Title}, nil })
		if got, err := add.Call(context.Background(), addReq{Title: "a"}); err != nil || got.Title != "fake a" {
			t.Fatalf("%+v %v", got, err)
		}
	})
	if local("test.fake") != nil {
		t.Fatal("the fake outlived its test")
	}
}

func TestDefineRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "a/b", "a b", "a?b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Define(%q) didn't panic", name)
				}
			}()
			Define[int, int](name)
		}()
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
