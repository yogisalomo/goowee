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

Enable **Single Page Application** in the dashboard, or add a
`public/_redirects` file:

```
/* /index.html 200
```

### Netlify

Add a `public/_redirects` file:

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

GitHub Pages does not support SPA fallback natively. Options:

- Use hash-based routing (`/#/todos/42`) instead of path-based routing.
- Deploy to Cloudflare Pages or Netlify instead (free tier, same as GitHub Pages).

### Self-hosted (Nginx)

Add this to your nginx config:

```nginx
location / {
    try_files $uri $uri/ /index.html;
}
```

See [`docs/docker/`](docker/) for a ready-to-use Docker image.

## Docker

A minimal nginx-based Docker image is provided in `docs/docker/`:

```sh
# Build and run
docker build -t my-goowee-app -f docs/docker/Dockerfile .
docker run -p 8080:8080 my-goowee-app
```

Or during development:

```sh
docker run --rm -p 8080:8080 -v ./dist:/usr/share/nginx/html:ro nginx:alpine
```

See [`docs/docker/README.md`](docker/README.md) for details.

## SSR deployments

The above covers client-only (static) deployments. For SSR + hydration, use
the `ssr-server` binary or write your own HTTP handler — SSR requires a Go
server to run the renderer before sending HTML to the browser.
