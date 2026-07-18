# Docker deployment for goowee apps

A minimal nginx-based Docker image that serves a goowee app with SPA fallback.

## Quick start

Build your app into `dist/`, then:

```sh
docker build -t my-goowee-app .
docker run -p 8080:8080 my-goowee-app
```

Open http://localhost:8080.

## Development (no build step)

Mount your `dist/` directory directly:

```sh
docker run --rm -p 8080:8080 -v ./dist:/usr/share/nginx/html:ro nginx:alpine
```

## What it does

- Serves static files from `/usr/share/nginx/html`
- Falls back to `index.html` for any path that doesn't match a file (SPA routing)
- Sets correct MIME type for `.wasm` files
- Caches `/static/` assets for 1 year

## Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Builds the image (nginx:alpine base, ~2MB) |
| `nginx.conf` | SPA-aware nginx configuration |

## Why SPA fallback?

goowee apps are single-page applications. The WASM binary reads
`window.location.pathname` at boot to determine which route to render. If the
server returns 404 for a deep link like `/blog/my-post`, the browser shows an
error and the WASM never loads. The `try_files` directive serves `index.html`
for unmatched paths, letting the WASM bootstrap and handle routing
client-side.
