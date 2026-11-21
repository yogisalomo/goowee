# Deploying a goowee App

A goowee app is a **single-page application** (SPA). The server must return
`index.html` for any path that doesn't match a static file. Without this,
direct navigation to deep links (e.g. `/todos/42`) returns 404 and the WASM
never loads.

## Why SPA fallback is required

The WASM binary reads `window.location.pathname` at boot via
`router.CurrentPath()`. If the server returns 404 for `/todos/42`, the browser
shows an error page and the app never bootstraps. Serving `index.html` lets the
WASM load, read the actual URL, and render the correct route client-side.

This is the same requirement as any client-side router (React Router, Vue
Router, etc.) — it's not goowee-specific.

## Build your app

Compile to WASM and copy the required files into a directory (here `dist/`):

```sh
# 1. Compile the WASM binary (-tags goowee_nodev leaves out the dev tools;
#    see docs/devtools.md)
GOOS=js GOARCH=wasm go build -tags goowee_nodev -o dist/main.wasm ./cmd/app

# 2. Copy the matching JS loader and goowee bridge runtime
go run github.com/yogisalomo/goowee/cmd/goowee assets ./dist

# 3. Add your index.html (see examples/counter for a template)
cp index.html dist/
```

`assets` takes goowee's version from your `go.mod` and the loader from the Go
toolchain, so the three files in `dist/` always match. A `goowee.js` copied
from another version is rejected at boot; an SSR app using `ssr.Handler` gets
the matching runtime served for it automatically.

Your `dist/` directory should contain:

```
dist/
  index.html
  main.wasm
  wasm_exec.js
  goowee.js
  ... (CSS, images, etc.)
```

Serve `dist/` with any static host configured for SPA fallback.

Ship the `goowee.js` from the same goowee version as `main.wasm`, and update
the two together. They agree on how DOM updates cross from Go: with an older
`goowee.js`, the app still works but falls back to the slower JSON format and
logs a warning in the console.

## Host-specific configurations

### Cloudflare Pages

Enable **Single Page Application** in the dashboard, or add a `_redirects` file
to your publish directory (the `dist/` you built above):

```
/* /index.html 200
```

### Netlify

Add a `_redirects` file to your publish directory (`dist/`):

```
/* /index.html 200
```

Or in `netlify.toml`:

```toml
[[redirects]]
  from = "/*"
  to = "/index.html"
  status = 200
```

### Vercel

Add `vercel.json`:

```json
{
  "rewrites": [{ "source": "/(.*)", "destination": "/index.html" }]
}
```

### GitHub Pages

GitHub Pages has no SPA-fallback setting, but it serves a `404.html` for any
unmatched path — so copying your `index.html` to `404.html` gives you the same
effect: a deep link loads `404.html`, the WASM boots, reads the URL, and renders
the right route. (The response carries a 404 status, which browsers ignore but
crawlers don't — fine for an app, worth knowing for SEO.)

If your site is served under a project sub-path
(`username.github.io/repo/`) rather than a domain root, also set the base path
so asset URLs and routing resolve under that prefix:

```html
<base href="/repo/">
```

goowee reads `<base href>` at boot and matches routes relative to it, so the
same WASM binary works at `/` and under a sub-path. To also have the content
in the first paint, prerender the pages (next section). This repo's `make pages`
target does both; see the `pages` rule in the [Makefile](../Makefile),
[`cmd/site-export`](../cmd/site-export/main.go) and
[`scripts/pages-index.html`](../scripts/pages-index.html).

> Note: goowee routing is path-based (History API), not hash-based — a
> `/#/route` URL will not drive the router.

### Prerendering for static hosts

A client-only site shows a splash until the WASM has downloaded and booted,
and crawlers and link previews see an empty `#root`. `ssr.Export` renders the
pages ahead of time with the same `Page` and `Document` functions
`ssr.Handler` takes. It writes each one as `<path>/index.html` and the
not-found route as `404.html`. The browser shows the content at once and
hydrates it as it would a server response.

```go
pages, err := ssr.Export(ssr.ExportOptions{
    Page:       ssr.RoutedPage(app.App),
    Document:   shell,                    // writes <html>…<base href="/repo/">…<div id="root">body</div>
    Dir:        "dist",
    Paths:      []string{"/"},
    Crawl:      true,                     // and every in-app page those link to
    Base:       "/repo/",                 // "" at a domain root
    ClientOnly: func(r *http.Request) bool { return r.URL.Path == "/editor" },
    Shell:      indexHTML,                // for ClientOnly pages, and renders that panic
})
```

- **Which pages.** `Paths` lists pages explicitly, base-relative. `Crawl`
  follows `<a href>` links within the site. A param route is exported for the
  values something links to (`/users/42`); list any others in `Paths`.
- **Pages nobody exported.** A URL with no file (`/users/43`) gets
  `404.html`. Each file records the path it was rendered for, so the runtime
  sees the mismatch: it drops that markup and renders the right page on the
  client, as the client-only setup would.
- **Directory URLs.** Static hosts serve `about/index.html` at `/about/`,
  usually redirecting `/about` there. The router treats `/about/` as `/about`.
- **Assets.** Export writes only the HTML. Copy `main.wasm`, `wasm_exec.js`,
  `goowee.js` and your stylesheets next to it.

### Self-hosted (Nginx)

Add this to your nginx config:

```nginx
location / {
    try_files $uri $uri/ /index.html;
}
```

See [`docs/docker/`](docker/) for a ready-to-use Docker image.

## Docker

[`docs/docker/`](docker/) has a reference `Dockerfile` and `nginx.conf` you can
copy into your project. Put them next to your built `dist/` and build from that
directory, so `dist/` and `nginx.conf` are both in the build context:

```sh
# layout: ./Dockerfile  ./nginx.conf  ./dist/
docker build -t my-goowee-app .
docker run --rm -p 8080:8080 my-goowee-app
```

Or, during development, serve an existing `dist/` with the config mounted in —
no image build (mount `nginx.conf` too, or you get stock nginx on port 80 with
no SPA fallback):

```sh
docker run --rm -p 8080:8080 \
  -v "$PWD/dist:/usr/share/nginx/html:ro" \
  -v "$PWD/nginx.conf:/etc/nginx/conf.d/default.conf:ro" \
  nginx:alpine
```

See [`docs/docker/README.md`](docker/README.md) for details.

## SSR deployments

The above covers static deployments, client-only or prerendered with
`ssr.Export`. To render each request on a server instead, use
the `ssr-server` binary or write your own HTTP handler — SSR requires a Go
server to run the renderer before sending HTML to the browser.
