//go:build js && wasm

// Command net is the app test/e2e/net.mjs drives: goowee/net against a real
// server in a real browser. Fetch: a JSON load through UseResource, a JSON
// POST with a custom header, an error status, a binary body and a cancelled
// request, and a file upload in a Form whose bytes never enter Go memory.
// Socket: text and binary messages both ways, a subprotocol, a reconnection
// that resumes from a cursor, and Close. Subscribe: server-sent events, named
// and unnamed, a reconnection with Last-Event-ID, and the browser giving up.
package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/net"
)

func main() { bridge.Run(app()) }

func app() core.Node {
	return core.Component("NetE2E", func() core.Node {
		user := hooks.UseResource(nil, func(ctx context.Context) (string, error) {
			var u struct{ Name string }
			resp, err := net.Fetch(ctx, net.Request{URL: "/api/user"})
			if err == nil {
				err = resp.JSON(&u)
			}
			return u.Name, err
		})
		echo, setEcho := hooks.UseState("")
		missing, setMissing := hooks.UseState("")
		bytes, setBytes := hooks.UseState("")
		slow, setSlow := hooks.UseState("")
		cancelSlow := func() {}

		uploaded, setUploaded := hooks.UseState("")
		sse, setSSE := hooks.UseState("")
		hooks.OnMount(func() func() {
			sub := net.Subscribe("/api/events", net.SubscribeOptions{
				Events:  []string{"presence"},
				OnOpen:  func() { setSSE(sse.Get() + "open|") },
				OnEvent: func(e net.Event) { setSSE(sse.Get() + e.Type + ":" + e.Data + "#" + e.ID + "|") },
				OnError: func(r bool) { setSSE(sse.Get() + fmt.Sprintf("error:%v|", r)) },
			})
			return sub.Close
		})

		msgs, setMsgs := hooks.UseState([]string{})
		closes, setCloses := hooks.UseState("")
		status, setStatus := hooks.UseState("connecting")
		var sock *net.Socket
		hooks.OnMount(func() func() {
			sock = net.Dial("/ws?since=0", net.SocketOptions{
				Protocols: []string{"goowee-e2e"},
				OnOpen:    func() { setStatus("open") },
				OnMessage: func(m net.Message) {
					text := m.Text()
					if m.Binary {
						text = fmt.Sprintf("bin%v", m.Data)
					}
					setMsgs(append(msgs.Get(), text))
				},
				OnClose: func(e net.CloseEvent) {
					setStatus("closed")
					setCloses(closes.Get() + fmt.Sprintf("%d:%v ", e.Code, e.Reconnecting))
				},
				Reconnect:    true,
				MinBackoff:   50 * time.Millisecond,
				MaxBackoff:   200 * time.Millisecond,
				ReconnectURL: func() string { return fmt.Sprintf("/ws?since=%d", len(msgs.Peek())) },
			})
			return sock.Close
		})

		// run does f off the render loop (Fetch blocks) and shows its result.
		run := func(set func(string), f func() (string, error)) {
			go func() {
				out, err := f()
				if err != nil {
					out = "error: " + err.Error()
				}
				core.Schedule(func() { set(out) })
			}()
		}
		return Div(
			P(ID("user"), Textf("user: %s", user.Data)),
			Button(ID("post"), OnClick(func() {
				run(setEcho, func() (string, error) {
					resp, err := net.Fetch(context.Background(), net.Request{
						Method: "POST", URL: "/api/echo",
						Header: map[string]string{"X-Goowee": "e2e"},
						JSON:   map[string]any{"n": 1, "s": "héllo"},
					})
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("%d %s %s %s", resp.Status, resp.Header["x-echo-method"], resp.Header["x-echo-type"], resp.Text()), nil
				})
			}), Text("post")),
			P(ID("echo"), TextS(echo)),
			Button(ID("missing"), OnClick(func() {
				run(setMissing, func() (string, error) {
					resp, err := net.Fetch(context.Background(), net.Request{URL: "/api/missing"})
					if err != nil {
						return "", err
					}
					var v any
					return fmt.Sprintf("ok=%v %v", resp.OK(), resp.JSON(&v)), nil
				})
			}), Text("missing")),
			P(ID("missing-out"), TextS(missing)),
			Button(ID("bytes"), OnClick(func() {
				run(setBytes, func() (string, error) {
					resp, err := net.Fetch(context.Background(), net.Request{URL: "/api/bytes"})
					sum := 0
					for _, b := range resp.Body {
						sum += int(b)
					}
					return fmt.Sprintf("bytes %d sum %d", len(resp.Body), sum), err
				})
			}), Text("bytes")),
			P(ID("bytes-out"), TextS(bytes)),
			Button(ID("slow"), OnClick(func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancelSlow = cancel
				run(setSlow, func() (string, error) {
					_, err := net.Fetch(ctx, net.Request{URL: "/api/slow"})
					return "finished", err
				})
			}), Text("slow")),
			Button(ID("cancel"), OnClick(func() { cancelSlow() }), Text("cancel")),
			P(ID("slow-out"), TextS(slow)),
			Input(ID("upload"), Type("file"), OnChangeE(func(e core.EventData) {
				form := new(net.Form).Add("caption", "beach")
				for _, f := range e.Files() {
					form.AddFile("photo", f)
				}
				run(setUploaded, func() (string, error) {
					// TotalAlloc counts every byte Go allocates: a copy of the
					// file into Go memory would add its whole size.
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					resp, err := net.Fetch(context.Background(), net.Request{Method: "POST", URL: "/api/upload", Form: form})
					runtime.ReadMemStats(&after)
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("%d %s alloc:%dMB", resp.Status, resp.Text(), (after.TotalAlloc-before.TotalAlloc)>>20), nil
				})
			})),
			P(ID("uploaded"), TextS(uploaded)),
			P(ID("sse"), TextS(sse)),

			P(ID("status"), TextS(status)),
			P(ID("msgs"), TextS(core.Computed([]core.SignalAccessor{msgs}, func() string {
				return strings.Join(msgs.Get(), "|")
			}))),
			P(ID("closes"), TextS(closes)),
			Button(ID("send"), OnClick(func() {
				if err := sock.Send("ping"); err != nil {
					setStatus(err.Error())
				}
				sock.SendBinary([]byte{1, 2, 3})
			}), Text("send")),
			Button(ID("close"), OnClick(func() {
				sock.Close()
				setStatus("closed by app")
			}), Text("close")),
		)
	})
}
