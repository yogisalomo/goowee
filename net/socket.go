package net

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/yogisalomo/goowee/core"
)

// SocketOptions configures a Socket. Every callback runs on the render loop
// (through core.Schedule), so it can read and set signals directly; none runs
// after Close.
type SocketOptions struct {
	// Protocols are the WebSocket subprotocols to offer.
	Protocols []string
	// OnOpen runs each time a connection opens (again after a reconnect).
	OnOpen func()
	// OnMessage runs for each message, in arrival order.
	OnMessage func(Message)
	// OnClose runs each time a connection ends or fails to open, except one
	// ended by Close.
	OnClose func(CloseEvent)

	// Reconnect connects again after a connection ends (other than through
	// Close), waiting between attempts: MinBackoff after a connection that was
	// open, doubling for each attempt in a row that fails to open, up to
	// MaxBackoff. Each wait is jittered (between half and all of it), so
	// clients dropped together don't reconnect in lockstep.
	Reconnect bool
	// MinBackoff is the first wait before reconnecting (default 1s).
	MinBackoff time.Duration
	// MaxBackoff caps the wait (default 30s).
	MaxBackoff time.Duration
	// ReconnectURL, when set, gives the URL for each reconnection (Dial's url
	// is used for the first connection), e.g. to resume from a replay cursor:
	//
	//	ReconnectURL: func() string { return "/ws?since=" + strconv.FormatInt(seq.Peek(), 10) },
	//
	// It runs on the render loop.
	ReconnectURL func() string
}

// ReconnectDelayer is implemented by a Transport that decides how long a
// Socket waits before reconnecting, given the wait its backoff planned:
// gooweetest.FakeNet does, so a reconnection test needn't sit out a real
// backoff.
type ReconnectDelayer interface {
	ReconnectDelay(planned time.Duration) time.Duration
}

// Socket is a WebSocket opened by Dial, reconnecting when SocketOptions ask
// for it. Its methods are safe to call from any goroutine.
type Socket struct {
	url  string
	opts SocketOptions

	mu       sync.Mutex
	tr       Transport // what the current attempt dialed with
	conn     Conn
	open     bool
	closed   bool        // Close was called
	gen      int         // connection attempts; events of an older one are ignored
	failures int         // attempts in a row that never opened
	timer    *time.Timer // the pending reconnection
}

// Dial opens a WebSocket to url, absolute (ws://, wss://) or relative to the
// page (an http(s) URL or a path, such as "/ws", connects with the page's
// scheme), and returns without waiting: OnOpen runs once it's connected, and
// a failure to connect arrives as OnClose. Close the socket when it's no
// longer needed, typically from an OnMount cleanup:
//
//	hooks.OnMount(func() func() {
//	    sock := net.Dial("/ws", net.SocketOptions{OnMessage: onMessage, Reconnect: true})
//	    return sock.Close
//	})
func Dial(url string, opts SocketOptions) *Socket {
	s := &Socket{url: url, opts: opts}
	s.connect(url)
	return s
}

// Send sends a text message. It returns ErrNotOpen while the socket isn't
// connected (before OnOpen, between reconnects, after Close); messages are not
// queued.
func (s *Socket) Send(text string) error { return s.send(Message{Data: []byte(text)}) }

// SendBinary sends a binary message, like Send.
func (s *Socket) SendBinary(data []byte) error { return s.send(Message{Data: data, Binary: true}) }

func (s *Socket) send(m Message) error {
	s.mu.Lock()
	conn, open := s.conn, s.open
	s.mu.Unlock()
	if conn == nil || !open {
		return ErrNotOpen
	}
	return conn.Send(m)
}

// Close closes the connection (code 1000) and stops reconnecting. No callback
// runs after Close, including OnClose for this connection. Calling it again
// does nothing.
func (s *Socket) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conn := s.conn
	s.conn, s.open = nil, false
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	if conn != nil {
		conn.Close(1000, "")
	}
}

// connect starts one connection attempt.
func (s *Socket) connect(url string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.gen++
	gen := s.gen
	tr := current()
	s.tr = tr
	s.mu.Unlock()
	conn, err := tr.Dial(url, s.opts.Protocols, ConnEvents{
		Open:    func() { core.Schedule(func() { s.opened(gen) }) },
		Message: func(m Message) { core.Schedule(func() { s.message(gen, m) }) },
		Close:   func(e CloseEvent) { core.Schedule(func() { s.ended(gen, e) }) },
	})
	if err != nil {
		core.Schedule(func() { s.ended(gen, CloseEvent{Code: 1006, Reason: err.Error()}) })
		return
	}
	s.mu.Lock()
	if s.closed || gen != s.gen {
		s.mu.Unlock()
		conn.Close(1000, "") // Close ran while Dial was starting this one
		return
	}
	s.conn = conn
	s.mu.Unlock()
}

// live reports whether gen is the live attempt of an unclosed socket.
func (s *Socket) live(gen int) bool { return !s.closed && gen == s.gen }

func (s *Socket) opened(gen int) {
	s.mu.Lock()
	if !s.live(gen) {
		s.mu.Unlock()
		return
	}
	s.open, s.failures = true, 0
	s.mu.Unlock()
	if s.opts.OnOpen != nil {
		s.opts.OnOpen()
	}
}

func (s *Socket) message(gen int, m Message) {
	s.mu.Lock()
	live := s.live(gen)
	s.mu.Unlock()
	if live && s.opts.OnMessage != nil {
		s.opts.OnMessage(m)
	}
}

func (s *Socket) ended(gen int, e CloseEvent) {
	s.mu.Lock()
	if !s.live(gen) {
		s.mu.Unlock()
		return
	}
	wait := s.backoff()
	if d, ok := s.tr.(ReconnectDelayer); ok {
		wait = d.ReconnectDelay(wait)
	}
	if !s.open {
		s.failures++
	}
	s.conn, s.open = nil, false
	e.Reconnecting = s.opts.Reconnect
	if e.Reconnecting {
		s.timer = time.AfterFunc(wait, func() { core.Schedule(s.reconnect) })
	}
	s.mu.Unlock()
	if s.opts.OnClose != nil {
		s.opts.OnClose(e)
	}
}

// reconnect runs on the render loop when a reconnection's wait is over.
func (s *Socket) reconnect() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.timer = nil
	s.mu.Unlock()
	url := s.url
	if s.opts.ReconnectURL != nil {
		url = s.opts.ReconnectURL()
	}
	s.connect(url)
}

// backoff is the wait before the next attempt: MinBackoff doubled per
// attempt in a row that failed to open, capped at MaxBackoff, then jittered.
// The caller holds s.mu.
func (s *Socket) backoff() time.Duration {
	lo, hi := s.opts.MinBackoff, s.opts.MaxBackoff
	if lo <= 0 {
		lo = time.Second
	}
	if hi <= 0 {
		hi = 30 * time.Second
	}
	hi = max(hi, lo)
	d := lo
	for i := 0; i < s.failures && d < hi; i++ {
		d *= 2
	}
	d = min(d, hi)
	return d/2 + rand.N(d/2+1)
}
