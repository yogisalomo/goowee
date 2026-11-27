// Command site-export prerenders the tutorial site (examples/counter) into a
// static directory with ssr.Export, for GitHub Pages: `make pages` runs it,
// then `goowee build --web` adds the production wasm and runtime under
// content-hashed names and points these pages at them.
// Every page reachable by links from "/" is written as <path>/index.html, the
// not-found route as 404.html, and /ai (whose <textarea> can't carry
// hydration markers, ADR-009) as the client-rendered shell.
//
//	go run ./cmd/site-export -out _site -base /goowee/ -shell scripts/pages-index.html
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/yogisalomo/goowee/examples/counter/app"
	"github.com/yogisalomo/goowee/ssr"
)

func main() {
	out := flag.String("out", "_site", "directory to write")
	base := flag.String("base", "/", "URL prefix the site is served under")
	shellPath := flag.String("shell", "scripts/pages-index.html", "client-rendered shell, with __BASE__ for the base")
	flag.Parse()

	b := *base
	if !strings.HasSuffix(b, "/") {
		b += "/"
	}
	raw, err := os.ReadFile(*shellPath)
	if err != nil {
		log.Fatal(err)
	}
	shell := []byte(strings.ReplaceAll(string(raw), "__BASE__", b))

	pages, err := ssr.Export(ssr.ExportOptions{
		Page:       ssr.RoutedPage(app.App),
		Document:   document(b),
		Dir:        *out,
		Paths:      []string{"/"},
		Crawl:      true,
		Base:       b,
		ClientOnly: func(r *http.Request) bool { return r.URL.Path == "/ai" },
		Shell:      shell,
	})
	if err != nil {
		log.Fatal(err)
	}
	prerendered := 0
	for _, p := range pages {
		if !p.ClientOnly {
			prerendered++
		}
	}
	fmt.Printf("exported %d pages to %s (%d prerendered, %d client-only)\n", len(pages), *out, prerendered, len(pages)-prerendered)
}

// document writes a page the way scripts/pages-index.html does, with the
// server-rendered head metadata and body: the <base> that makes assets and
// routes resolve under the project path, the stylesheet, and the scripts.
// Asset names are quoted as written so goowee build can rewrite them to the
// hashed files (the wasm too: goowee.boot's default name isn't in the page).
func document(base string) func(w io.Writer, head, body string) {
	return func(w io.Writer, head, body string) {
		if head == "" {
			head = `    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>goowee — reactive Go UIs in WebAssembly</title>`
		}
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <base href="%s">
%s
    <link rel="preload" href="main.wasm" as="fetch" type="application/wasm" crossorigin fetchpriority="low">
    <link rel="stylesheet" href="site.css">
    <script src="wasm_exec.js"></script>
    <script src="goowee.js"></script>
</head>
<body>
    <div id="root">%s</div>
    <script>goowee.boot({when: "idle", wasm: "main.wasm"});</script>
</body>
</html>
`, base, head, body)
	}
}
