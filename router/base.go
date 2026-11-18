package router

import "strings"

// stripBase removes the base prefix from a browser pathname, yielding the
// base-relative path the router matches against. "/goowee/counter" with base
// "/goowee" → "/counter"; the base itself → "/". A pathname outside the base is
// returned unchanged (shouldn't happen in practice).
func stripBase(pathname, base string) string {
	if base == "" {
		return pathname
	}
	if pathname == base {
		return "/"
	}
	if strings.HasPrefix(pathname, base+"/") {
		return pathname[len(base):]
	}
	return pathname
}

// trimSlash drops a trailing slash ("/counter/" → "/counter"), which static
// hosts add when they serve a directory's index.html, so the path is the one
// the app's links and the prerendered page used.
func trimSlash(p string) string {
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		if p = strings.TrimRight(p, "/"); p == "" {
			return "/"
		}
	}
	return p
}
