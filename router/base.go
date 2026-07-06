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
