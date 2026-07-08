# Boot Latency / TTI Measurement — Plan & Implementation

**Status:** Implemented (this PR)
**Roadmap:** 3.2 (boot latency), feeds 3.1 (bundle size) and 3.4 (mutation transport)
**Origin:** `docs/26-07-02-fable-review.md` §9 — "SSR-first is the right
architecture precisely because WASM boot is slow … that makes boot the critical
path, not a polish item."

## Why this first

goowee ships an SSR-first architecture whose entire justification is hiding WASM
boot cost behind server-rendered HTML. Yet nothing measures that cost. CI has a
binary-**size** ceiling (`ci.yml`, 6 MiB guard) but no **time** measurement, so
every performance decision downstream — evaluating TinyGo (3.1, an L-effort
migration), the binary mutation transport (3.4), streaming instantiation — is a
guess about which phase actually dominates.

Binary size is not a goal; it is one *input* to time-to-interactive. This harness
measures TTI and splits it into phases so the size/transport work can be aimed at
the phase that actually costs, and so its wins can be proven rather than assumed.

## What "boot" is, phase by phase

The loader is `WebAssembly.instantiateStreaming(fetch("main.wasm"), …).then(go.run)`.
From navigation start to interactive, the phases are:

1. **Download** — fetch `main.wasm` over the wire (dominated by binary size).
2. **Compile** — `WebAssembly` compiles the module (overlaps download under
   `instantiateStreaming`).
3. **Go runtime boot** — `go.run` bootstraps the Go runtime (`wasm_exec.js`),
   schedulers, GC, then enters `main()`.
4. **First render / hydrate** — the app renders, produces its first mutation
   batch, and `applyMutations` flushes it to the DOM (claiming SSR nodes when
   hydrating). The first `applyMutations` call is the moment the page becomes
   interactive — the same signal the E2E smoke test already keys on.

TTI = navigation start → first `applyMutations`.

## Design

Measure in the browser with the standard `performance` API (marks + Resource
Timing), read the marks out over the DevTools Protocol from a Node reporter —
the exact CDP pattern `test/e2e/smoke.mjs` already uses (no new dependencies).

### Instrumentation (framework-level, in `runtime/goowee.js`)

The loader is currently copy-pasted in three places (`examples/counter/index.html`,
`scripts/pages-index.html`, and inline in `cmd/ssr-server/main.go`). Rather than
instrument three copies, factor the loader into one `goowee.boot()` in the runtime
and have all three call it. This both instruments boot and removes the
duplication (a latent divergence bug of its own).

Marks emitted (all under the `goowee:` prefix, so they never collide with app or
browser marks):

| Mark | When |
|------|------|
| `goowee:fetch-start` | immediately before `fetch("main.wasm")` |
| `goowee:instantiated` | `instantiateStreaming` resolved (download+compile done) |
| `goowee:run` | immediately before `go.run` |
| `goowee:first-listen` | first `goListen` — Go booted far enough to register events |
| `goowee:hydrate-start` / `:hydrate-end` | around the one-time SSR-claim pass |
| `goowee:interactive` | first `applyMutations` — **TTI** |

Marks are one-shot (guarded by a flag) so SPA route changes don't overwrite them.
Zero Go changes: every mark sits in JS the runtime already owns. Download **size**
comes for free from the Resource Timing entry for `main.wasm`
(`transferSize`/`encodedBodySize`), so the harness reports bytes-on-the-wire too.

`goowee.bootTimings()` returns a plain object (marks + derived phase durations +
the wasm resource entry) that the reporter reads by value over CDP.

### Reporter (`test/e2e/boot.mjs` + `test/e2e/boot.sh`)

Mirrors `smoke.mjs`: spawn headless Chromium, connect over CDP, and for each
scenario navigate, wait for `goowee:interactive`, then read `bootTimings()`.

- **Scenarios**, both against the one SSR server: `/` (server-rendered →
  **hydrate** path) and `/error` (served via the index.html fallback → **fresh
  client render**, no SSR nodes). This is a clean A/B of hydrate vs. cold render
  on identical assets.
- **Cold download** via `Network.setCacheDisabled(true)` so the download phase
  reflects a first visit, not a warm cache.
- **Repeat & aggregate**: N iterations per scenario (default 7), report
  **median** and min to smooth headless jitter.
- **Budget**: opt-in only. `GOOWEE_TTI_BUDGET_MS` gates CI later (3.5); unset, the
  harness just prints. Measurement-first: we set a budget *after* recording a
  baseline, not before.

`make boot` wraps build → serve → report, exactly like `make e2e`. CI wiring is
deliberately deferred to 3.5 (regression budgets) — the existing browser E2E
(`make e2e`) is also local-only, so this stays consistent; the follow-up is to
run both in CI once budgets exist.

## Baseline

Recorded from this environment (headless Chromium, localhost SSR server — so
download time is loopback-fast and understates real-network download; treat the
**phase split** as the signal, not absolute download ms). See the "Baseline"
section at the end, populated by `make boot`.

## Explicitly out of scope

- CI wiring / regression gating (roadmap 3.5).
- Actually reducing any phase (TinyGo 3.1, transport 3.4) — this measures so those
  can be aimed and proven.
- Real-network / throttled-download numbers (localhost only here); the harness
  accepts a `GOOWEE_BOOT_URL` so it can later point at a deployed, throttled
  target.

## Baseline results

Recorded via `make boot` (headless Chromium, localhost SSR server, 7 cold-cache
loads/scenario, medians). Download is loopback-fast here, so read the **phase
split**, not absolute download ms — on a real network the download share only
grows.

| Scenario | download+compile | go boot+render | hydrate | **TTI** | wasm on wire |
|----------|-----------------:|---------------:|--------:|--------:|-------------:|
| SSR + hydrate | 231.8 ms | 41.2 ms | 3.1 ms | **306.7 ms** | 4061 KB |
| client render | 193.9 ms | 39.2 ms | 0.6 ms | **276.8 ms** | 4061 KB |

### What the numbers say

1. **Download+compile of the binary dominates TTI (~65–75%),** even on loopback
   where the wire is free. This is the lever. Confirms binary size (3.1) is the
   right target — but see (2) before reaching for TinyGo.
2. **The wasm is served uncompressed (4061 KB on the wire).** `gzip -9` of the
   same binary is **1094 KB — a 3.7× reduction** — and brotli does better. Turning
   on compression at the server/CDN is a *zero-code-risk* ~3.7× download cut,
   cheaper and safer than the L-effort TinyGo migration. **This is the
   highest-value follow-up the measurement surfaced** (it belongs to 3.1 and is
   noted there; not done in this PR, which is measurement-only).
3. **On localhost, SSR TTI is *higher* than client render (307 vs 277 ms),** not
   lower. SSR's payoff is first-*contentful*-paint — the user sees real content
   immediately — which TTI (first interactivity) doesn't capture; hydration adds a
   little work on top. Implication: to credit SSR fairly, also measure FCP (a
   natural next mark), and don't expect SSR to move TTI — TTI moves when the
   binary shrinks.
4. **Go runtime boot + first render (~40 ms) and hydrate (~3 ms) are
   negligible.** This independently reconfirms the mutation-transport gate (3.4):
   boot time is not spent in the bridge, so binary encoding won't help boot.

### Recommended next actions, in value order

1. **Enable gzip/brotli for `.wasm`** (3.1) — ~3.7× download cut, near-zero risk.
   ✅ **Done.** The reference SSR server (`cmd/ssr-server`) now gzips the wasm
   (4181 KB → 1138 KB, cached and compressed once), the JS/CSS, and the SSR HTML;
   GitHub Pages already gzips via its CDN. Serving is documented in
   `docs/serving.md`. Confirmed with the harness under throttling (below).
2. **Add an FCP mark** so SSR's actual benefit is visible next to TTI.
3. Only then evaluate **TinyGo** (3.1) against the *post-compression* download —
   the incremental win may be much smaller once the binary is already ~1 MB.
4. Wire `make boot` into CI with a `GOOWEE_TTI_BUDGET_MS` gate (3.5) once a
   real-network baseline is chosen.

### Confirming the compression win (throttled)

Loopback has effectively infinite bandwidth, so it hides the download win — under
`THROTTLE=off` the compressed and uncompressed TTI are indistinguishable. Run the
harness under an emulated network to see it:

```sh
THROTTLE=4g make boot     # also: fast3g, slow3g
```

`boot.mjs` now applies the profile via CDP `Network.emulateNetworkConditions` and
reports the wasm bytes-on-the-wire (which drop ~3.7× with gzip). A deterministic
cross-check at a fixed 500 KB/s (curl `--limit-rate`) shows the download phase
directly: **8.0 s uncompressed → 2.1 s gzip** for `main.wasm`. Since download
dominates TTI, that is the bulk of the boot-time win on any real connection.
