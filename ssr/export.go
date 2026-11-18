package ssr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yogisalomo/goowee/core"
)

// ExportOptions configures Export.
type ExportOptions struct {
	// Page and Document are the same functions Handler takes (required).
	Page     func(r *http.Request) Page
	Document func(w io.Writer, head, body string)

	// Dir is where the site is written (required). It is created if needed;
	// files already there are overwritten, others are left alone.
	Dir string

	// Paths are the pages to export, base-relative: "/", "/about",
	// "/users/42". Default: just "/".
	Paths []string

	// Crawl also exports every in-app page the exported pages link to (<a
	// href> within Base), following links from those in turn. Param routes
	// are exported for the values the site links to; list others in Paths.
	Crawl bool

	// Base is the URL prefix the site is served under: "/goowee/" for a
	// GitHub Pages project site, "" or "/" at a domain root. Paths stay
	// base-relative; RoutedPage gives the base to the router, so links carry
	// it (see BasePath for a custom Page func), and Document must emit the
	// matching <base href>.
	Base string

	// ClientOnly selects paths that can't be prerendered faithfully, as for
	// Handler; they get Shell instead of a render.
	ClientOnly func(r *http.Request) bool

	// Shell is the client-rendered document (the app's index.html), written
	// for ClientOnly paths and, as Handler serves its Fallback, for pages
	// whose render panics. Required when ClientOnly is set.
	Shell []byte

	// NotFoundPath is a path no route matches, rendered as 404.html, which
	// static hosts serve for unknown URLs. Default "/404.html".
	NotFoundPath string
}

// ExportedPage reports one page Export wrote.
type ExportedPage struct {
	Path       string // base-relative path
	File       string // file written, relative to Dir
	ClientOnly bool   // Shell was written instead of a render (ClientOnly, or the render panicked)
}

// Export prerenders a site to static files for hosts without a Go server
// (GitHub Pages, Netlify, S3): each path is rendered with Page and Document,
// exactly as Handler would serve it, and written to Dir/<path>/index.html,
// plus Dir/404.html from the not-found route. The client hydrates the files
// as it hydrates a server response, so the first paint has the page's content
// and crawlers and link previews see it.
//
//	pages, err := ssr.Export(ssr.ExportOptions{
//	    Page:     ssr.RoutedPage(app.App),
//	    Document: shell,
//	    Dir:      "_site",
//	    Paths:    []string{"/"},
//	    Crawl:    true,
//	    Base:     "/goowee/",
//	})
//
// Each file records the path it was rendered for (a goowee-prerendered meta
// tag). A static host serves 404.html for any URL it has no file for, which
// includes a param route nobody linked to; the runtime then sees the page was
// rendered for another path, drops the markup and renders on the client,
// rather than hydrating the wrong page.
//
// A page whose render panics gets Shell, as Handler serves its Fallback (and
// fails the export without one). A page whose status isn't 200 fails the
// export when it was listed in Paths; one found by crawling is skipped and
// reported in the error. Export writes the HTML documents only: copy the wasm, the
// runtime and other assets next to them.
func Export(opts ExportOptions) ([]ExportedPage, error) {
	if opts.Page == nil || opts.Document == nil || opts.Dir == "" {
		return nil, errors.New("ssr.Export: Page, Document and Dir are required")
	}
	if opts.ClientOnly != nil && opts.Shell == nil {
		return nil, errors.New("ssr.Export: ClientOnly needs a Shell to write for those paths")
	}
	base := strings.TrimSuffix(opts.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	paths := opts.Paths
	if len(paths) == 0 {
		paths = []string{"/"}
	}
	notFound := opts.NotFoundPath
	if notFound == "" {
		notFound = "/404.html"
	}

	var (
		out     []ExportedPage
		skipped []string
		seen    = map[string]bool{}
		queue   []string
		listed  = map[string]bool{}
	)
	for _, p := range paths {
		p = cleanPath(p)
		listed[p] = true
		if !seen[p] {
			seen[p] = true
			queue = append(queue, p)
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		req := exportRequest(p, base)
		if opts.ClientOnly != nil && opts.ClientOnly(req) {
			file, err := write(opts.Dir, p, opts.Shell)
			if err != nil {
				return out, err
			}
			out = append(out, ExportedPage{Path: p, File: file, ClientOnly: true})
			continue
		}
		doc, body, status, err := renderDocument(opts, req, base+p)
		if errors.Is(err, errPanicked) && opts.Shell != nil {
			// As Handler does: a render that panics falls back to the
			// client-rendered shell.
			core.Log(core.LogRecoverRender, "prerender panicked; writing the client-rendered shell", map[string]any{
				"path": p, "panic": err.Error(),
			})
			file, werr := write(opts.Dir, p, opts.Shell)
			if werr != nil {
				return out, werr
			}
			out = append(out, ExportedPage{Path: p, File: file, ClientOnly: true})
			continue
		}
		if err == nil && status != http.StatusOK {
			err = fmt.Errorf("status %d", status)
		}
		if err != nil {
			if listed[p] {
				return out, fmt.Errorf("ssr.Export %s: %w", p, err)
			}
			skipped = append(skipped, p+" ("+err.Error()+")")
			continue
		}
		file, err := write(opts.Dir, p, doc)
		if err != nil {
			return out, err
		}
		out = append(out, ExportedPage{Path: p, File: file})
		if opts.Crawl {
			for _, link := range links(body, base) {
				if !seen[link] {
					seen[link] = true
					queue = append(queue, link)
				}
			}
		}
	}

	// 404.html: the not-found route, rendered for a path nothing matches.
	doc, _, status, err := renderDocument(opts, exportRequest(cleanPath(notFound), base), base+cleanPath(notFound))
	if err != nil {
		return out, fmt.Errorf("ssr.Export 404.html: %w", err)
	}
	if status != http.StatusNotFound {
		return out, fmt.Errorf("ssr.Export 404.html: %s rendered with status %d, not 404; set NotFoundPath to a path no route matches", notFound, status)
	}
	if err := os.WriteFile(filepath.Join(opts.Dir, "404.html"), doc, 0o644); err != nil {
		return out, err
	}
	out = append(out, ExportedPage{Path: notFound, File: "404.html"})

	if len(skipped) > 0 {
		sort.Strings(skipped)
		return out, fmt.Errorf("ssr.Export: skipped linked pages that didn't render: %s", strings.Join(skipped, ", "))
	}
	return out, nil
}

var errPanicked = errors.New("render panicked")

type baseKey struct{}

// BasePath returns the URL prefix Export is rendering for ("/goowee", or ""
// at a domain root). RoutedPage passes it to the router; a custom Page func
// should do the same with router.SetBasePath, so links carry the prefix.
// Requests served by Handler have none.
func BasePath(r *http.Request) string {
	b, _ := r.Context().Value(baseKey{}).(string)
	return b
}

func exportRequest(p, base string) *http.Request {
	ctx := context.WithValue(context.Background(), baseKey{}, base)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://export.invalid"+p, nil)
	if err != nil {
		panic("ssr.Export: " + err.Error()) // p was cleaned; can't happen
	}
	return req
}

// renderDocument renders one page into a full document, marked with the URL
// path it was rendered for.
func renderDocument(opts ExportOptions, req *http.Request, urlPath string) (doc []byte, body string, status int, err error) {
	page, body, head, rec := render(opts.Page, req)
	if rec != nil {
		return nil, "", 0, fmt.Errorf("%w: %v", errPanicked, rec)
	}
	status = http.StatusOK
	if page.Status != nil {
		if s := page.Status(); s != 0 {
			status = s
		}
	}
	var buf bytes.Buffer
	opts.Document(&buf, head, body)
	return markPrerendered(buf.Bytes(), urlPath), body, status, nil
}

// markPrerendered adds <meta name="goowee-prerendered" content="/path"> at the
// start of the document's <head>, where the runtime looks for it.
func markPrerendered(doc []byte, urlPath string) []byte {
	meta := `<meta name="goowee-prerendered" content="` + html.EscapeString(urlPath) + `">`
	lower := bytes.ToLower(doc)
	i := bytes.Index(lower, []byte("<head"))
	if i >= 0 {
		if j := bytes.IndexByte(doc[i:], '>'); j >= 0 {
			at := i + j + 1
			return append(append(append([]byte{}, doc[:at]...), meta...), doc[at:]...)
		}
	}
	return append([]byte(meta), doc...)
}

// write stores doc as the file for base-relative path p.
func write(dir, p string, doc []byte) (string, error) {
	rel := "index.html"
	if p != "/" {
		rel = filepath.Join(filepath.FromSlash(strings.TrimPrefix(p, "/")), "index.html")
	}
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), os.WriteFile(full, doc, 0o644)
}

// cleanPath makes p an absolute, clean path without a trailing slash.
func cleanPath(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return path.Clean("/" + p)
}

var anchorHref = regexp.MustCompile(`<a\s[^>]*?href="([^"]*)"`)

// links returns the base-relative paths of the in-app pages body links to:
// <a href> values under base, without query or fragment, that don't look like
// files (no extension in the last segment).
func links(body, base string) []string {
	var out []string
	for _, m := range anchorHref.FindAllStringSubmatch(body, -1) {
		href := html.UnescapeString(m[1])
		if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
			continue // external, protocol-relative or relative: not crawled
		}
		if base != "" {
			if href != base && !strings.HasPrefix(href, base+"/") {
				continue
			}
			href = strings.TrimPrefix(href, base)
		}
		p := cleanPath(href)
		if path.Ext(p) != "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
