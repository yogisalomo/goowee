//go:build !(js && wasm)

package router

import "net/url"

// NewURL creates a router positioned at u's path and query — what a server
// renders for a request: router.NewURL(req.URL). It is not in browser
// builds, which then don't link net/url (#126); use New there.
func NewURL(u *url.URL) *Router {
	r := New(u.Path)
	r.setQuery(u.RawQuery)
	r.Fragment.Set(u.Fragment)
	return r
}
