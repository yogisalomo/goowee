//go:build !(js && wasm)

package net

import "context"

// platform has no network outside the browser: SSR and native tests (see
// SetTransport and gooweetest.NewFakeNet).
var platform Transport = unavailable{}

type unavailable struct{}

func (unavailable) Fetch(context.Context, Request) (Response, error) {
	return Response{}, ErrUnavailable
}

func (unavailable) Dial(string, []string, ConnEvents) (Conn, error) {
	return nil, ErrUnavailable
}
