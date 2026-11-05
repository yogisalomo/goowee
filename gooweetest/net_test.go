package gooweetest_test

import (
	"context"
	"encoding/json"
	"errors"
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
