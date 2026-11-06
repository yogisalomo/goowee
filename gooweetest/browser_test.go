package gooweetest_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/net"
	"github.com/yogisalomo/goowee/storage"
)

// #99: a picked file is uploaded as a Form field; the handler sees the
// fields and reads the file's contents.
func TestFormUpload(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	var caption, got string
	n.Handle("POST /api/photos", func(r net.Request) (net.Response, error) {
		caption = r.Form.Value("caption")
		f, _ := r.Form.File("photo")
		data, err := f.Bytes()
		got = fmt.Sprintf("%s %s %q %v", f.Name, f.Type, data, err)
		return net.Response{Status: 201}, nil
	})
	s := gooweetest.Render(t, core.Component("Upload", func() core.Node {
		status, setStatus := hooks.UseState("")
		return Div(
			Input(Type("file"), OnChangeE(func(e core.EventData) {
				form := new(net.Form).Add("caption", "beach")
				for _, f := range e.Files() {
					form.AddFile("photo", f)
				}
				go func() {
					resp, err := net.Fetch(context.Background(), net.Request{Method: "POST", URL: "/api/photos", Form: form})
					core.Schedule(func() { setStatus(fmt.Sprint(resp.Status, err)) })
				}()
			})),
			P(TextS(status)),
		)
	}))
	s.SelectFiles(s.Find("input"), gooweetest.File{Name: "beach.jpg", Type: "image/jpeg", Data: []byte("JPEG")})
	s.WaitForText("201 <nil>")
	if caption != "beach" || got != `beach.jpg image/jpeg "JPEG" <nil>` {
		t.Fatalf("server saw caption %q, file %s", caption, got)
	}
}

// #100: a server-sent events feed, driven from the test.
func TestEventStream(t *testing.T) {
	n := gooweetest.NewFakeNet(t)
	s := gooweetest.Render(t, core.Component("Feed", func() core.Node {
		log, setLog := hooks.UseState("")
		add := func(s string) { setLog(log.Get() + s + " ") }
		hooks.OnMount(func() func() {
			sub := net.Subscribe("/api/events?room=7", net.SubscribeOptions{
				Events:  []string{"presence"},
				OnOpen:  func() { add("open") },
				OnEvent: func(e net.Event) { add(e.Type + ":" + e.Data) },
				OnError: func(r bool) { add(fmt.Sprint("error:", r)) },
			})
			return sub.Close
		})
		return P(TextS(log))
	}))
	es := s.WaitForStream(n, "/api/events")
	es.Send("hello")
	es.SendEvent(net.Event{Type: "presence", Data: "ada"})
	es.SendEvent(net.Event{Type: "typing", Data: "ignored"}) // not subscribed
	es.Fail(true)
	es.Reopen()
	s.Flush()
	if got := strings.TrimSpace(s.Find("p").Text()); got != "open message:hello presence:ada error:true open" {
		t.Fatalf("feed: %q", got)
	}
	es.Fail(false)
	s.Flush()
	if !es.Closed() || !strings.HasSuffix(strings.TrimSpace(s.Find("p").Text()), "error:false") {
		t.Fatalf("after the browser gave up: %q closed=%v", s.Find("p").Text(), es.Closed())
	}
}

// #100: storage is in memory under gooweetest; seed it before Render.
func TestStorageInTests(t *testing.T) {
	storage.Local.Set("draft", "half-written")
	s := gooweetest.Render(t, core.Component("Draft", func() core.Node {
		draft, setDraft := hooks.UseState("")
		hooks.OnMount(func() func() {
			if v, ok := storage.Local.Get("draft"); ok {
				setDraft(v)
			}
			return nil
		})
		hooks.Watch([]core.SignalAccessor{draft}, func() { storage.Local.Set("draft", draft.Get()) })
		return Textarea(BindValue(draft))
	}))
	if got := s.Find("textarea").Value(); got != "half-written" {
		t.Fatalf("restored %q", got)
	}
	s.Input(s.Find("textarea"), "finished")
	if v, _ := storage.Local.Get("draft"); v != "finished" {
		t.Fatalf("saved %q", v)
	}
}

// Render's cleanup cleared the store the test above wrote.
func TestStorageClearedBetweenTests(t *testing.T) {
	if keys := storage.Local.Keys(); len(keys) != 0 {
		t.Fatalf("left over from an earlier test: %v", keys)
	}
}
