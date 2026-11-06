package gooweetest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/net"
)

// FakeNet is an in-memory goowee/net transport for component tests. It
// answers the app's net.Fetch calls from handlers the test registers, records
// every request, and gives the test the server end of every socket the app
// dials:
//
//	n := gooweetest.NewFakeNet(t)
//	n.Handle("GET /api/user", func(r net.Request) (net.Response, error) {
//	    return gooweetest.JSONResponse(200, User{Name: "Ada"})
//	})
//	s := gooweetest.Render(t, Profile())
//	s.WaitForText("Ada")
//
//	ws := n.Socket("/ws")        // the socket Profile dialed
//	ws.Send(`{"status":"away"}`) // Profile's OnMessage runs on the next flush
//	s.WaitForText("away")
//
// Socket callbacks run on the Screen's render loop, like in the browser, so
// call Screen.Flush (or WaitFor) after sending to see their effect. For a
// socket the app dials later (after a load, say), use Screen.WaitForSocket.
// Event streams (net.Subscribe) work the same way: Stream, WaitForStream.
type FakeNet struct {
	t testing.TB

	mu       sync.Mutex
	routes   map[string]func(net.Request) (net.Response, error) // by pattern
	mux      *http.ServeMux                                     // matches requests to routes' patterns
	requests []net.Request
	sockets  []*FakeSocket
	streams  []*FakeStream
	delay    *time.Duration // SetReconnectDelay; nil keeps the sockets' own backoff
}

// NewFakeNet installs a FakeNet as goowee/net's transport until the test ends.
// Install it before Render, so requests made at mount reach it. Like Render,
// it changes process-wide state: don't use it in parallel tests.
func NewFakeNet(t testing.TB) *FakeNet {
	t.Helper()
	n := &FakeNet{t: t}
	prev := net.SetTransport(n)
	t.Cleanup(func() { net.SetTransport(prev) })
	return n
}

// Handle answers requests matching pattern with fn. Patterns follow
// net/http.ServeMux: "GET /api/user" (a method and an exact path),
// "/api/user" (any method), "POST /api/" (everything under /api/),
// "/rooms/{id}/messages" (a wildcard segment), and "/" alone for every request
// no other pattern matches, e.g. tests that only check what the app sent:
//
//	n.Handle("/", gooweetest.Status(204))
//
// The most specific pattern wins; registering a pattern again replaces its
// handler, so a test can change an answer midway. A request no pattern matches
// gets a 404 (and a t.Logf naming it). fn runs on the app's fetching
// goroutine; an error it returns is what net.Fetch returns, standing in for a
// network failure. Patterns ServeMux would reject, or that conflict, fail the
// test.
func (n *FakeNet) Handle(pattern string, fn func(net.Request) (net.Response, error)) {
	n.t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	routes := map[string]func(net.Request) (net.Response, error){pattern: fn}
	for p, f := range n.routes {
		if p != pattern {
			routes[p] = f
		}
	}
	mux := http.NewServeMux()
	if err := register(mux, routes); err != nil {
		n.t.Fatalf("gooweetest: FakeNet.Handle(%q): %v", pattern, err)
	}
	n.routes, n.mux = routes, mux
}

// register adds every pattern to mux, turning ServeMux's panic on a bad or
// conflicting pattern into an error.
func register(mux *http.ServeMux, routes map[string]func(net.Request) (net.Response, error)) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	for p := range routes {
		mux.Handle(p, http.NotFoundHandler())
	}
	return nil
}

// Status is a handler answering every request with an empty response of the
// given status: n.Handle("POST /api/", gooweetest.Status(204)).
func Status(code int) func(net.Request) (net.Response, error) {
	return func(net.Request) (net.Response, error) { return net.Response{Status: code}, nil }
}

// route finds the handler for req: the most specific matching pattern.
func (n *FakeNet) route(req net.Request) func(net.Request) (net.Response, error) {
	if n.mux == nil {
		return nil
	}
	r, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return nil
	}
	if _, pattern := n.mux.Handler(r); pattern != "" {
		return n.routes[pattern]
	}
	return nil
}

// JSONResponse is a response with status and v encoded as its JSON body, in
// the form a Handle function returns.
func JSONResponse(status int, v any) (net.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return net.Response{}, err
	}
	return net.Response{Status: status, Header: map[string]string{"content-type": "application/json"}, Body: body}, nil
}

// Requests returns the requests the app has made, in order, with Method set
// and a JSON body already encoded into Body.
func (n *FakeNet) Requests() []net.Request {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]net.Request(nil), n.requests...)
}

// Fetch implements net.Transport.
func (n *FakeNet) Fetch(ctx context.Context, req net.Request) (net.Response, error) {
	n.mu.Lock()
	n.requests = append(n.requests, req)
	fn := n.route(req)
	n.mu.Unlock()
	if fn == nil {
		n.t.Logf("gooweetest: FakeNet has no handler for %s %s; answering 404", req.Method, req.URL)
		return net.Response{Status: 404, StatusText: "Not Found", URL: req.URL}, nil
	}
	// Like the browser, give up when ctx ends even if fn is still working.
	type result struct {
		resp net.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := fn(req)
		done <- result{resp, err}
	}()
	select {
	case <-ctx.Done():
		return net.Response{}, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return net.Response{}, r.err
		}
		if r.resp.Status == 0 {
			r.resp.Status = 200
		}
		if r.resp.URL == "" {
			r.resp.URL = req.URL
		}
		return r.resp, nil
	}
}

// Dial implements net.Transport. The connection opens at once.
func (n *FakeNet) Dial(url string, protocols []string, ev net.ConnEvents) (net.Conn, error) {
	s := &FakeSocket{URL: url, Protocols: protocols, n: n, ev: ev}
	n.mu.Lock()
	n.sockets = append(n.sockets, s)
	n.mu.Unlock()
	ev.Open()
	return fakeConn{s}, nil
}

// Socket returns the most recent socket the app dialed whose URL path is
// path (query excluded; check FakeSocket.URL for it). It fails the test when
// there is none.
func (n *FakeNet) Socket(path string) *FakeSocket {
	n.t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.sockets) - 1; i >= 0; i-- {
		if urlPath(n.sockets[i].URL) == path {
			return n.sockets[i]
		}
	}
	n.t.Fatalf("gooweetest: the app dialed no socket to %s (sockets: %d)", path, len(n.sockets))
	return nil
}

// SetReconnectDelay makes sockets dialed through n reconnect after d instead
// of their SocketOptions backoff, so a test of reconnection runs quickly; use
// Screen.WaitForSocket for the new socket. (A Socket reconnects only with
// SocketOptions.Reconnect set.)
func (n *FakeNet) SetReconnectDelay(d time.Duration) {
	n.mu.Lock()
	n.delay = &d
	n.mu.Unlock()
}

// ReconnectDelay implements net.ReconnectDelayer.
func (n *FakeNet) ReconnectDelay(planned time.Duration) time.Duration {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.delay != nil {
		return *n.delay
	}
	return planned
}

// openSocket is the most recent socket to path that is still open, or nil.
func (n *FakeNet) openSocket(path string) *FakeSocket {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.sockets) - 1; i >= 0; i-- {
		if s := n.sockets[i]; urlPath(s.URL) == path && !s.Closed() {
			return s
		}
	}
	return nil
}

// WaitForSocket flushes until the app has an open socket to path (query
// excluded) through n, and returns the most recent one: for apps that dial
// after an async step, and for the new socket after a reconnection (Close the
// old one first). It fails the test after timeout (default 2s).
//
//	ws := s.WaitForSocket(n, "/ws")
//	ws.SendJSON(frame)
func (s *Screen) WaitForSocket(n *FakeNet, path string, timeout ...time.Duration) *FakeSocket {
	s.t.Helper()
	limit := 2 * time.Second
	if len(timeout) > 0 {
		limit = timeout[0]
	}
	for deadline := time.Now().Add(limit); ; time.Sleep(time.Millisecond) {
		s.Flush()
		if ws := n.openSocket(path); ws != nil {
			return ws
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("gooweetest: no open socket to %s within %v (%d dialed in all)", path, limit, len(n.Sockets()))
			return nil
		}
	}
}

// Sockets returns every socket the app has dialed, oldest first.
func (n *FakeNet) Sockets() []*FakeSocket {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]*FakeSocket(nil), n.sockets...)
}

// urlPath is u's path: scheme, host, query and fragment removed.
func urlPath(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
		if j := strings.IndexByte(u, '/'); j >= 0 {
			u = u[j:]
		} else {
			u = "/"
		}
	}
	return u
}

// FakeSocket is the server end of a socket the app dialed through a FakeNet.
type FakeSocket struct {
	// URL and Protocols are what the app dialed with.
	URL       string
	Protocols []string

	n  *FakeNet
	ev net.ConnEvents

	mu       sync.Mutex
	received []net.Message
	closed   bool
}

// Send sends the app a text message; its OnMessage runs on the next flush.
func (s *FakeSocket) Send(text string) { s.deliver(net.Message{Data: []byte(text)}) }

// SendBinary sends the app a binary message.
func (s *FakeSocket) SendBinary(data []byte) {
	s.deliver(net.Message{Data: append([]byte(nil), data...), Binary: true})
}

// SendJSON sends the app v encoded as a JSON text message.
func (s *FakeSocket) SendJSON(v any) {
	s.n.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		s.n.t.Fatalf("gooweetest: FakeSocket.SendJSON: %v", err)
	}
	s.deliver(net.Message{Data: data})
}

func (s *FakeSocket) deliver(m net.Message) {
	s.n.t.Helper()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		s.n.t.Fatalf("gooweetest: sending on the closed socket to %s", s.URL)
	}
	s.ev.Message(m)
}

// Received returns the messages the app sent on this socket, in order.
func (s *FakeSocket) Received() []net.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]net.Message(nil), s.received...)
}

// Close closes the connection from the server's side, as a server restart or
// a dropped network would: the app's OnClose runs on the next flush, and a
// Socket with Reconnect dials again after its backoff (a new FakeSocket).
func (s *FakeSocket) Close(code int, reason string) {
	if s.close() {
		s.ev.Close(net.CloseEvent{Code: code, Reason: reason})
	}
}

// Closed reports whether the connection is closed, by either side.
func (s *FakeSocket) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// close marks the socket closed, reporting whether it was open.
func (s *FakeSocket) close() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	return true
}

// fakeConn is the app's end of a FakeSocket.
type fakeConn struct{ s *FakeSocket }

func (c fakeConn) Send(m net.Message) error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if c.s.closed {
		return net.ErrNotOpen
	}
	c.s.received = append(c.s.received, net.Message{Data: append([]byte(nil), m.Data...), Binary: m.Binary})
	return nil
}

func (c fakeConn) Close(code int, reason string) {
	if c.s.close() {
		c.s.ev.Close(net.CloseEvent{Code: code, Reason: reason})
	}
}

// Subscribe implements net.StreamTransport. The stream opens at once.
func (n *FakeNet) Subscribe(url string, events []string, withCredentials bool, ev net.StreamEvents) (net.Stream, error) {
	s := &FakeStream{URL: url, Events: events, WithCredentials: withCredentials, n: n, ev: ev}
	n.mu.Lock()
	n.streams = append(n.streams, s)
	n.mu.Unlock()
	ev.Open()
	return fakeStreamConn{s}, nil
}

// Stream returns the most recent event stream the app subscribed to whose URL
// path is path (query excluded). It fails the test when there is none; for a
// subscription made after an async step, use Screen.WaitForStream.
func (n *FakeNet) Stream(path string) *FakeStream {
	n.t.Helper()
	if s := n.openStream(path, true); s != nil {
		return s
	}
	n.t.Fatalf("gooweetest: the app subscribed to no event stream at %s", path)
	return nil
}

func (n *FakeNet) openStream(path string, anyState bool) *FakeStream {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.streams) - 1; i >= 0; i-- {
		if s := n.streams[i]; urlPath(s.URL) == path && (anyState || !s.Closed()) {
			return s
		}
	}
	return nil
}

// WaitForStream flushes until the app has an open event stream at path
// (query excluded) through n, and returns the most recent one. It fails the
// test after timeout (default 2s).
func (s *Screen) WaitForStream(n *FakeNet, path string, timeout ...time.Duration) *FakeStream {
	s.t.Helper()
	limit := 2 * time.Second
	if len(timeout) > 0 {
		limit = timeout[0]
	}
	for deadline := time.Now().Add(limit); ; time.Sleep(time.Millisecond) {
		s.Flush()
		if st := n.openStream(path, false); st != nil {
			return st
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("gooweetest: no open event stream at %s within %v", path, limit)
			return nil
		}
	}
}

// FakeStream is the server end of an event stream the app opened with
// net.Subscribe through a FakeNet.
type FakeStream struct {
	// URL, Events and WithCredentials are what the app subscribed with.
	URL             string
	Events          []string
	WithCredentials bool

	n  *FakeNet
	ev net.StreamEvents

	mu     sync.Mutex
	closed bool
}

// Send sends the app an unnamed ("message") event; its OnEvent runs on the
// next flush.
func (s *FakeStream) Send(data string) { s.SendEvent(net.Event{Data: data}) }

// SendEvent sends the app an event. Like the browser, a named event reaches
// the app only if it subscribed to that type (SubscribeOptions.Events).
func (s *FakeStream) SendEvent(e net.Event) {
	s.n.t.Helper()
	if e.Type == "" {
		e.Type = "message"
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		s.n.t.Fatalf("gooweetest: sending on the closed event stream at %s", s.URL)
	}
	if e.Type != "message" && !slices.Contains(s.Events, e.Type) {
		return
	}
	s.ev.Event(e)
}

// Fail drops the connection, as a server restart or a lost network would: the
// app's OnError runs with reconnecting. When reconnecting, the stream stays
// usable and Reopen reports the browser's successful reconnection; otherwise
// it is closed, as the browser does after an HTTP error.
func (s *FakeStream) Fail(reconnecting bool) {
	if !reconnecting {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
	}
	s.ev.Error(reconnecting)
}

// Reopen reports a reconnection after Fail(true): the app's OnOpen runs again.
func (s *FakeStream) Reopen() { s.ev.Open() }

// Closed reports whether the stream is closed, by the app or by Fail(false).
func (s *FakeStream) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// fakeStreamConn is the app's end of a FakeStream.
type fakeStreamConn struct{ s *FakeStream }

func (c fakeStreamConn) Close() {
	c.s.mu.Lock()
	c.s.closed = true
	c.s.mu.Unlock()
}
