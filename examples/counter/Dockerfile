# Dockerfile for the goowee counter example — serves the built site with nginx.
# `make docker` builds the example into ./dist, then builds and runs this image.
# The same Dockerfile + nginx.conf are documented as a reusable sample in
# docs/docker/. Build context is this directory (examples/counter), so ./dist
# and ./nginx.conf are both present.
FROM nginx:alpine

# Replace nginx's default server with the SPA-aware config.
RUN rm /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/

# Built site (index.html, main.wasm, wasm_exec.js, goowee.js, counter.js, site.css).
COPY dist/ /usr/share/nginx/html/

EXPOSE 8080
