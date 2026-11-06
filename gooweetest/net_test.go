package gooweetest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/net"
)

type user struct{ Name string }

// profile loads a user with net.Fetch and posts a rename from a click.
func profile() core.Node {
	return core.Component("Profile", func() core.Node {
		u := hooks.UseResource(nil, func(ctx context.Context) (user, error) {
			var u user
			resp, err := net.Fetch(ctx, net.Request{URL: "/api/user?id=7"})
			if err == nil {
				err = resp.JSON(&u)
			}
			return u, err
		})
		saved, setSaved := hooks.UseState("")
		return Div(
			ShowResource(u,
				func() core.Node { return P(Text("Loading…")) },
				func(err error) core.Node { return P(Class("err"), Textf("Error: %s", err)) },
				func(u *core.Signal[user]) core.Node {
					return P(Textf("Hello, %s", core.Computed([]core.SignalAccessor{u}, func() string { return u.Get().Name })))
				},
			),
			Button(OnClick(func() {
				go func() {
					resp, err := net.Fetch(context.Background(), net.Request{Method: "POST", URL: "/api/user", JSON: user{Name: "Grace"}})
					core.Schedule(func() {
						if err != nil {
							setSaved(err.Error())
						} else {
							setSaved(resp.Text())
						}
					})
				}()
			}), Text("Rename")),
			P(Class("saved"), TextS(saved)),
		)
	})
}

func TestFakeNetFetch(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	n.Handle("GET /api/user", func(r net.Request) (net.Response, error) {
		return gooweetest.JSONResponse(200, user{Name: "Ada"})
	})
	n.Handle("POST /api/user", func(r net.Request) (net.Response, error) {
		var u user
		json.Unmarshal(r.Body, &u)
		return net.Response{Status: 201, Body: []byte("saved " + u.Name)}, nil
	})
	s := gooweetest.Render(t, profile())
	s.WaitForText("Hello, Ada")
	s.Click(s.FindByText("Rename"))
	s.WaitForText("saved Grace")
	reqs := n.Requests()
	if len(reqs) != 2 || reqs[0].Method != "GET" || reqs[0].URL != "/api/user?id=7" ||
		reqs[1].Method != "POST" || string(reqs[1].Body) != `{"Name":"Grace"}` ||
		reqs[1].Header["Content-Type"] != "application/json" {
		t.Fatalf("requests: %+v", reqs)
	}
}

func TestFakeNetErrors(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	s := gooweetest.Render(t, profile()) // no handler: 404
	s.WaitForText("Error: goowee/net: /api/user?id=7: 404 Not Found")

	n.Handle("/api/user", func(net.Request) (net.Response, error) {
		return net.Response{}, errors.New("offline")
	})
	s.Click(s.FindByText("Rename"))
	s.WaitForText("offline")
}

// A cancelled fetch returns at once, even while the handler is still busy, as
// the browser aborts a request.
func TestFakeNetHonorsCancellation(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	release := make(chan struct{})
	defer close(release)
	n.Handle("/api/slow", func(net.Request) (net.Response, error) {
		<-release
		return net.Response{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := net.Fetch(ctx, net.Request{URL: "/api/slow"})
		done <- err
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Fetch did not return when its context was cancelled")
	}
}

// feed shows a socket's messages and status, sends on click, and resumes from
// the number of messages seen when it reconnects.
func feed() core.Node {
	return core.Component("Feed", func() core.Node {
		msgs, setMsgs := hooks.UseState([]string{})
		status, setStatus := hooks.UseState("connecting")
		var sock *net.Socket
		hooks.OnMount(func() func() {
			sock = net.Dial("/ws?since=0", net.SocketOptions{
				OnOpen:    func() { setStatus("open") },
				OnMessage: func(m net.Message) { setMsgs(append(msgs.Get(), m.Text())) },
				OnClose: func(e net.CloseEvent) {
					if e.Reconnecting {
						setStatus("reconnecting")
					}
				},
				Reconnect:    true,
				MinBackoff:   time.Millisecond,
				ReconnectURL: func() string { return "/ws?since=" + strings.Repeat("i", len(msgs.Get())) },
			})
			return sock.Close
		})
		return Div(
			P(Class("status"), TextS(status)),
			Ul(For(msgs, func(m string) string { return m }, func(m string) core.Node { return Li(Text(m)) })),
			Button(OnClick(func() { sock.Send("ping") }), Text("Ping")),
		)
	})
}

func TestFakeNetSocket(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	s := gooweetest.Render(t, feed())
	if got := s.Find("p.status").Text(); got != "open" {
		t.Fatalf("status %q after mount", got)
	}
	ws := n.Socket("/ws")
	ws.Send("one")
	ws.SendJSON(map[string]int{"two": 2})
	s.Flush()
	if got := s.Find("ul").Text(); got != `one{"two":2}` {
		t.Fatalf("messages %q", got)
	}
	s.Click(s.FindByText("Ping"))
	if got := ws.Received(); len(got) != 1 || got[0].Text() != "ping" {
		t.Fatalf("server received %+v", got)
	}

	// The server drops the connection; the app reconnects from its cursor.
	ws.Close(1012, "restart")
	s.Flush()
	if got := s.Find("p.status").Text(); got != "reconnecting" {
		t.Fatalf("status %q after the drop", got)
	}
	s.WaitFor(func() bool { return len(n.Sockets()) == 2 })
	s.Flush()
	again := n.Socket("/ws")
	if again == ws || again.URL != "/ws?since=ii" || s.Find("p.status").Text() != "open" {
		t.Fatalf("reconnected to %q, status %q", again.URL, s.Find("p.status").Text())
	}

	// Unmounting runs the OnMount cleanup, which closes the socket.
	s.Unmount()
	if !again.Closed() {
		t.Fatal("socket left open after unmount")
	}
}

// #114: ServeMux patterns: prefixes, wildcards and a catch-all, most specific
// first; re-registering a pattern replaces its answer.
func TestFakeNetPatterns(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	answer := func(s string) func(net.Request) (net.Response, error) {
		return func(net.Request) (net.Response, error) { return net.Response{Body: []byte(s)}, nil }
	}
	n.Handle("/", gooweetest.Status(204))
	n.Handle("POST /api/", answer("api post"))
	n.Handle("GET /api/rooms/{id}/messages", answer("messages"))
	n.Handle("GET /api/rooms/7/messages", answer("room 7"))
	get := func(method, url string) string {
		resp, err := net.Fetch(context.Background(), net.Request{Method: method, URL: url})
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%d %s", resp.Status, resp.Text())
	}
	for _, c := range []struct{ method, url, want string }{
		{"POST", "/api/invite", "200 api post"},
		{"POST", "/api/questions/7/answer?x=1", "200 api post"},
		{"GET", "/api/rooms/3/messages", "200 messages"},
		{"GET", "/api/rooms/7/messages", "200 room 7"}, // more specific than {id}
		{"GET", "http://localhost:8080/api/rooms/3/messages", "200 messages"},
		{"GET", "/api/activity", "204 "}, // only the catch-all matches
		{"DELETE", "/elsewhere", "204 "},
	} {
		if got := get(c.method, c.url); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.method, c.url, got, c.want)
		}
	}
	n.Handle("POST /api/", answer("changed"))
	if got := get("POST", "/api/invite"); got != "200 changed" {
		t.Errorf("re-registered pattern: %q", got)
	}
	if len(n.Requests()) != 8 {
		t.Errorf("requests recorded: %d", len(n.Requests()))
	}

	// A pattern ServeMux rejects fails the test.
	rec := &recordTB{TB: t}
	bad := gooweetest.NewFakeNet(rec)
	bad.Handle("GET /a/{x}", gooweetest.Status(200))
	bad.Handle("GET /a/{y}", gooweetest.Status(200)) // conflicts with {x}
	if !strings.Contains(rec.fatal, "conflicts") {
		t.Errorf("conflicting patterns: %q", rec.fatal)
	}
}

// recordTB records a Fatalf instead of stopping the test.
type recordTB struct {
	testing.TB
	fatal string
}

func (r *recordTB) Fatalf(format string, args ...any) { r.fatal = fmt.Sprintf(format, args...) }
func (r *recordTB) Helper()                           {}

// #115: an app that dials after an async step, and the new socket after a
// reconnection, are awaited with WaitForSocket; SetReconnectDelay skips the
// real backoff (1s by default).
func TestWaitForSocket(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	n.Handle("GET /api/bootstrap", func(net.Request) (net.Response, error) {
		time.Sleep(20 * time.Millisecond)
		return gooweetest.JSONResponse(200, map[string]int{"seq": 41})
	})
	n.SetReconnectDelay(0)
	s := gooweetest.Render(t, core.Component("Live", func() core.Node {
		msgs, setMsgs := hooks.UseState("")
		hooks.OnMount(func() func() {
			var sock *net.Socket
			go func() {
				resp, _ := net.Fetch(context.Background(), net.Request{URL: "/api/bootstrap"})
				var b struct{ Seq int }
				resp.JSON(&b)
				core.Schedule(func() { // dial only once bootstrap has loaded
					sock = net.Dial(fmt.Sprintf("/ws?since=%d", b.Seq), net.SocketOptions{
						OnMessage: func(m net.Message) { setMsgs(msgs.Get() + m.Text()) },
						Reconnect: true, // default backoff 0.5–1s; SetReconnectDelay overrides it
					})
				})
			}()
			return func() {
				if sock != nil {
					sock.Close()
				}
			}
		})
		return P(TextS(msgs))
	}))
	ws := s.WaitForSocket(n, "/ws")
	if ws.URL != "/ws?since=41" {
		t.Fatalf("dialed %q", ws.URL)
	}
	ws.Send("a")
	ws.Close(1012, "restart")
	start := time.Now()
	again := s.WaitForSocket(n, "/ws", 400*time.Millisecond)
	if again == ws || time.Since(start) > 400*time.Millisecond {
		t.Fatalf("reconnection: same socket %v after %v", again == ws, time.Since(start))
	}
	again.Send("b")
	s.Flush()
	if got := s.Find("p").Text(); got != "ab" {
		t.Fatalf("messages %q", got)
	}
}
