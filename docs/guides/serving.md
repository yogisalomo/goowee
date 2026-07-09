# Serving a goowee app — compression

A goowee app ships one large asset: the WebAssembly binary (~4 MB raw for the
example app). Download+compile of that binary is **~65–75% of time-to-interactive**
(see `docs/plans/boot-latency-measurement.md`), so how you serve it is the single
biggest lever on boot latency — bigger, and far cheaper, than shrinking the
binary.

## The one thing that matters: compress the `.wasm`

WebAssembly is highly compressible. For the example binary:

| encoding | bytes on the wire | vs. raw |
|----------|------------------:|--------:|
| identity (uncompressed) | ~4 181 KB | 1.0× |
| **gzip** (`-9`)          | ~1 138 KB | **3.7×** |
| brotli (`-q 11`)         | ~1 000 KB | ~4.2× |

At a bandwidth-limited **500 KB/s** the download alone goes from **~8.0 s → ~2.1 s**
with gzip. Serve `application/wasm` with `Content-Encoding: gzip` (or `br`) and a
`Vary: Accept-Encoding` header, and the browser decompresses transparently before
`instantiateStreaming` compiles it. This is a zero-code-risk win — do it before
reaching for TinyGo.

Measure the effect yourself with the boot harness under a throttled network:

```sh
THROTTLE=4g make boot     # or THROTTLE=fast3g / slow3g
```

## By host

**GitHub Pages (and most CDNs) — automatic.** GitHub's CDN gzips `application/wasm`
on the fly when the client sends `Accept-Encoding: gzip`; the live demo already
serves the binary at ~1.1 MB. Nothing to configure. (Pages does gzip, not brotli,
for wasm.)

**The reference SSR server (`cmd/ssr-server`) — built in.** It gzips the wasm
(cached, compressed once at max level, invalidated on rebuild), the JS/CSS, and
the SSR HTML whenever the client accepts gzip. It uses only `compress/gzip` from
the standard library — no dependency — so brotli is left to a real CDN/proxy in
front of it.

**nginx** — enable static pre-compressed serving so nginx streams a
pre-built `.gz`/`.br` sibling instead of recompressing per request:

```nginx
gzip_static  on;   # serves main.wasm.gz when present
brotli_static on;  # serves main.wasm.br when present (ngx_brotli)
types { application/wasm wasm; }
```

Generate the siblings at build time: `gzip -9 -k main.wasm` and, if you have it,
`brotli -q 11 -k main.wasm`.

**A plain static file server you control.** Either compress on the fly (like the
reference server) or pre-compress and set `Content-Encoding` yourself. Pure static
hosts that can't set response headers rely on the host/CDN doing it — check the
response headers with `curl -sI -H 'Accept-Encoding: gzip' <url>/main.wasm` and
look for `content-encoding: gzip`.

## What compression does *not* fix

Compression shrinks the download; it does not shrink the *compiled* module or the
Go runtime boot (~40 ms) — those are already small relative to download. Once the
binary is served compressed, the next lever is the raw binary size itself (3.1:
TinyGo, code splitting), evaluated against the *post-compression* number — the
incremental win from TinyGo is smaller once you're already at ~1 MB on the wire.
