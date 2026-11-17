# Request: SPA Deployment Documentation & Minimal Docker/Nginx Setup

## Summary

goowee apps are SPAs — the WASM reads `window.location.pathname` on boot to match routes. This means the server **must** serve `index.html` for all paths that don't match a static file, otherwise direct navigation to deep links (e.g. `/blog/my-post`) returns 404 before the WASM even loads.

Currently there is no guidance on how to deploy a goowee app to a static host, and users hit this 404 wall immediately. This request covers two things:

1. **Documentation** — a "Deployment" section in the docs explaining the SPA requirement and how to configure it on popular hosts.
2. **Reference Docker image** — a minimal nginx-based Docker image that serves a goowee app with correct SPA fallback, so users who self-host have a working starting point.

---

## 1. Documentation: Deployment Guide

Add a `docs/deployment.md` (or a section in the existing docs) covering:

### The Core Requirement

> A goowee app is a single-page application. The server must serve `index.html` for any path that doesn't match a static file. Without this, direct navigation to deep links (e.g. `/todos/42`) returns 404.

### Why This Exists

The WASM binary reads `window.location.pathname` at boot via `router.CurrentPath()`. If the server returns 404 for `/todos/42`, the browser shows an error page and the WASM never loads. Serving `index.html` lets the WASM bootstrap, read the actual URL, and render the correct route client-side.

### Host-Specific Configurations

#### Cloudflare Pages

Enable "Single Page Application" in the dashboard, or add a `public/_redirects` file:

```
/* /index.html 200
```

#### Netlify

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

#### Vercel

Add `vercel.json`:

```json
{
  "rewrites": [{ "source": "/(.*)", "destination": "/index.html" }]
}
```

#### GitHub Pages

GitHub Pages does not support SPA fallback natively. Options:
- Use hash-based routing (`/#/todos/42`) instead of path-based routing.
- Deploy to Cloudflare Pages or Netlify instead (free tier, same as GitHub Pages).

#### Self-Hosted (Nginx)

See the Docker image below, or add this to your nginx config:

```nginx
location / {
    try_files $uri $uri/ /index.html;
}
```

---

## 2. Reference Docker Image

A minimal nginx-based image that serves a goowee app with SPA fallback. This gives users a one-command way to run their app locally or self-host.

### Dockerfile

```dockerfile
FROM nginx:alpine

# Remove default config
RUN rm /etc/nginx/conf.d/default.conf

# Add SPA-aware nginx config
COPY nginx.conf /etc/nginx/conf.d/

# App files go in /usr/share/nginx/html
COPY dist/ /usr/share/nginx/html/

EXPOSE 8080
```

### nginx.conf

```nginx
server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # SPA fallback: serve index.html for any path that doesn't match a file
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Cache static assets aggressively (they're content-hashed or versioned)
    location /static/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # WASM needs correct MIME type (nginx serves it correctly by default,
    # but be explicit for clarity)
    location ~* \.wasm$ {
        application/wasm;
    }
}
```

### Usage

Build the app, copy output into `dist/`, then:

```bash
docker build -t my-goowee-app .
docker run -p 8080:8080 my-goowee-app
```

Or as a one-liner during development:

```bash
docker run --rm -p 8080:8080 -v ./dist:/usr/share/nginx/html:ro nginx:alpine
```

### Makefile Target (suggested)

Add to goowee's Makefile or document it for app authors:

```makefile
docker:
	docker build -t $(APP_NAME) .
	docker run --rm -p 8080:8080 $(APP_NAME)
```

---

## Files to Create/Update

| File | Action |
|---|---|
| `docs/deployment.md` | Create — full deployment guide |
| `docs/docker/Dockerfile` | Create — minimal nginx SPA image |
| `docs/docker/nginx.conf` | Create — SPA fallback nginx config |
| `AGENTS.md` | Update — add a note under "Project setup" pointing to deployment docs |

---

## Notes

- The nginx image should be `nginx:alpine` (~2MB base). The total image with a typical goowee app (4MB WASM + assets) will be under 10MB.
- The `try_files` directive is the standard nginx approach for SPA fallback. It checks for the exact file, then directory, then falls back to `index.html`.
- The WASM MIME type (`application/wasm`) is served correctly by nginx by default, but being explicit in the config prevents confusion.
- No SSR configuration is needed for the client-only deployment. SSR deployments would need a different setup (the existing `ssr-server` binary).
