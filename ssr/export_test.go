package ssr

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
)

func exportApp(r *router.Router) core.Node {
	return h.Div(
		r.Link("/", "home"), r.Link("/about", "about"), r.Link("/users/7?tab=x#top", "user 7"),
		h.A(h.Href("https://example.com/"), h.Text("external")),
		h.A(h.Href("/goowee/file.pdf"), h.Text("a file")),
		r.Route(map[string]func() core.Node{
			"/":          func() core.Node { return h.P(h.Text("Home page")) },
			"/about":     func() core.Node { return h.P(h.Text("About page"), r.Link("/broken/x/y", "broken")) },
			"/users/:id": func() core.Node { return h.P(h.Text("User " + r.Param("id"))) },
			"/boom":      func() core.Node { panic("boom") },
			"/ai":        func() core.Node { return h.P(h.Text("client only")) },
		}),
	)
}

func testDocument(w io.Writer, head, body string) {
	fmt.Fprintf(w, "<!DOCTYPE html><html><head><base href=\"/goowee/\">%s</head><body><div id=\"root\">%s</div></body></html>", head, body)
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExportCrawlsAndWritesEachPage(t *testing.T) {
	dir := t.TempDir()
	pages, err := Export(ExportOptions{
		Page:       RoutedPage(exportApp),
		Document:   testDocument,
		Dir:        dir,
		Crawl:      true,
		Base:       "/goowee/",
		ClientOnly: func(r *http.Request) bool { return r.URL.Path == "/ai" },
		Shell:      []byte("<!DOCTYPE html><div id=root>shell</div>"),
		Paths:      []string{"/", "/ai"},
	})
	// /broken/x/y is linked but matches no route: skipped and reported.
	if err == nil || !strings.Contains(err.Error(), "/broken/x/y (status 404)") {
		t.Fatalf("err = %v, want the broken link reported", err)
	}
	want := []string{"404.html", "about/index.html", "ai/index.html", "index.html", "users/7/index.html"}
	if got := files(t, dir); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("files %v, want %v", got, want)
	}
	if len(pages) != 5 {
		t.Fatalf("pages %+v", pages)
	}

	home := read(t, dir, "index.html")
	for _, s := range []string{
		`<head><meta name="goowee-prerendered" content="/goowee/">`, // marked for its URL
		`Home page`, HydrationMarker, // a hydratable render
		`href="/goowee/about"`, // links carry the base
	} {
		if !strings.Contains(home, s) {
			t.Errorf("index.html lacks %s:\n%s", s, home)
		}
	}
	if user := read(t, dir, "users/7/index.html"); !strings.Contains(user, "User 7") || !strings.Contains(user, `content="/goowee/users/7"`) {
		t.Errorf("users/7: %s", user)
	}
	if ai := read(t, dir, "ai/index.html"); ai != "<!DOCTYPE html><div id=root>shell</div>" {
		t.Errorf("ai/index.html isn't the shell: %s", ai)
	}
	notFound := read(t, dir, "404.html")
	if !strings.Contains(notFound, "404 — page not found") || !strings.Contains(notFound, `content="/goowee/404.html"`) {
		t.Errorf("404.html: %s", notFound)
	}
}

// A page whose render panics gets the shell, as Handler serves its Fallback;
// without a shell the export fails.
func TestExportPanicFallsBackToTheShell(t *testing.T) {
	dir := t.TempDir()
	pages, err := Export(ExportOptions{
		Page: RoutedPage(exportApp), Document: testDocument, Dir: dir,
		Paths: []string{"/boom"}, Shell: []byte("shell"),
	})
	if err != nil || len(pages) != 2 || !pages[0].ClientOnly || read(t, dir, "boom/index.html") != "shell" {
		t.Fatalf("err=%v pages=%+v", err, pages)
	}
	_, err = Export(ExportOptions{Page: RoutedPage(exportApp), Document: testDocument, Dir: t.TempDir(), Paths: []string{"/boom"}})
	if err == nil || !strings.Contains(err.Error(), "render panicked") {
		t.Fatalf("without a shell: err = %v", err)
	}
}

// A listed path that isn't a page fails the export.
func TestExportListedPathMustRender(t *testing.T) {
	_, err := Export(ExportOptions{Page: RoutedPage(exportApp), Document: testDocument, Dir: t.TempDir(), Paths: []string{"/nope"}})
	if err == nil || !strings.Contains(err.Error(), "/nope: status 404") {
		t.Fatalf("err = %v", err)
	}
}

func TestExportOptionErrors(t *testing.T) {
	if _, err := Export(ExportOptions{}); err == nil {
		t.Fatal("no error without Page, Document and Dir")
	}
	_, err := Export(ExportOptions{Page: RoutedPage(exportApp), Document: testDocument, Dir: t.TempDir(),
		ClientOnly: func(*http.Request) bool { return true }})
	if err == nil || !strings.Contains(err.Error(), "Shell") {
		t.Fatalf("ClientOnly without Shell: %v", err)
	}
	// A NotFoundPath that some route matches isn't a 404 page.
	_, err = Export(ExportOptions{Page: RoutedPage(exportApp), Document: testDocument, Dir: t.TempDir(), NotFoundPath: "/about"})
	if err == nil || !strings.Contains(err.Error(), "not 404") {
		t.Fatalf("NotFoundPath matching a route: %v", err)
	}
}

// Without a base, paths and links are at the domain root, and Handler's
// requests carry no base.
func TestExportAtTheRoot(t *testing.T) {
	dir := t.TempDir()
	if _, err := Export(ExportOptions{Page: RoutedPage(exportApp), Document: testDocument, Dir: dir, Paths: []string{"/about"}}); err != nil {
		t.Fatal(err)
	}
	about := read(t, dir, "about/index.html")
	if !strings.Contains(about, `href="/users/7?tab=x#top"`) || !strings.Contains(about, `content="/about"`) {
		t.Fatalf("about: %s", about)
	}
	req, _ := http.NewRequest("GET", "/", nil)
	if BasePath(req) != "" {
		t.Fatal("a plain request has a base path")
	}
}

func TestMarkPrerendered(t *testing.T) {
	for in, want := range map[string]string{
		`<html><HEAD lang="x"><title>t</title>`: `<html><HEAD lang="x"><meta name="goowee-prerendered" content="/a&amp;b"><title>t</title>`,
		`<p>no head</p>`:                        `<meta name="goowee-prerendered" content="/a&amp;b"><p>no head</p>`,
	} {
		if got := string(markPrerendered([]byte(in), "/a&b")); got != want {
			t.Errorf("markPrerendered(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinks(t *testing.T) {
	body := `<a href="/goowee/x">x</a><a class="c" href="/goowee/y/?q=1#f">y</a><a href="/other">o</a>` +
		`<a href="https://e.com/goowee/z">e</a><a href="//cdn/goowee/q">c</a><a href="/goowee/doc.pdf">f</a>` +
		`<link href="/goowee/site.css"><a href="/goowee">root</a><a href="/goowee/a&amp;b">amp</a>`
	got := strings.Join(links(body, "/goowee"), " ")
	if want := "/x /y / /a&b"; got != want {
		t.Fatalf("links = %q, want %q", got, want)
	}
}
