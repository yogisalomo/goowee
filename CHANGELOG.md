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
- `ssr.Handler(ssr.HandlerOptions{Page, Document, Fallback, ClientOnly})` —
  serve server-rendered documents: concurrent renders, the page's status (404
  when the router falls through), the client-rendered fallback when a render
  panics or a path is client-only, gzip. `ssr.RoutedPage(app)`,
  `router.NewURL`, `Router.NotFound`. The reference server uses it and no
  longer hard-codes its SSR routes (param routes like `/greet/:name` are now
  server-rendered, unknown paths get 404). (#77)
- `e.PreventDefault()` / `e.StopPropagation()` on `core.EventData`, decided per
  event while the handler runs; typed accessors `Code`, `Repeat`,
  `IsComposing`, `InputType`, `CtrlKey`/`ShiftKey`/`AltKey`/`MetaKey`,
  `OffsetX/Y`, `Button(s)`, `PointerID`, `PointerType`, `Pressure`,
  `DeltaX/Y/Z`, `DeltaMode`, `Touches`, `ChangedTouches`; `core.NewEventData`.
  Helpers `OnMouseEnter`, `OnMouseLeave`, `OnPointerDown/Move/Up`, `OnWheel`,
  `OnContextMenu`, `OnLoad`, `OnError`. `router.Link` takes extra items;
  `router.InAppClick`. A tutorial "Events" lesson. (#67, #68)
- `core.ComponentWithProps(name, props, func(*core.Signal[P]) core.Node)` — a
  component whose preserved instance receives new props through a signal and
  keeps its state; `ComponentNode.HasProps` / `Adopt` for renderers. (#57)
- `bridge.Element(ref) js.Value` (js/wasm): the live DOM element behind a ref,
  for handing to a JavaScript library (maps, charts, editors, `<canvas>`) from
  `OnMount` (ADR-022). The dashboard example draws a canvas chart with it.
  (#53)
- `Scheduler.QueueEffect` / `RunEffects` / `PendingEffects`, and
  `core.LogRecoverEffect` for a panicking effect.
- `core.Batch(fn)` — group signal writes so each written signal notifies once,
  after `fn` returns; derived state never observes a half-applied update. Event
  handlers, `core.Schedule` callbacks, `ref.Get` replies (and effects) are
  batched automatically (ADR-020). (#73)
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

- `h.SrcS(sig)` — bound `src`, alongside `HrefS`/`ClassS`/…. (#55)

### Changed
- Removed the unused `ssr.HydrationMeta`, `ssr.SlotRef`, `Renderer.Meta` and
  `Renderer.RenderWithMeta` (leftovers of the pre-ADR-008 design). (#79)
- `focus`/`blur`/`scroll` handlers fire for the target element only (a
  container's `OnFocus` no longer fires for descendants; use `OnFocusIn`).
- Removing a subtree sends one `RemoveNode` per DOM root (plus portal/head
  content inside it) instead of one per descendant; the JS runtime forgets the
  removed subtree's nodes (and parked file handles) itself. Clearing 100
  five-node rows: 500 mutations → 100. `goowee.nodeCount()` exposes the tracked
  node count (the E2E test checks it for leaks). (#71)
- `h.For` renders only new and changed items (unchanged rows are reused as-is),
  the keyed diff's LIS is O(n log n) with an O(n) fast path when nothing moved,
  and `core.FlatTree` is shallow (children are flattened as the renderer
  reaches them). Appending one row to a 5,000-row list: 14 ms → 1.3 ms. (#70)
- Reactive regions get an end anchor: SSR emits `<!--/{id}-->` after each
  region's content and the client keeps an empty comment there; node ids after
  a region shift by one (ADR-021).
- `core.Computed` is lazy: it subscribes to its deps only while observed and
  recomputes on `Get` when unobserved (reads are always current). It no longer
  registers a disposer on the current component.
- A panic while a re-render diffs through an `h.ErrorBoundary` now renders the
  fallback (it used to keep a half-updated subtree); a later successful render
  restores the child (amends ADR-018).
- `runtime/goowee.js`: an `InsertBefore` goes to its reference node's actual
  parent.
- Signal notification no longer allocates or looks subscribers up by id:
  `Set` is O(N) in subscribers (was O(N²)) and unsubscribe is O(1) — 1,000
  subscribers: ~505 µs → ~1 µs per `Set`. A panicking `core.Schedule` callback
  is now contained and logged (`recover.scheduled`) instead of aborting the
  frame. (#69)
- Bound text (`TextS`, `Textf`, `BindProp("textContent", …)`) is formatted in Go
  with `%v` and sent as a string, the same format SSR uses — floats no longer
  change format after hydration (`1e+08` vs JS's `100000000`). (#64)
- `bridge.Run` activates the scheduler before the first render, so
  `core.Schedule` called during initial setup (e.g. from `OnMount`, to defer a
  `ref.Get` until the element exists) runs on the first flush instead of being
  dropped.
- `runtime/goowee.js`: `applyMutations` now returns a JSON string of read
  replies when the batch carried reads (otherwise `undefined`); file inputs
  park their `File` objects by handle (released when the selection changes or
  the node is removed) and expose `goowee.readFile(handle)`.

### Fixed
- Router query params decode `%XX` escapes (`?q=go%20wasm` → `go wasm`), via
  `net/url`.
- The reference server's static-file path is cleaned (no `..` escapes).
- SSR renders run concurrently: `ssr.Renderer.Render` no longer holds a
  process-wide lock (server renders track no component frames; hooks see
  `EnvServer` via `runtime.ServerRender`) (ADR-024). (#72)
- `javascript:`/`vbscript:` URLs in URL attributes (`href`, `src`, `action`,
  `xlink:href`, …) are replaced with `about:blank#blocked` on the server and
  the client; attribute names are validated identically in body, head and DOM
  (and `xlink:href`/`xml:lang`/`data-a_b` are now allowed). (#78)
- A bound or static `textContent` property is rendered as the element's
  content by SSR.
- Event handlers bubble through ancestors, innermost first, until one stops
  propagation — they used to stop at the first handler, so a parent never saw
  a child's click and `StopPropagation()` did nothing (ADR-023). (#65)
- Non-bubbling events (`invalid`, `mouseenter`/`mouseleave`, `load`/`error`,
  media events, …) are listened for in the capture phase and reach their
  target's handler — `h.OnInvalid` never fired before. (#66)
- `router.Link` leaves cmd/ctrl/shift/alt and middle clicks to the browser
  (open in new tab/window) instead of navigating in-app. (#67)
- Coalesced `scroll`/`pointermove` keep the latest event per element, so two
  containers scrolling in the same frame both update. (#68)
- A plain component kept across a parent re-render no longer silently shows
  stale data in `h.For`: when an item changes, element rows are diffed in
  place, `ComponentWithProps` rows get the new item, and plain component rows
  are remounted. Component identity is now name + key, so keying a component
  by a value remounts it when the value changes; dev mode warns once when an
  unkeyed plain component is kept across a re-render. (#57)
- `OnMount` (and `UseEffect`'s first run) runs after the component's DOM is in
  the document — refs are set, so `ref.Focus()`/`ref.Get()` work directly and
  no `core.Schedule` is needed to wait for an element; an effect whose
  component unmounts before it runs never runs (ADR-022). (#60)
- Content a reactive region renders as several roots (a `For` list, a
  multi-root `Show` branch) is placed before the region's following siblings
  — appended rows used to land after a trailing footer — and a region
  replaced by a plain element is rendered (it used to render nothing). (#83)
- A nested `Show`/`For`/`Switch` inside a subtree that re-renders is adopted
  instead of torn down and rebuilt: components inside keep their state, and
  toggled content stays under its real parent. (#59)
- `h.Textf`/`core.Computed` created inside a re-rendering region no longer
  leak a subscription per re-render, and `Watch`/`UseEffect`/`OnMount` called
  in a region's render function are disposed with that render. (#58)
- `h.ErrorBoundary` releases the effects, bindings, handlers and scope
  subscriptions of whatever its child mounted before panicking. (#61)
- Hydrating an SSR'd `h.Raw` node claims its `<div>` wrapper instead of logging
  a mismatch and dropping the node.
- A prop whose value is a slice, map, or func no longer panics the differ
  ("comparing uncomparable type") and freezes the subtree; it is treated as
  changed and re-set. An uncomparable key falls back to positional matching
  with a warning instead of panicking, and signal equality now also catches a
  struct whose interface field holds an uncomparable value. (#63)
- One mutation JSON can't encode (a `NaN`/`±Inf` property value) no longer
  silently drops every DOM update of the frame: the bridge falls back to
  per-mutation encoding, applies the rest, and logs what it dropped. (#64)
- `router.SubRoute` matching is deterministic: it now compiles and orders its
  patterns most-specific-first like `Route`, instead of ranging over the
  routes map (overlapping patterns used to match at random). A sub-route's
  params are merged into the parent route's params rather than replacing them,
  and keys it no longer matches are dropped. (#62)

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
