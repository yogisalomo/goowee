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
# 1. Compile the WASM binary
GOOS=js GOARCH=wasm go build -o dist/main.wasm ./cmd/app

# 2. Copy Go's JS loader
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/

# 3. Copy the goowee bridge runtime
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js dist/

# 4. Add your index.html (see examples/counter for a template)
cp index.html dist/
```

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
same WASM binary works at `/` and under a sub-path. This repo's `make pages`
target does exactly this — see the `pages` rule in the [Makefile](../Makefile)
and [`scripts/pages-index.html`](../scripts/pages-index.html) for a working
template (it substitutes the base path and copies `index.html` to `404.html`).

> Note: goowee routing is path-based (History API), not hash-based — a
> `/#/route` URL will not drive the router.

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

The above covers client-only (static) deployments. For SSR + hydration, use
the `ssr-server` binary or write your own HTTP handler — SSR requires a Go
server to run the renderer before sending HTML to the browser.
