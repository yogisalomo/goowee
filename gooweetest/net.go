package gooweetest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

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
// call Screen.Flush (or WaitFor) after sending to see their effect.
type FakeNet struct {
	t testing.TB

	mu       sync.Mutex
	routes   []fakeRoute
	requests []net.Request
	sockets  []*FakeSocket
}

type fakeRoute struct {
	method, path string // method "" matches any
	fn           func(net.Request) (net.Response, error)
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

// Handle answers requests matching pattern with fn. The pattern is
// "METHOD /path" or "/path" (any method); the path must equal the request
// URL's path, query excluded (fn can read the query from Request.URL). When
// several patterns match, the latest registered wins, so a test can change an
// answer midway. A request that no pattern matches gets a 404. fn runs on the
// app's fetching goroutine; an error it returns is what net.Fetch returns,
// standing in for a network failure.
func (n *FakeNet) Handle(pattern string, fn func(net.Request) (net.Response, error)) {
	route := fakeRoute{path: pattern, fn: fn}
	if method, path, ok := strings.Cut(pattern, " "); ok {
		route.method, route.path = method, strings.TrimSpace(path)
	}
	n.mu.Lock()
	n.routes = append(n.routes, route)
	n.mu.Unlock()
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
	path := urlPath(req.URL)
	n.mu.Lock()
	n.requests = append(n.requests, req)
	var fn func(net.Request) (net.Response, error)
	for i := len(n.routes) - 1; i >= 0; i-- {
		if r := n.routes[i]; r.path == path && (r.method == "" || r.method == req.Method) {
			fn = r.fn
			break
		}
	}
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
