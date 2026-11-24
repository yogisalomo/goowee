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
| identity (uncompressed) | ~5 741 KB | 1.0× |
| **gzip** (`-9`)          | ~1 481 KB | **3.9×** |
| **brotli** (`-q 11`)     | ~1 064 KB | **5.4×** |

(Go 1.25, goowee 0.4.0; tracked per release in `docs/perf/`.) At a bandwidth-limited **500 KB/s** the download alone
goes from **~11.5 s → ~3.0 s** with gzip, and **~2.1 s** with brotli. Serve `application/wasm` with `Content-Encoding: gzip` (or `br`) and a
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

**What costs what.** goowee's own client code doesn't link `fmt`,
`encoding/json`, `log` or `net/url` (#126), so an app pays for them only
when it uses them. Sizes are brotli, Go 1.25, for the smallest app
(`test/e2e/hello`, 674 KB) plus one use of each:

| The app uses | Brings in | Adds |
|---|---|---|
| `goowee/router` | | ~5 KB |
| `h.Textf`, or `fmt` itself | `fmt` | ~35–40 KB |
| `h.JSONLD`, `core.UseJSON(json.Marshal)` | `encoding/json` (marshal) | ~105 KB |
| `json.Marshal` and `json.Unmarshal` | `encoding/json` | ~135 KB |
| `hooks.UsePersist`, `hooks.Load` | `encoding/json` | ~140 KB |
| `goowee/net` | `encoding/json`, more | ~145 KB |

The costs overlap: an app that already uses `encoding/json` pays for it
once. To format text without fmt, build the string with `strconv` and
bind it with `TextS` (a `core.Computed`), as `test/e2e/hello` does. Bound
strings, numbers and bools are formatted without fmt. A bound slice, map or
struct needs `core.UseFmt(fmt.Sprintf)` (called for you by `Textf`), and a
struct or typed slice as a property value or `ref.Call` argument needs
`core.UseJSON(json.Marshal)`; without them goowee logs a warning.

**Booting the module yourself.** `goowee.boot()` hands the WebAssembly
instance to the runtime, which then calls into Go through the module's exports
(about 20 ns a call) instead of `syscall/js` callbacks (about 18 µs). A page
that instantiates the module with its own code still works; to get the faster
calls, pass the instance to the runtime before running it:

```js
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject);
goowee.useInstance(go, instance);
go.run(instance);
```

**Start the download early: the preload hint.** Without a hint the WASM
download starts only after `wasm_exec.js` and `goowee.js` have loaded and the
shell's `goowee.boot()` runs. A hint in `<head>` starts it with the HTML
parse:

```html
<link rel="preload" href="main.wasm" as="fetch" type="application/wasm" crossorigin>
```

`goowee.boot()` fetches with the credentials mode `crossorigin` gives, so the
browser reuses the preloaded response. The shells `goowee new` writes, and
`examples/counter`'s, have it.

**When a server-rendered page boots: `goowee.boot({when})`.** A
server-rendered page is readable before the WASM arrives, so it can choose
when to boot:

| `when` | Boots | For |
|---|---|---|
| `"load"` (default) | at once | apps, pages you interact with at once |
| `"idle"` | after the load event, when the browser is idle (`idleTimeout`, 2000 ms by default) | pages that are read first; the tutorial site uses it |
| `"visible"` | when `target` (an element or selector, `#root` by default) scrolls into view | a widget further down the page |
| `"interaction"` | at the first pointerdown, keydown or focusin | pages mostly read (docs, a blog), where most visitors never need the app |

Clicks, keys and submits on the server markup before the boot are queued and
replayed once the app is live (#131), so the interaction that starts an
`"interaction"` boot is handled, once. A client-only page has nothing to show
before the boot and always boots at once. `?goowee-boot=idle` (or another
mode) in the URL overrides `when`, to try a mode on a live page.

With a lazy mode, use a low-priority hint (`fetchpriority="low"`) to warm
the cache without competing with the page's CSS, fonts and images, or no
hint at all to download nothing until the boot. With `"interaction"` and no
hint, a visitor who only reads never downloads the WASM
(`test/e2e/lazyboot.mjs` checks that nothing is requested before the first
click).

Measured with `THROTTLE=fast3g make boot` on the tutorial site's home page
(one stylesheet, a low-priority hint, 1.4 MB of WASM, medians of 3 cold
loads; times from navigation start):

| mode | FCP | load event | boot starts | TTI |
|---|---|---|---|---|
| load | 1496 ms | 1490 ms | at once | 9353 ms |
| idle | 1484 ms | 1475 ms | 1477 ms | 9354 ms |
| interaction (a click 1 s after load) | 1488 ms | 1479 ms | 1576 ms | 9366 ms |

With the hint the download runs from the start in every mode, so on this
page, with nothing else to load, the modes cost nothing in time to
interactive and change only when the boot's work runs. They pay off on pages
whose own images and fonts would otherwise share the bandwidth with the WASM.

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

Check the effect of any new dependency with `make size-report`; CI reports each
program's brotli size against `docs/perf/size-budget.json`.

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

