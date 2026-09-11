# Changelog

All notable changes to goowee are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

goowee is **v0 (experimental)** — see [docs/api-stability.md](docs/api-stability.md).
Under `v0.x`, breaking changes to the public API are allowed between minor
releases but are called out here; anything not listed as public may change at any
time. The public surface freezes under semver at v1.

## [Unreleased]

### Added
- `ref.Get(prop, fn)` — read a value back from a ref'd node (`offsetWidth`,
  `scrollTop`, `selectionStart`, `getBoundingClientRect`, …). The read is
  answered after the next frame's DOM updates apply and `fn` runs on the render
  loop, so it may set signals. Backed by a new `MutRead` mutation whose reply
  piggybacks on `applyMutations`' return value (ADR-019).
- `e.Files()` on `change`/`input` events from `<input type="file">`: each
  `core.File` carries `Name`, `Size`, `Type`, `LastModified`, and
  `Bytes() ([]byte, error)` reads the contents on demand — from a goroutine,
  applied with `core.Schedule`, like a fetch. Closes #51 (no more
  `getElementById` to reach `input.files`).
- `core.SetFileReader` hook (installed by the bridge) and `core.ErrNoFileReader`
  for SSR/tests; `core.LogRecoverRead` for a panicking read callback.

### Changed
- `bridge.Run` activates the scheduler before the first render, so
  `core.Schedule` called during initial setup (e.g. from `OnMount`, to defer a
  `ref.Get` until the element exists) runs on the first flush instead of being
  dropped.
- `runtime/goowee.js`: `applyMutations` now returns a JSON string of read
  replies when the batch carried reads (otherwise `undefined`); file inputs
  park their `File` objects by handle (released when the selection changes or
  the node is removed) and expose `goowee.readFile(handle)`.

## [0.1.0] — 2026-07-23

First tagged release. goowee is a reactive UI framework in pure Go that compiles
to a single WebAssembly binary: signals drive fine-grained DOM updates, with no
virtual DOM and no JavaScript build step. This entry summarizes the feature
surface at the first release rather than a diff.

### Reactive core
- Generic `Signal[T]` with `Get`/`Set`/`Update`, token-based subscriptions,
  reentrancy-safe notification, and an equality skip (same value → no notify).
  Custom equality via `WithEquals`; comparability decided by reflection so
  slice/map-valued signals are safe.
- `Computed` — derived read-only signals with explicit dependencies.
- `core.Schedule(fn)` — apply off-loop updates (timers, network, goroutines) on
  the render loop safely (ADR-015).
- Dirty-scope scheduling: N signal writes in a frame coalesce into one re-render
  and one mutation batch, flushed on-demand via `requestAnimationFrame`.

### Components & hooks
- Solid-style **run-once** components: the component body is setup, runs once on
  mount; state lives in signals captured by closures (ADR-001).
- `hooks`: `UseState`, `UseEffect`, `Watch`, `OnMount` (mount/cleanup lifecycle),
  `UseComputed`, `UseResource` (async data with `Data`/`Loading`/`Err` +
  `Refetch`, goroutine fetch applied via `core.Schedule` with a generation
  guard), `UseScope`.

### Rendering & DOM
- Pure-Go typed DSL (`h`): elements, attributes, properties, typed events,
  two-way binds (`BindValue`/`BindChecked`/`BindSelect`/`BindValueLazy`),
  control flow (`Show`/`ShowElse`/`Switch`/`For`), `Map`/`Group`/`If`,
  `VirtualList`, `Raw` (pre-rendered HTML), `ShowResource`.
- Keyed reconciliation with a longest-increasing-subsequence pass, so a list
  reorder moves only the nodes outside the LIS.
- `h.Svg` namespaced elements (`createElementNS`) + shape helpers.
- `h.Ref`/`h.RefTo` imperative command handles (`Focus`/`Blur`/`Click`/
  `ScrollIntoView`); `h.Portal` (client-side, rendered fresh under hydration).
- `h.ErrorBoundary` catches render-time panics and shows a fallback; update-time
  panics are contained (the subtree keeps its previous state) rather than
  blanking the page.
- Forms & inputs: caret preservation on programmatic `value` writes, form/focus/
  clipboard events, `SelectOnFocus`.
- `Aria*` attribute helpers + `AutoFocus`.

### SSR & hydration
- `ssr.New().Render(node)` returns `(body, head)`; request-safe (render context,
  no effects run on the server), with escaped attributes and self-closing void
  elements.
- Claim-based hydration: the client adopts the server-rendered DOM by node id
  instead of rebuilding it. SSR/DOM id parity holds **by construction** via a
  shared `internal/walker`, guarded by a golden test. Text nodes use
  `<!--g{id}-->` markers.
- `h.Dynamic()` escape hatch for non-deterministic subtrees (client value wins);
  structural mismatches log a clear error and best-effort recover.
- `h.Metadata` + `Page(PageMeta{…})` for `<head>` metadata (title/OG/Twitter/
  JSON-LD), injected into the document head with no duplication on hydration.

### Router
- Deterministic, most-specific-first matching; URL params with reactive
  `ParamSignal`; query params; nested `SubRoute`; `Guard`; `Lazy`;
  `Navigate`/`NavigateReplace`/`Back`/`Forward`; built-in History API integration
  behind a `js/wasm` build tag; `<base href>` base-path support (GitHub Pages).

### DX, devtools & observability
- Structured `core.Log` recover/observability events forwarded to the browser
  console; dev-mode inspector (`?goowee-dev`) exposing `goowee.inspect()` for the
  component/scope tree and signal graph.
- High-frequency events (scroll/pointermove) coalesced to one dispatch per frame.

### Build, serving & tooling
- Public/internal API boundary: the DOM renderer lives in `internal/`, framework
  internals in `internal/runtime`; a single client entry point `bridge.Run(node)`.
  Public packages: `h`, `hooks`, `router`, `ssr`, `bridge`, `devtools`, and a
  named subset of `core` (see docs/api-stability.md).
- Reference SSR server gzips wasm/JS/CSS/HTML; `make pages` builds a client-only
  static site (with `404.html` SPA fallback) for GitHub Pages.
- CI gates: `go vet`, `go test -race`, native + `js/wasm` builds, a WASM size
  budget, benchmark smoke, **allocation budgets** (O(1) signal notify, bounded
  coalesce/diff), and a **headless-Chrome E2E** (hydration, interactivity,
  routing/history, refs, async, error boundary, off-loop stopwatch, devtools).
- Boot-latency harness (`make boot`) reporting a median TTI phase split plus an
  **FCP** mark that credits SSR's early first paint; network-throttling profiles.

### Docs
- User-facing guides (getting-started, concepts, metadata, serving), an API
  reference, an API-stability policy, 18 ADRs of settled decisions, and an
  agent guide (AGENTS.md). A live tutorial site (the example app is itself built
  with goowee).

### Known limitations
- Standard Go (`GOOS=js GOARCH=wasm`) is the only supported build target. TinyGo
  was evaluated and **not adopted**: TinyGo wasm does not implement `recover()`
  ([tinygo#2914](https://github.com/tinygo-org/tinygo/issues/2914)), which breaks
  error boundaries and panic containment. See
  [docs/plans/tinygo-recover-implementation.md](docs/plans/tinygo-recover-implementation.md).
- Update-time panics are contained but do not switch to the error-boundary
  fallback UI (ADR-018). Ref *reads* (measurement) are not yet supported
  (ADR-017). A re-rendering scope rooted at an SVG child does not inherit the SVG
  namespace.

[0.1.0]: https://github.com/yogisalomo/goowee/releases/tag/v0.1.0
