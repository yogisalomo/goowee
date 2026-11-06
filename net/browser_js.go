//go:build js && wasm

package net

import (
	"context"
	"errors"
	"fmt"
	"syscall/js"
)

// platform is the browser's fetch and WebSocket.
var platform Transport = browser{}

type browser struct{}

func (browser) Fetch(ctx context.Context, req Request) (Response, error) {
	global := js.Global()
	init := global.Get("Object").New()
	init.Set("method", req.Method)
	if len(req.Header) > 0 {
		header := global.Get("Object").New()
		for k, v := range req.Header {
			header.Set(k, v)
		}
		init.Set("headers", header)
	}
	if req.Body != nil {
		body := global.Get("Uint8Array").New(len(req.Body))
		js.CopyBytesToJS(body, req.Body)
		init.Set("body", body)
	}
	if req.Form != nil {
		form := global.Get("FormData").New()
		for _, f := range req.Form.Fields {
			if f.File == nil {
				form.Call("append", f.Name, f.Value)
				continue
			}
			// The parked File itself: its bytes never pass through Go.
			file := js.Undefined()
			if f.File.Handle() != 0 {
				file = global.Get("goowee").Call("fileValue", f.File.Handle())
			}
			if file.IsUndefined() {
				return Response{}, fmt.Errorf("goowee/net: %s %s: file %q is no longer available (its input's selection changed, a later paste or drop replaced it, or its element was removed)", req.Method, req.URL, f.File.Name)
			}
			form.Call("append", f.Name, file, f.File.Name)
		}
		init.Set("body", form)
	}
	if req.Credentials != "" {
		init.Set("credentials", req.Credentials)
	}
	// The abort signal covers the body download too.
	ctrl := global.Get("AbortController").New()
	init.Set("signal", ctrl.Get("signal"))
	stop := context.AfterFunc(ctx, func() { ctrl.Call("abort") })
	defer stop()

	resp, err := await(global.Call("fetch", req.URL, init))
	var buf js.Value
	if err == nil {
		buf, err = await(resp.Call("arrayBuffer"))
	}
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		return Response{}, fmt.Errorf("goowee/net: %s %s: %w", req.Method, req.URL, err)
	}
	r := Response{
		Status:     resp.Get("status").Int(),
		StatusText: resp.Get("statusText").String(),
		URL:        resp.Get("url").String(),
		Header:     map[string]string{},
	}
	entries := resp.Get("headers").Call("entries")
	for {
		next := entries.Call("next")
		if next.Get("done").Bool() {
			break
		}
		kv := next.Get("value")
		r.Header[kv.Index(0).String()] = kv.Index(1).String()
	}
	body := global.Get("Uint8Array").New(buf)
	r.Body = make([]byte, body.Get("length").Int())
	js.CopyBytesToGo(r.Body, body)
	return r, nil
}

// await blocks until the promise p settles. A rejection becomes an error
// carrying the rejection's text (a TypeError's "Failed to fetch", say).
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	done := make(chan result, 1)
	onValue := js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- result{v: args[0]}
		return nil
	})
	onError := js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- result{err: jsError(args[0])}
		return nil
	})
	defer onValue.Release()
	defer onError.Release()
	p.Call("then", onValue, onError)
	r := <-done
	return r.v, r.err
}

func jsError(v js.Value) error {
	if v.IsUndefined() || v.IsNull() {
		return errors.New("rejected")
	}
	return errors.New(v.Call("toString").String())
}

func (browser) Dial(url string, protocols []string, ev ConnEvents) (c Conn, err error) {
	// The URL and WebSocket constructors throw on a malformed URL or protocol.
	defer func() {
		if r := recover(); r != nil {
			c, err = nil, fmt.Errorf("goowee/net: dial %s: %v", url, r)
		}
	}()
	global := js.Global()
	// Resolve against the page, as fetch does, and use the page's scheme: an
	// https page connects with wss.
	u := global.Get("URL").New(url, global.Get("document").Get("baseURI"))
	switch u.Get("protocol").String() {
	case "http:":
		u.Set("protocol", "ws:")
	case "https:":
		u.Set("protocol", "wss:")
	}
	var ws js.Value
	if len(protocols) > 0 {
		list := make([]any, len(protocols))
		for i, p := range protocols {
			list[i] = p
		}
		ws = global.Get("WebSocket").New(u.Call("toString"), list)
	} else {
		ws = global.Get("WebSocket").New(u.Call("toString"))
	}
	ws.Set("binaryType", "arraybuffer")
	bc := &browserConn{ws: ws}
	bc.onOpen = js.FuncOf(func(js.Value, []js.Value) any {
		ev.Open()
		return nil
	})
	bc.onMessage = js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		if data.Type() == js.TypeString {
			ev.Message(Message{Data: []byte(data.String())})
			return nil
		}
		bytes := global.Get("Uint8Array").New(data)
		m := Message{Data: make([]byte, bytes.Get("length").Int()), Binary: true}
		js.CopyBytesToGo(m.Data, bytes)
		ev.Message(m)
		return nil
	})
	bc.onClose = js.FuncOf(func(_ js.Value, args []js.Value) any {
		e := args[0]
		bc.release()
		ev.Close(CloseEvent{Code: e.Get("code").Int(), Reason: e.Get("reason").String()})
		return nil
	})
	ws.Set("onopen", bc.onOpen)
	ws.Set("onmessage", bc.onMessage)
	ws.Set("onclose", bc.onClose)
	return bc, nil
}

type browserConn struct {
	ws                         js.Value
	onOpen, onMessage, onClose js.Func
}

func (c *browserConn) Send(m Message) (err error) {
	if c.ws.Get("readyState").Int() != 1 { // OPEN
		return ErrNotOpen
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("goowee/net: send: %v", r)
		}
	}()
	if m.Binary {
		data := js.Global().Get("Uint8Array").New(len(m.Data))
		js.CopyBytesToJS(data, m.Data)
		c.ws.Call("send", data)
	} else {
		c.ws.Call("send", string(m.Data))
	}
	return nil
}

func (c *browserConn) Close(code int, reason string) {
	// close throws for a code outside 1000 and 3000–4999; fall back to 1000.
	defer func() {
		if recover() != nil {
			c.ws.Call("close", 1000)
		}
	}()
	c.ws.Call("close", code, reason)
}

// release drops the event handlers once the connection is closed; no event
// follows close.
func (c *browserConn) release() {
	c.ws.Set("onopen", js.Null())
	c.ws.Set("onmessage", js.Null())
	c.ws.Set("onclose", js.Null())
	c.onOpen.Release()
	c.onMessage.Release()
	c.onClose.Release()
}

// Subscribe opens an EventSource.
func (browser) Subscribe(url string, events []string, withCredentials bool, ev StreamEvents) (st Stream, err error) {
	defer func() { // the constructor throws on a malformed URL
		if r := recover(); r != nil {
			st, err = nil, fmt.Errorf("goowee/net: subscribe %s: %v", url, r)
		}
	}()
	global := js.Global()
	init := global.Get("Object").New()
	init.Set("withCredentials", withCredentials)
	es := global.Get("EventSource").New(url, init)
	bs := &browserStream{es: es}
	bs.onOpen = js.FuncOf(func(js.Value, []js.Value) any {
		ev.Open()
		return nil
	})
	bs.onEvent = js.FuncOf(func(_ js.Value, args []js.Value) any {
		e := args[0]
		ev.Event(Event{Type: e.Get("type").String(), Data: e.Get("data").String(), ID: e.Get("lastEventId").String()})
		return nil
	})
	bs.onError = js.FuncOf(func(js.Value, []js.Value) any {
		reconnecting := es.Get("readyState").Int() != 2 // CLOSED: the browser gave up
		if !reconnecting {
			bs.release()
		}
		ev.Error(reconnecting)
		return nil
	})
	es.Set("onopen", bs.onOpen)
	es.Set("onerror", bs.onError)
	bs.types = append([]string{"message"}, events...)
	for _, t := range bs.types {
		es.Call("addEventListener", t, bs.onEvent)
	}
	return bs, nil
}

type browserStream struct {
	es                       js.Value
	types                    []string
	onOpen, onEvent, onError js.Func
	released                 bool
}

func (s *browserStream) Close() {
	s.es.Call("close") // no event follows close
	s.release()
}

func (s *browserStream) release() {
	if s.released {
		return
	}
	s.released = true
	s.es.Set("onopen", js.Null())
	s.es.Set("onerror", js.Null())
	for _, t := range s.types {
		s.es.Call("removeEventListener", t, s.onEvent)
	}
	s.onOpen.Release()
	s.onEvent.Release()
	s.onError.Release()
}
