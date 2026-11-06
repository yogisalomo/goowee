// Package net is goowee's browser networking: Fetch for HTTP requests and Dial
// for a WebSocket, built on the browser's own fetch and WebSocket. It costs the
// WASM binary a few kilobytes, where net/http costs about 7 MB (and has no
// WebSocket client).
//
// Fetch blocks until the response has arrived, so call it from a goroutine;
// hooks.UseResource's fetch function runs in one, and its context aborts the
// request when the result is no longer wanted:
//
//	user := hooks.UseResource(nil, func(ctx context.Context) (User, error) {
//	    var u User
//	    resp, err := net.Fetch(ctx, net.Request{URL: "/api/user"})
//	    if err == nil {
//	        err = resp.JSON(&u)
//	    }
//	    return u, err
//	})
//
// A Socket's callbacks run on the render loop, so they read and set signals
// directly, with no core.Schedule:
//
//	hooks.OnMount(func() func() {
//	    sock := net.Dial("/ws", net.SocketOptions{
//	        OnMessage: func(m net.Message) { setLast(m.Text()) },
//	        Reconnect: true,
//	    })
//	    return sock.Close
//	})
//
// Outside the browser (SSR, native tests) there is no network: Fetch returns
// ErrUnavailable and sockets never open. Component tests install an in-memory
// fake with gooweetest.NewFakeNet; SetTransport installs any other Transport.
package net

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/yogisalomo/goowee/core"
)

// ErrUnavailable is returned by Fetch, and reported by a Socket that cannot
// connect, when there is no network transport: outside the browser, unless a
// Transport is installed.
var ErrUnavailable = errors.New("goowee/net: no network here: Fetch and Dial use the browser's fetch and WebSocket (in component tests, install gooweetest.NewFakeNet)")

// ErrNotOpen is returned by Socket.Send while the socket isn't connected.
var ErrNotOpen = errors.New("goowee/net: socket is not open")

// Request is an HTTP request for Fetch.
type Request struct {
	// Method is the HTTP method; "" means GET.
	Method string
	// URL is the resource, absolute or relative to the page.
	URL string
	// Header holds the request headers.
	Header map[string]string
	// Body is the request body. For a JSON body, set JSON instead.
	Body []byte
	// JSON, when non-nil, is encoded with encoding/json as the body, and
	// Content-Type defaults to application/json.
	JSON any
	// Form, when non-nil, is sent as a multipart/form-data body (the
	// browser's FormData), e.g. to upload a picked or pasted file; the browser
	// sets Content-Type with the boundary. Set one of Body, JSON and Form.
	Form *Form
	// Credentials is the fetch credentials mode: "same-origin" (the browser's
	// default, used when empty), "include" (send cookies cross-origin too) or
	// "omit".
	Credentials string
}

// Form is a multipart/form-data body: text fields and files, in order. In the
// browser each file is appended as the browser's own File object, so its
// contents go to the network without being copied into WebAssembly memory:
//
//	OnChangeE(func(e core.EventData) {
//	    form := new(net.Form).Add("caption", caption.Get())
//	    for _, f := range e.Files() {
//	        form.AddFile("photo", f)
//	    }
//	    go func() {
//	        resp, err := net.Fetch(ctx, net.Request{Method: "POST", URL: "/api/photos", Form: form})
//	        core.Schedule(func() { /* show resp/err */ })
//	    }()
//	})
//
// A file stays sendable as long as File.Bytes could read it: until its input's
// selection changes, the next paste or drop replaces it, or its element is
// removed.
type Form struct {
	Fields []FormField
}

// FormField is one field of a Form: a text Value, or a File.
type FormField struct {
	Name  string
	Value string
	File  *core.File
}

// Add appends a text field and returns f, for chaining.
func (f *Form) Add(name, value string) *Form {
	f.Fields = append(f.Fields, FormField{Name: name, Value: value})
	return f
}

// AddFile appends a file field (sent with the file's name) and returns f.
func (f *Form) AddFile(name string, file core.File) *Form {
	f.Fields = append(f.Fields, FormField{Name: name, File: &file})
	return f
}

// Value returns the first text field called name ("" if none).
func (f *Form) Value(name string) string {
	for _, fd := range f.Fields {
		if fd.Name == name && fd.File == nil {
			return fd.Value
		}
	}
	return ""
}

// File returns the first file field called name.
func (f *Form) File(name string) (core.File, bool) {
	for _, fd := range f.Fields {
		if fd.Name == name && fd.File != nil {
			return *fd.File, true
		}
	}
	return core.File{}, false
}

// Response is an HTTP response. Fetch returns one for every status the server
// answers with, so check OK (or use JSON, which does) to treat a 404 or a 500
// as an error.
type Response struct {
	Status     int
	StatusText string
	// URL is the response's URL, after any redirects.
	URL string
	// Header holds the response headers the page can read, with lowercase
	// names; a repeated header's values are joined with ", ".
	Header map[string]string
	Body   []byte
}

// OK reports whether the status is 2xx.
func (r Response) OK() bool { return r.Status >= 200 && r.Status <= 299 }

// Text returns the body as a string.
func (r Response) Text() string { return string(r.Body) }

// JSON decodes the body into v. For a response that isn't OK it decodes
// nothing and returns a *StatusError, so `return v, resp.JSON(&v)` treats an
// error status as an error. (To read an error response's JSON body,
// json.Unmarshal its Body.)
func (r Response) JSON(v any) error {
	if !r.OK() {
		return &StatusError{Status: r.Status, StatusText: r.StatusText, URL: r.URL, Body: r.Body}
	}
	return json.Unmarshal(r.Body, v)
}

// StatusError is the error Response.JSON returns for a status that isn't 2xx.
type StatusError struct {
	Status     int
	StatusText string
	URL        string
	Body       []byte
}

func (e *StatusError) Error() string {
	msg := "goowee/net: "
	if e.URL != "" {
		msg += e.URL + ": "
	}
	msg += strconv.Itoa(e.Status)
	if e.StatusText != "" {
		msg += " " + e.StatusText
	}
	return msg
}

// Fetch sends req and returns the response once its body has arrived. The
// error is non-nil only when no response arrived: a network failure, a
// request the browser refused (CORS, say), or ctx ending first, which aborts
// the request and returns ctx.Err(). An error status is a Response; see
// Response.OK.
//
// Fetch blocks, so call it from a goroutine (UseResource's fetch runs in one),
// never directly in an event handler, an effect or another render-loop
// callback: waiting there would stall the WASM event loop the response needs.
// Apply the result with core.Schedule, as with any off-loop work.
func Fetch(ctx context.Context, req Request) (Response, error) {
	if req.Form != nil {
		if req.Body != nil || req.JSON != nil {
			return Response{}, errors.New("goowee/net: Request sets Form together with Body or JSON")
		}
		if hasHeader(req.Header, "Content-Type") {
			return Response{}, errors.New("goowee/net: Request.Form sets its own Content-Type (with the multipart boundary); leave it out")
		}
	}
	if req.JSON != nil {
		if req.Body != nil {
			return Response{}, errors.New("goowee/net: Request sets both Body and JSON")
		}
		body, err := json.Marshal(req.JSON)
		if err != nil {
			return Response{}, err
		}
		req.Body, req.JSON = body, nil
		if !hasHeader(req.Header, "Content-Type") {
			header := make(map[string]string, len(req.Header)+1)
			for k, v := range req.Header {
				header[k] = v
			}
			header["Content-Type"] = "application/json"
			req.Header = header
		}
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	return current().Fetch(ctx, req)
}

func hasHeader(h map[string]string, name string) bool {
	for k := range h {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

// Transport carries Fetch requests and Socket connections. In a WASM build the
// default is the browser's fetch and WebSocket; elsewhere it fails with
// ErrUnavailable. gooweetest.NewFakeNet installs an in-memory fake for tests;
// another host (a test of a client package, a non-browser shell) can install
// its own with SetTransport.
type Transport interface {
	// Fetch performs req, whose Method is set and whose JSON is already
	// encoded into Body, honoring ctx. A Form's files carry the Handle of the
	// browser's File objects; read their contents with File.Bytes.
	Fetch(ctx context.Context, req Request) (Response, error)
	// Dial starts opening a WebSocket to url and returns without waiting. It
	// reports the connection through events, from any goroutine: Open once
	// connected, Message for each message, then Close exactly once when the
	// connection ends or fails to open. An error means no connection was
	// started (and no event follows).
	Dial(url string, protocols []string, events ConnEvents) (Conn, error)
}

// ConnEvents are a connection's callbacks to its Socket, which moves them onto
// the render loop.
type ConnEvents struct {
	Open    func()
	Message func(Message)
	Close   func(CloseEvent)
}

// Conn is one WebSocket connection made by a Transport.
type Conn interface {
	// Send sends a message; it fails when the connection isn't open.
	Send(m Message) error
	// Close starts closing the connection with a close code (1000 or
	// 3000–4999) and reason; the Close event follows.
	Close(code int, reason string)
}

// Message is one WebSocket message.
type Message struct {
	Data []byte
	// Binary reports a binary message; otherwise Data is a text message's
	// UTF-8.
	Binary bool
}

// Text returns the message as a string.
func (m Message) Text() string { return string(m.Data) }

// CloseEvent describes how a socket's connection ended.
type CloseEvent struct {
	// Code is the close code: 1000 for a normal close, 1006 when the
	// connection dropped or never opened.
	Code   int
	Reason string
	// Reconnecting reports that the Socket will connect again
	// (SocketOptions.Reconnect).
	Reconnecting bool
}

var (
	transportMu sync.Mutex
	transport   Transport
)

// SetTransport installs t for Fetch and Dial and returns the transport it
// replaces; nil restores the default. Sockets keep the transport they dialed
// with until they reconnect.
func SetTransport(t Transport) (previous Transport) {
	transportMu.Lock()
	defer transportMu.Unlock()
	previous = transport
	if previous == nil {
		previous = platform
	}
	transport = t
	return previous
}

func current() Transport {
	transportMu.Lock()
	defer transportMu.Unlock()
	if transport == nil {
		return platform
	}
	return transport
}
