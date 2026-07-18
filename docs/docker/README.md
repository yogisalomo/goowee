# Docker deployment for goowee apps

A minimal, **reference** nginx-based image that serves a goowee app with SPA
fallback. Copy the `Dockerfile` and `nginx.conf` into your own project and adapt
them — this directory is a sample, not something this repo builds.

## Quick start

Build your app into a `dist/` directory (see [`../deployment.md`](../deployment.md)),
put the `Dockerfile` and `nginx.conf` next to it, then build from that directory
so `dist/` and `nginx.conf` are both in the build context:

```sh
docker build -t my-goowee-app .
docker run --rm -p 8080:8080 my-goowee-app
```

Open http://localhost:8080.

## Development (no build step)

Serve an existing `dist/` with the sample config mounted in — no image build.
Mount `nginx.conf` too, otherwise you'd get stock nginx (port 80, no SPA
fallback):

```sh
docker run --rm -p 8080:8080 \
  -v "$PWD/dist:/usr/share/nginx/html:ro" \
  -v "$PWD/nginx.conf:/etc/nginx/conf.d/default.conf:ro" \
  nginx:alpine
```

## What it does

- Serves static files from `/usr/share/nginx/html`
- Falls back to `index.html` for any path that doesn't match a file (SPA routing)
- Serves the app shell (`index.html`) with `Cache-Control: no-cache` so redeploys are picked up
- Caches built assets (`.wasm`, `.js`, `.css`, images) for a short TTL — not
  `immutable`, because goowee's default output uses fixed filenames
- Relies on nginx's bundled `application/wasm` MIME type (no extra config needed)

## Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Reference image (nginx:alpine base) |
| `nginx.conf` | SPA-aware nginx configuration |

## Why SPA fallback?

goowee apps are single-page applications. The WASM binary reads
`window.location.pathname` at boot to determine which route to render. If the
server returns 404 for a deep link like `/blog/my-post`, the browser shows an
error and the WASM never loads. The `try_files` directive serves `index.html`
for unmatched paths, letting the WASM bootstrap and handle routing
client-side.
