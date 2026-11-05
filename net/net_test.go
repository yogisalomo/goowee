package net

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
)

// scripted is a Transport the test drives by hand.
type scripted struct {
	mu    sync.Mutex
	fetch func(context.Context, Request) (Response, error)
	dial  func(url string) error // optional: fail a dial
	conns []*scriptedConn
}

type scriptedConn struct {
	url       string
	protocols []string
	ev        ConnEvents

	mu        sync.Mutex
	sent      []Message
	closeCode int // 0 while open
}

func (s *scripted) Fetch(ctx context.Context, req Request) (Response, error) {
	return s.fetch(ctx, req)
}

func (s *scripted) Dial(url string, protocols []string, ev ConnEvents) (Conn, error) {
	if s.dial != nil {
		if err := s.dial(url); err != nil {
			return nil, err
		}
	}
	c := &scriptedConn{url: url, protocols: protocols, ev: ev}
	s.mu.Lock()
	s.conns = append(s.conns, c)
	s.mu.Unlock()
	return c, nil
}

func (s *scripted) conn(i int) *scriptedConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.conns) {
		return nil
	}
	return s.conns[i]
}

func (s *scripted) dials() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (c *scriptedConn) Send(m Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

func (c *scriptedConn) Close(code int, reason string) {
	c.mu.Lock()
	c.closeCode = code
	c.mu.Unlock()
}

// install makes tr the transport and an active scheduler the render loop for
// the rest of the test; flush runs what the transport's events posted.
func install(t *testing.T, tr Transport) (flush func()) {
	t.Helper()
	sched := core.NewScheduler()
	core.SetActiveScheduler(sched)
	prev := SetTransport(tr)
	t.Cleanup(func() {
		SetTransport(prev)
		core.SetActiveScheduler(nil)
	})
	return func() { sched.Flush() }
}

// eventually flushes until cond holds, for work that waits on a timer.
func eventually(t *testing.T, flush func(), cond func() bool, what string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		flush()
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestFetchNormalizesRequest(t *testing.T) {
	var got Request
	tr := &scripted{fetch: func(_ context.Context, req Request) (Response, error) {
		got = req
		return Response{Status: 200}, nil
	}}
	install(t, tr)
	header := map[string]string{"X-Test": "1"}
	if _, err := Fetch(context.Background(), Request{URL: "/api", Header: header, JSON: map[string]int{"n": 1}}); err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || string(got.Body) != `{"n":1}` || got.JSON != nil ||
		got.Header["Content-Type"] != "application/json" || got.Header["X-Test"] != "1" {
		t.Fatalf("transport got %+v", got)
	}
	if len(header) != 1 {
		t.Fatalf("Fetch changed the caller's header map: %v", header)
	}
	// A Content-Type the caller set, in any case, is kept.
	Fetch(context.Background(), Request{Method: "PUT", URL: "/api", Header: map[string]string{"content-type": "application/merge-patch+json"}, JSON: 1})
	if got.Method != "PUT" || len(got.Header) != 1 || got.Header["content-type"] != "application/merge-patch+json" {
		t.Fatalf("explicit content type: %+v", got)
	}
	if _, err := Fetch(context.Background(), Request{URL: "/api", Body: []byte("x"), JSON: 1}); err == nil {
		t.Fatal("Body and JSON together: no error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = Request{}
	if _, err := Fetch(ctx, Request{URL: "/late"}); !errors.Is(err, context.Canceled) || got.URL != "" {
		t.Fatalf("cancelled context: err %v, transport saw %q", err, got.URL)
	}
}

func TestResponseJSON(t *testing.T) {
	var v struct{ Name string }
	if err := (Response{Status: 200, Body: []byte(`{"Name":"Ada"}`)}).JSON(&v); err != nil || v.Name != "Ada" {
		t.Fatalf("decode: %v %+v", err, v)
	}
	err := Response{Status: 404, StatusText: "Not Found", URL: "/api/x", Body: []byte(`{"Name":"no"}`)}.JSON(&v)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 404 || v.Name != "Ada" {
		t.Fatalf("404: err %v, decoded %+v", err, v)
	}
	if got, want := err.Error(), "goowee/net: /api/x: 404 Not Found"; got != want {
		t.Fatalf("error text %q, want %q", got, want)
	}
}

// Outside the browser there's no network: Fetch fails, and a socket reports
// the failure on the render loop.
func TestUnavailableOutsideTheBrowser(t *testing.T) {
	flush := install(t, nil)
	if _, err := Fetch(context.Background(), Request{URL: "/api"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Fetch: %v", err)
	}
	var closed CloseEvent
	s := Dial("/ws", SocketOptions{OnClose: func(e CloseEvent) { closed = e }})
	defer s.Close()
	flush()
	if closed.Code != 1006 || !strings.Contains(closed.Reason, "no network") || closed.Reconnecting {
		t.Fatalf("OnClose: %+v", closed)
	}
}

func TestSocketCallbacksRunOnTheRenderLoop(t *testing.T) {
	tr := &scripted{}
	flush := install(t, tr)
	var log []string
	s := Dial("/ws", SocketOptions{
		Protocols: []string{"chat"},
		OnOpen:    func() { log = append(log, "open") },
		OnMessage: func(m Message) { log = append(log, "msg "+m.Text()) },
		OnClose:   func(e CloseEvent) { log = append(log, "close") },
	})
	defer s.Close()
	c := tr.conn(0)
	if c == nil || c.url != "/ws" || len(c.protocols) != 1 {
		t.Fatalf("dial: %+v", c)
	}
	if err := s.Send("early"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("Send before open: %v", err)
	}
	c.ev.Open()
	c.ev.Message(Message{Data: []byte("a")})
	c.ev.Message(Message{Data: []byte("b")})
	if len(log) != 0 {
		t.Fatalf("callbacks ran off the render loop: %v", log)
	}
	flush()
	if got := strings.Join(log, ","); got != "open,msg a,msg b" {
		t.Fatalf("callbacks: %s", got)
	}
	if err := s.Send("hi"); err != nil {
		t.Fatal(err)
	}
	if err := s.SendBinary([]byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 2 || c.sent[0].Text() != "hi" || c.sent[0].Binary || !c.sent[1].Binary {
		t.Fatalf("sent: %+v", c.sent)
	}
	c.ev.Close(CloseEvent{Code: 1001})
	flush()
	if log[len(log)-1] != "close" || tr.dials() != 1 {
		t.Fatalf("close without Reconnect: %v, %d dials", log, tr.dials())
	}
	if err := s.Send("late"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("Send after close: %v", err)
	}
}

// After Close, nothing runs: not events already queued, not OnClose.
func TestSocketCloseSilencesCallbacks(t *testing.T) {
	tr := &scripted{}
	flush := install(t, tr)
	calls := 0
	s := Dial("/ws", SocketOptions{
		OnOpen:    func() { calls++ },
		OnMessage: func(Message) { calls++ },
		OnClose:   func(CloseEvent) { calls++ },
		Reconnect: true, MinBackoff: time.Millisecond,
	})
	c := tr.conn(0)
	c.ev.Open()
	c.ev.Message(Message{Data: []byte("queued")})
	s.Close()
	c.ev.Close(CloseEvent{Code: 1000})
	flush()
	time.Sleep(10 * time.Millisecond)
	flush()
	if calls != 0 || c.closeCode != 1000 || tr.dials() != 1 {
		t.Fatalf("after Close: %d callbacks, close code %d, %d dials", calls, c.closeCode, tr.dials())
	}
	s.Close() // again: no-op
}

func TestSocketReconnects(t *testing.T) {
	tr := &scripted{}
	flush := install(t, tr)
	seq := core.NewSignal(0)
	var closes []CloseEvent
	opens := 0
	s := Dial("/ws?since=0", SocketOptions{
		OnOpen:     func() { opens++ },
		OnMessage:  func(Message) { seq.Set(seq.Get() + 1) },
		OnClose:    func(e CloseEvent) { closes = append(closes, e) },
		Reconnect:  true,
		MinBackoff: time.Millisecond, MaxBackoff: 4 * time.Millisecond,
		ReconnectURL: func() string {
			return "/ws?since=" + string(rune('0'+seq.Peek()))
		},
	})
	defer s.Close()
	first := tr.conn(0)
	first.ev.Open()
	first.ev.Message(Message{Data: []byte("1")})
	first.ev.Message(Message{Data: []byte("2")})
	first.ev.Close(CloseEvent{Code: 1006})
	flush()
	if len(closes) != 1 || !closes[0].Reconnecting {
		t.Fatalf("OnClose: %+v", closes)
	}
	eventually(t, flush, func() bool { return tr.dials() == 2 }, "the reconnection")
	second := tr.conn(1)
	if second.url != "/ws?since=2" {
		t.Fatalf("reconnected to %q, want the cursor from ReconnectURL", second.url)
	}
	// Events from the dropped connection are ignored.
	first.ev.Message(Message{Data: []byte("stale")})
	first.ev.Open()
	flush()
	if seq.Peek() != 2 || opens != 1 {
		t.Fatalf("a stale connection's events were delivered: seq %d, opens %d", seq.Peek(), opens)
	}
	second.ev.Open()
	flush()
	if opens != 2 {
		t.Fatalf("opens %d after reconnecting", opens)
	}
}

// Failed attempts back off, and Close cancels a pending reconnection.
func TestSocketBackoff(t *testing.T) {
	s := &Socket{opts: SocketOptions{MinBackoff: 100 * time.Millisecond, MaxBackoff: 800 * time.Millisecond}}
	for _, c := range []struct {
		failures int
		max      time.Duration
	}{{0, 100}, {1, 200}, {2, 400}, {3, 800}, {10, 800}} {
		s.failures = c.failures
		for range 50 {
			d := s.backoff()
			if hi := c.max * time.Millisecond; d < hi/2 || d > hi {
				t.Fatalf("failures %d: backoff %v outside [%v, %v]", c.failures, d, hi/2, hi)
			}
		}
	}
	if d := (&Socket{}).backoff(); d < 500*time.Millisecond || d > time.Second {
		t.Fatalf("default first backoff %v", d)
	}

	tr := &scripted{}
	flush := install(t, tr)
	fail := true
	tr.dial = func(string) error {
		if fail {
			return errors.New("refused")
		}
		return nil
	}
	var reasons []string
	sock := Dial("/ws", SocketOptions{
		OnClose:    func(e CloseEvent) { reasons = append(reasons, e.Reason) },
		Reconnect:  true,
		MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	})
	eventually(t, flush, func() bool { return len(reasons) >= 3 }, "three failed attempts")
	if reasons[0] != "refused" {
		t.Fatalf("reasons %v", reasons)
	}
	sock.Close()
	n := len(reasons)
	fail = false
	time.Sleep(10 * time.Millisecond)
	flush()
	if len(reasons) != n || tr.dials() != 0 {
		t.Fatalf("attempts continued after Close: %d more failures, %d dials", len(reasons)-n, tr.dials())
	}
}
