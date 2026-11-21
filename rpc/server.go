//go:build !(js && wasm)

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
)

// Option configures Handle.
type Option func(*options)

type options struct {
	before    []func(r *http.Request) (context.Context, error)
	crossSite bool
	maxBody   int64
}

// Before runs check before the handler on each HTTP call: authentication,
// say. A non-nil error ends the call with it (an *Error keeps its code, so
// rpc.Errorf(rpc.CodeUnauthorized, …) answers 401). check may return a
// context for the handler, carrying the user it found; nil keeps the
// request's. In-process calls (server rendering) skip it: the page's own
// handler has already authenticated the request.
func Before(check func(r *http.Request) (context.Context, error)) Option {
	return func(o *options) { o.before = append(o.before, check) }
}

// AllowCrossSite turns off the same-origin check, for a function called from
// another origin (with your own CORS headers and auth).
func AllowCrossSite() Option { return func(o *options) { o.crossSite = true } }

// MaxBody limits the request body (default 1 MiB).
func MaxBody(n int64) Option { return func(o *options) { o.maxBody = n } }

// Handle serves f on mux at f.Path() (POST only) and registers fn for calls
// made in this process, so a server render that calls f runs fn directly.
//
// By default a call must be same-origin: its Origin header, when present,
// must match the request's host, and its Content-Type must be
// application/json, which a cross-site HTML form can't send. That blocks
// cross-site request forgery for cookie-authenticated apps; AllowCrossSite
// turns it off. A handler error that isn't an *Error is logged and answered
// as CodeInternal, without its text.
func Handle[Req, Res any](mux *http.ServeMux, f *Func[Req, Res], fn func(ctx context.Context, req Req) (Res, error), opts ...Option) {
	o := options{maxBody: 1 << 20}
	for _, opt := range opts {
		opt(&o)
	}
	mu.Lock()
	handlers[f.name] = func(ctx context.Context, req any) (any, error) { return fn(ctx, req.(Req)) }
	mu.Unlock()

	mux.HandleFunc("POST "+f.Path(), func(w http.ResponseWriter, r *http.Request) {
		if !o.crossSite {
			if !sameOrigin(r) {
				writeError(w, &Error{Code: CodeForbidden, Message: "rpc: cross-site call refused"})
				return
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" && !hasPrefix(ct, "application/json;") {
				writeError(w, &Error{Code: CodeInvalid, Message: "rpc: Content-Type must be application/json"})
				return
			}
		}
		ctx := r.Context()
		for _, check := range o.before {
			c, err := check(r)
			if err != nil {
				writeError(w, asLoggedError(f.name, err))
				return
			}
			if c != nil {
				ctx = c
			}
		}
		var req Req
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, o.maxBody))
		if err != nil {
			writeError(w, &Error{Code: CodeInvalid, Message: "rpc: request body too large or unreadable"})
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeError(w, &Error{Code: CodeInvalid, Message: "rpc: request isn't valid JSON for " + f.name + ": " + err.Error()})
				return
			}
		}
		res, err := fn(ctx, req)
		if err != nil {
			writeError(w, asLoggedError(f.name, err))
			return
		}
		data, err := json.Marshal(res)
		if err != nil {
			writeError(w, asLoggedError(f.name, err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	})
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

// sameOrigin reports whether r comes from the page's own origin. A request
// without Origin (same-origin GETs, older browsers, non-browser clients) is
// let through; the Content-Type check still stops forms.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func asLoggedError(name string, err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	log.Printf("rpc %s: %v", name, err)
	return &Error{Code: CodeInternal, Message: "internal error"}
}

func writeError(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusOf(e.Code))
	data, _ := json.Marshal(struct {
		Error *Error `json:"error"`
	}{e})
	w.Write(data)
}
