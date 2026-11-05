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
