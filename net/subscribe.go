package net

import (
	"sync"

	"github.com/yogisalomo/goowee/core"
)

// Event is one server-sent event.
type Event struct {
	// Type is the event's name ("event:" line), "message" when it has none.
	Type string
	Data string
	// ID is the last event id the stream has seen ("id:" line); the browser
	// sends it back as Last-Event-ID when it reconnects.
	ID string
}

// SubscribeOptions configures a Subscription. Every callback runs on the
// render loop (through core.Schedule), so it can read and set signals
// directly; none runs after Close.
type SubscribeOptions struct {
	// Events lists the named event types to receive, besides unnamed
	// ("message") events.
	Events []string
	// OnOpen runs each time the stream connects (again after a reconnect).
	OnOpen func()
	// OnEvent runs for each event, in arrival order.
	OnEvent func(Event)
	// OnError runs when the connection fails or drops. reconnecting reports
	// whether the browser will connect again by itself (it gives up on an
	// HTTP error status or a response that isn't text/event-stream).
	OnError func(reconnecting bool)
	// WithCredentials sends cookies on a cross-origin stream.
	WithCredentials bool
}

// StreamTransport is a Transport that can also open server-sent event
// streams for Subscribe. The browser transport and gooweetest.FakeNet are;
// with one that isn't, a subscription fails at once (OnError(false)).
type StreamTransport interface {
	Subscribe(url string, events []string, withCredentials bool, ev StreamEvents) (Stream, error)
}

// StreamEvents are a stream's callbacks to its Subscription, which moves them
// onto the render loop. Error's argument is whether the stream will reconnect.
type StreamEvents struct {
	Open  func()
	Event func(Event)
	Error func(reconnecting bool)
}

// Stream is one open event stream made by a StreamTransport.
type Stream interface {
	Close()
}

// Subscription is a server-sent events stream opened by Subscribe. Its
// methods are safe to call from any goroutine.
type Subscription struct {
	opts SubscribeOptions

	mu     sync.Mutex
	stream Stream
	closed bool
}

// Subscribe opens a server-sent events stream (the browser's EventSource) to
// url, absolute or relative to the page, and returns without waiting. The
// browser reconnects by itself after a dropped connection, sending the last
// event id. Close the subscription when it's no longer needed, typically from
// an OnMount cleanup:
//
//	hooks.OnMount(func() func() {
//	    sub := net.Subscribe("/api/events", net.SubscribeOptions{
//	        OnEvent: func(e net.Event) { setLast(e.Data) },
//	    })
//	    return sub.Close
//	})
//
// Use Dial for a WebSocket when the page also sends messages.
func Subscribe(url string, opts SubscribeOptions) *Subscription {
	s := &Subscription{opts: opts}
	tr, ok := current().(StreamTransport)
	if !ok {
		core.Schedule(func() { s.failed() })
		return s
	}
	stream, err := tr.Subscribe(url, opts.Events, opts.WithCredentials, StreamEvents{
		Open:  func() { core.Schedule(s.opened) },
		Event: func(e Event) { core.Schedule(func() { s.event(e) }) },
		Error: func(reconnecting bool) { core.Schedule(func() { s.errored(reconnecting) }) },
	})
	if err != nil {
		core.Schedule(func() { s.failed() })
		return s
	}
	s.mu.Lock()
	s.stream = stream
	closed := s.closed
	s.mu.Unlock()
	if closed {
		stream.Close()
	}
	return s
}

// Close closes the stream; no callback runs after it. Calling it again does
// nothing.
func (s *Subscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	stream := s.stream
	s.stream = nil
	s.mu.Unlock()
	if stream != nil {
		stream.Close()
	}
}

func (s *Subscription) live() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *Subscription) opened() {
	if s.live() && s.opts.OnOpen != nil {
		s.opts.OnOpen()
	}
}

func (s *Subscription) event(e Event) {
	if s.live() && s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

func (s *Subscription) errored(reconnecting bool) {
	if s.live() && s.opts.OnError != nil {
		s.opts.OnError(reconnecting)
	}
}

// failed reports a stream that couldn't be opened at all.
func (s *Subscription) failed() { s.errored(false) }
