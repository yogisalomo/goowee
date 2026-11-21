// Package rpc defines typed server functions: one Go definition, shared by
// the server and the WASM client, names a function and its request and
// response types, so the two sides can't disagree without a compile error.
//
//	// shared package: builds for the host and for wasm
//	var AddTodo = rpc.Define[AddTodoReq, Todo]("todos.add")
//
//	// server
//	rpc.Handle(mux, AddTodo, func(ctx context.Context, req AddTodoReq) (Todo, error) { … })
//
//	// client, in a goroutine (Call blocks) or through UseQuery
//	todo, err := AddTodo.Call(ctx, AddTodoReq{Title: t})
//
// A call is a POST of the request as JSON to BasePath + "/" + name, made with
// goowee/net, so gooweetest.NewFakeNet works in component tests and Fake
// stubs one function. Errors travel as *Error values with a code, a message
// and per-field messages for forms. On the server, a call made while the
// function's handler is registered in the same process (server rendering)
// runs the handler directly, without HTTP.
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/yogisalomo/goowee/net"
)

// BasePath is the URL path every function is served under: a function named
// "todos.add" is at BasePath + "/todos.add". Set it once, before Handle and
// the first Call, on both sides.
var BasePath = "/_rpc"

// Func is a server function taking Req and returning Res, made with Define.
type Func[Req, Res any] struct {
	name string
}

// Define declares a server function. Put definitions in a package both the
// server and the client import. name must be unique; it becomes part of the
// URL, so keep it to letters, digits, '.', '-' and '_'.
func Define[Req, Res any](name string) *Func[Req, Res] {
	if name == "" || strings.ContainsAny(name, "/?#% ") {
		panic(fmt.Sprintf("rpc.Define(%q): use letters, digits, '.', '-' and '_'", name))
	}
	return &Func[Req, Res]{name: name}
}

// Name is the function's name.
func (f *Func[Req, Res]) Name() string { return f.name }

// Path is the URL path the function is served at.
func (f *Func[Req, Res]) Path() string { return strings.TrimSuffix(BasePath, "/") + "/" + f.name }

// Call invokes the function and returns its result. In the browser it
// fetches; in a process where the handler is registered (Handle) or stubbed
// (Fake), it calls it directly. Call blocks: run it in a goroutine, a
// UseResource fetch, or UseQuery, never on the render loop.
//
// A failure is an *Error: the handler's own *Error, CodeInternal for any
// other handler error, or CodeNetwork when no response arrived. ctx ending
// aborts the request and returns ctx.Err().
func (f *Func[Req, Res]) Call(ctx context.Context, req Req) (Res, error) {
	var zero Res
	if h := local(f.name); h != nil {
		out, err := h(ctx, req)
		if err != nil {
			return zero, asError(err)
		}
		return out.(Res), nil
	}
	resp, err := net.Fetch(ctx, net.Request{Method: "POST", URL: f.Path(), JSON: req,
		Header: map[string]string{"Accept": "application/json"}})
	if err != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, &Error{Code: CodeNetwork, Message: err.Error()}
	}
	if !resp.OK() {
		return zero, decodeError(resp)
	}
	var out Res
	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &out); err != nil {
			return zero, &Error{Code: CodeInternal, Message: "rpc: decoding the response of " + f.name + ": " + err.Error()}
		}
	}
	return out, nil
}

// handler is a registered function with its types erased, so calls in the
// same process can find it by name.
type handler func(ctx context.Context, req any) (any, error)

var (
	mu       sync.RWMutex
	handlers = map[string]handler{}
	fakes    = map[string]handler{}
)

func local(name string) handler {
	mu.RLock()
	defer mu.RUnlock()
	if h, ok := fakes[name]; ok {
		return h
	}
	return handlers[name]
}

// Error is a server function's failure, as the client receives it. Code is
// one of the Code constants (or the app's own string), Message is for people,
// and Fields maps form field names to messages for a form to show (#150).
type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	// Status is the HTTP status the error was sent with (0 for a call that
	// never left the process).
	Status int `json:"-"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return "rpc: " + e.Code
	}
	return e.Message
}

// Error codes with a meaning the server and client agree on. Each maps to an
// HTTP status on the wire.
const (
	CodeInvalid      = "invalid"      // 400: the request doesn't validate; see Fields
	CodeUnauthorized = "unauthorized" // 401
	CodeForbidden    = "forbidden"    // 403
	CodeNotFound     = "not_found"    // 404
	CodeConflict     = "conflict"     // 409
	CodeInternal     = "internal"     // 500: any error that isn't an *Error
	CodeNetwork      = "network"      // no response arrived (client side only)
)

// Invalid returns a CodeInvalid error with per-field messages.
func Invalid(message string, fields map[string]string) *Error {
	return &Error{Code: CodeInvalid, Message: message, Fields: fields}
}

// Errorf returns an error with code and a formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func statusOf(code string) int {
	switch code {
	case CodeInvalid:
		return 400
	case CodeUnauthorized:
		return 401
	case CodeForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeConflict:
		return 409
	case CodeInternal, "":
		return 500
	}
	return 422 // an app-defined code: the request was understood but refused
}

// asError makes any handler error an *Error. A plain error's text stays on
// the server (it may hold internals): the caller gets CodeInternal.
func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeInternal, Message: "internal error"}
}

func decodeError(resp net.Response) *Error {
	var body struct {
		Error *Error `json:"error"`
	}
	if json.Unmarshal(resp.Body, &body) == nil && body.Error != nil {
		body.Error.Status = resp.Status
		return body.Error
	}
	return &Error{Code: CodeInternal, Message: fmt.Sprintf("rpc: HTTP %d", resp.Status), Status: resp.Status}
}
