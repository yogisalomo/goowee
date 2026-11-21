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
| identity (uncompressed) | ~5 223 KB | 1.0× |
| **gzip** (`-9`)          | ~1 363 KB | **3.8×** |
| **brotli** (`-q 11`)     |   ~988 KB | **5.3×** |

(Go 1.25, goowee 0.3.0; tracked per release in `docs/perf/`.) At a bandwidth-limited **500 KB/s** the download alone
goes from **~10.4 s → ~2.7 s** with gzip, and **~2.0 s** with brotli. Serve `application/wasm` with `Content-Encoding: gzip` (or `br`) and a
`Vary: Accept-Encoding` header, and the browser decompresses transparently before
`instantiateStreaming` compiles it. This is a zero-code-risk win — do it before
reaching for TinyGo.

**Send `Content-Type: application/wasm`.** It lets the browser compile the
module while it downloads. With any other type, `goowee.boot()` still starts
the app, but downloads the whole file before compiling it and warns in the
console which header to fix. A wasm URL that answers with an HTML page (an
SPA fallback catching it) or an error status stops the boot with one console
error saying so, a `goowee:boot-error` event on `window`, and a rejected
`goowee.boot()` promise, e.g. to replace the loading placeholder:

```html
<script>
  goowee.boot().catch(() => {
    document.getElementById("root").textContent = "Couldn't load the app. Reload to try again.";
  });
</script>
```

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
the SSR HTML whenever the client accepts gzip. Brotli isn't in Go's standard
library, so for brotli the server serves a pre-built sibling: build it with
`make brotli` (`brotli -q 11 -k main.wasm`, ~11 s) and every browser that sends
`Accept-Encoding: br` gets `main.wasm.br` — 27% fewer bytes than gzip. A
sibling older than `main.wasm` (left over from a previous build) is ignored, so
a stale copy is never served: `make brotli serve-ssr`.

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
binary is served compressed, the next lever is the raw binary size itself:
build with TinyGo (next section), or split the code by route.

## What not to import: `net/http`

The binary is the app's one big download, so watch what you pull in. The
costliest trap is `net/http`: it works under `GOOS=js` (it's implemented on the
browser's `fetch`), but it drags in `crypto/tls`, `net`, and the HTTP/2 stack.
Measured with Go 1.25: a minimal WASM program that makes one request is
**10.0 MB with `net/http`** and **2.7 MB calling `fetch` through `syscall/js`**
— about 7 MB for one import, before goowee is even involved.

Use goowee's `net` package instead. It wraps the browser's `fetch` and
`WebSocket` (which `net/http` doesn't offer at all), and costs a goowee app
about 80 KB raw, 12 KB brotli-compressed (measured with an app that fetches
and opens a reconnecting socket, against the same app without them). `Fetch`
takes a context, so it fits `hooks.UseResource`, whose context aborts the
request on refetch and unmount:

```go
import "github.com/yogisalomo/goowee/net"

user := hooks.UseResource(nil, func(ctx context.Context) (User, error) {
    var u User
    resp, err := net.Fetch(ctx, net.Request{URL: "/api/user"})
    if err == nil {
        err = resp.JSON(&u) // a non-2xx status comes back as *net.StatusError
    }
    return u, err
})
```

For a POST, set `Method`, and `JSON` (or `Body`) and `Header`, on the
`net.Request`. `net.Dial` opens a WebSocket whose callbacks run on the render
loop and which can reconnect with backoff; see the
[API reference](../api/reference.md#net--fetch-and-websocket).

Check the effect of any new dependency with `make size-report`; CI holds each
program to its brotli size in `docs/perf/size-budget.json`.

## Building with TinyGo

TinyGo compiles goowee apps too, and its binaries are 3 to 4 times smaller
because it doesn't carry the standard Go runtime. CI builds the example with
TinyGo on every pull request and runs the smoke and devtools E2E against it.

```sh
tinygo build -target wasm -opt z -no-debug -stack-size=1MB -o web/main.wasm ./cmd/app
cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" web/
```

| brotli -q 11 | Go 1.25 | TinyGo 0.44 |
|---|---:|---:|
| minimal app (`test/e2e/hello`) | 797 KB | 194 KB |
| tutorial site (`examples/counter`) | 989 KB | 299 KB |

- **Use TinyGo 0.44 or later.** goowee needs `recover` on wasm alongside
  goroutines (error boundaries and contained handler panics do), which TinyGo
  has had since 0.43.
- **`-stack-size=1MB` is required.** TinyGo's default goroutine stack is 64 KB,
  and goowee renders recursively: about 20 nested component, element and scope
  levels overflow it, and TinyGo notices only after memory is corrupted. 1 MB
  renders 200 levels.
- **Serve TinyGo's own `wasm_exec.js`,** not the one from your Go
  installation. The two aren't interchangeable.
- **Speed.** Measured as wasm under Node, rendering is on par with the
  standard toolchain (a 100-row re-render: 392 µs against 401 µs). Signal
  fan-out is about 1.8× slower (1,000 subscribers: 6.8 µs against 3.8 µs).
- **What's missing.** TinyGo has no `runtime.Callers`. The dev-mode change log
  shows a write's location as "unknown", and dependency warnings carry no
  file and line. Parts of `reflect` and some `encoding/json` uses are
  unsupported in your own code. `int` is 32 bits on TinyGo's wasm target.

**Which to choose:** TinyGo when the download matters most (mobile, first
visits, many small apps); the standard toolchain when you depend on packages
TinyGo can't compile, or want the full dev-mode locations while developing.
Many projects develop on Go and ship TinyGo builds; run your E2E on the build
you ship.

