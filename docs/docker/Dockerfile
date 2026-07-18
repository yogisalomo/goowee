# Reference Dockerfile for serving a goowee app (an SPA) with nginx.
# This is a sample to copy into your own project, not something this repo builds.
#
# Expected build context: a directory that contains this Dockerfile, the
# accompanying nginx.conf, and your built site in ./dist. The simplest setup is
# to copy this Dockerfile and nginx.conf next to your dist/, then run:
#
#   docker build -t my-goowee-app .
#   docker run --rm -p 8080:8080 my-goowee-app
#
# See ../deployment.md for how to populate dist/.
FROM nginx:alpine

# Replace nginx's default server with the SPA-aware config.
RUN rm /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/

# App files (index.html, main.wasm, wasm_exec.js, goowee.js, assets).
COPY dist/ /usr/share/nginx/html/

EXPOSE 8080
