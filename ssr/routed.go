//go:build !(js && wasm)

package ssr

import (
	"net/http"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/router"
)

// RoutedPage adapts a router-based app — app(router) returns the root node —
// into a Page func: the router starts at the request's path and query, and
// the response is 404 when the router fell through to its not-found route.
// Under Export with a Base, the router gets that base path, so its links
// carry it.
func RoutedPage(app func(*router.Router) core.Node) func(*http.Request) Page {
	return func(r *http.Request) Page {
		rtr := router.NewURL(r.URL)
		if b := BasePath(r); b != "" {
			rtr.SetBasePath(b)
		}
		return Page{
			Node: app(rtr),
			Status: func() int {
				if rtr.NotFound() {
					return http.StatusNotFound
				}
				return http.StatusOK
			},
		}
	}
}

// (Host builds only, like router.NewURL: the browser has no request to
// route, and the package's tests compile for WASM.)
