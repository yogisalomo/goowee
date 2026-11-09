# Changelog

All notable changes to goowee are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

goowee is **v0 (experimental)** — see [docs/api-stability.md](docs/api-stability.md).
Under `v0.x`, breaking changes to the public API are allowed between minor
releases but are called out here; anything not listed as public may change at any
time. The public surface freezes under semver at v1.

## [Unreleased]

## [0.2.7] — 2026-11-10

A patch release with two fixes found building Dure: `Navigate` with a query
string, and the document title. The second turned out wider than reported: a
`Metadata` title never became the tab's title when the page already had a
`<title>`, which is now fixed, and the title can follow a signal. Upgrade from
0.2.6 without code changes; if your app set a `Metadata` title, the tab now
shows it instead of the HTML shell's.

### Added
- `h.TitleS(sig)`: a document title that follows a signal, e.g. an unread
  count in the tab title; the server renders its current value (#121).
- gooweetest: `s.Title()`, the title the browser would show.

### Fixed
- `r.Navigate` and `r.NavigateReplace` (and so `Link`/`LinkTo`) kept a target's
  query in `Path`: `/r/2?t=20` matched `:id` as `"2?t=20"` and left `Query`
  at the previous URL's values. They now split off the query and fragment,
  set `Path` and `Query` together, and push the whole URL; a target without a
  query clears it, `"?t=3"` keeps the current path, and `"#x"` keeps the path
  and query (#120).
- The app's `Metadata` title never became the document's title when the page
  already had a `<title>` (the HTML shell's, or the server-rendered one),
  because `document.title` reads the first `<title>` and goowee's was
  appended after it. The runtime now removes the page's own title when goowee
  renders one, and hydration renders the title for real instead of skipping
  it, so `TitleS` updates reach the tab. With several titles mounted, the
  first in the tree wins, as in the server-rendered head (#121).

## [0.2.6] — 2026-11-07

A patch release that makes browser code testable and cheaper: picked and
pasted files upload without passing through WebAssembly memory, server-sent
events and browser storage get goowee APIs with test doubles, and a testing
guide covers the pattern for everything else. Upgrade from 0.2.5 without code
changes.

### Added
- **Zero-copy file uploads** (#99): `net.Request{Form: new(net.Form).Add(…).AddFile(name, f)}`
  sends a multipart body in which a picked, pasted or dropped file goes as the
  browser's own `File`, so its contents never pass through WebAssembly memory
  (a 16 MB upload allocates under 1 MB in Go; `File.Bytes` then a `Blob`
  allocated all 16). `bridge.FileValue(f)` hands the `File` to a JavaScript
  library, and `core.File.Handle` is the key both use.
- **`net.Subscribe`: server-sent events** (#100) through the browser's
  `EventSource`, with named event types, callbacks on the render loop, and
  the browser's own reconnection (`OnError(reconnecting)`). It needs a
  transport implementing the new optional `net.StreamTransport`; the browser
  and `FakeNet` do. In tests: `s.WaitForStream(n, path)` → `Send`,
  `SendEvent`, `Fail`, `Reopen`.
- **`goowee/storage`** (#100): `storage.Local` and `storage.Session` on the
  browser's `localStorage`/`sessionStorage`, an in-memory store outside the
  browser (gooweetest clears it when a test ends), empty during server
  renders so requests can't see each other's values, and `ErrUnavailable`
  where the browser refuses storage.
- [docs/guides/testing.md](docs/guides/testing.md): gooweetest, a table of the
  browser APIs goowee covers and their test doubles, and the `_js.go` seam
  for the rest (#100).
- `test/e2e/storage.mjs` (part of `make e2e`), and the net e2e now covers a
  16 MB Form upload and an event stream.

## [0.2.5] — 2026-11-07

A patch release with a tree-scoped context API, a `goowee.boot()` that starts
on hosts that don't send `application/wasm`, a default `go test ./...` that
no longer fails on busy machines, and two `FakeNet` improvements found moving
Dure's tests onto it. Upgrade from 0.2.4 without code changes; one gooweetest
behavior note (see Changed): overlapping `FakeNet` patterns now resolve to the
most specific one.

### Added
- **Tree-scoped context** (#111, ADR-027): `core.NewContext[T](default)`,
  `ctx.Provide(signal, children...)` and `ctx.Use(func(*core.Signal[T]) core.Node)`.
  The nearest provider wins, consumers update by binding the signal, content
  mounted later (a `Show`, a new `For` row) still finds its provider, and
  removing a provider's subtree releases its consumers' subscriptions. Each
  render resolves its own providers, so concurrent SSR requests never see each
  other's values.
- gooweetest: `FakeNet.Handle` takes `net/http.ServeMux` patterns, with
  prefixes (`"POST /api/"`), wildcards (`"/rooms/{id}"`) and `"/"` as a
  catch-all, plus `gooweetest.Status(code)`, so tests that only check what the
  app sent don't stub every endpoint (#114). `s.WaitForSocket(n, path)` waits
  for an app that dials after an async step, or for the socket after a
  reconnection, and `n.SetReconnectDelay(d)` skips the real backoff (#115),
  through the new `net.ReconnectDelayer`.
- `make ssr-scaling`: the parallel-SSR speedup check, now opt-in (see Fixed).
- `test/e2e/wasmmime.mjs` (part of `make e2e`): boot under each server
  behaviour for `main.wasm`.

### Changed
- gooweetest: when several `FakeNet` patterns match, the most specific wins
  (ServeMux's rule) instead of the latest registered. Registering the same
  pattern again still replaces its handler.

### Fixed
- `goowee.boot()` failed to start the app when `main.wasm` wasn't served as
  `application/wasm` (#109). It now compiles the module after downloading it,
  with a console warning naming the header to fix; `application/wasm` keeps
  the streaming fast path. Real failures (network, HTTP status, an HTML page
  answering the wasm URL) report one actionable console error, a
  `goowee:boot-error` event and `goowee.bootError`, not an unhandled promise
  rejection; the returned promise still rejects for callers that chain on it.
- `go test ./...` failed on loaded or small machines: `TestConcurrentSSRScales`
  required a 1.5× wall-clock speedup (#110). It's opt-in now
  (`GOOWEE_PERF_TESTS=1`, `make ssr-scaling`), and the default suite checks
  the no-global-lock property deterministically: two renders must be inside
  setup at the same time.
- The e2e harnesses left a Chrome profile (up to ~200 MB) in the temp
  directory on every run; they now delete it.

## [0.2.4] — 2026-11-06

A patch release for the composer of a real goowee app (Dure, #106): pasted
screenshots and dropped files reach Go through `e.Files()` like a file
input's, with their text and types; `ref.Call` calls DOM methods with
arguments, such as placing the caret after inserted text; and gooweetest can
paste, drop and pick files. Also fixes `OnPaste`/`OnCut`/`OnCopy`, which
passed `""` in the browser. Upgrade from 0.2.3 without code changes.

### Added
- **Paste and drop carry their data** (#106). A paste's clipboard files (a
  pasted screenshot) and a drop's files come through `e.Files()`, readable
  with `File.Bytes()` like a file input's (each element's latest paste or
  drop stays readable until the next one or until it's removed).
  `e.DataText()` is the pasted or dropped text (for cut and copy, the
  selection), and `e.DataTypes()` lists the formats, also during dragover,
  when the files themselves aren't exposed yet. New helpers: `h.OnPasteE`,
  `h.OnDragEnter`, `h.OnDragOver`, `h.OnDragLeave`, `h.OnDrop`.
- `ref.Call(method, args...)` calls a DOM method with arguments, after the
  frame's DOM updates, so it sees a value the same handler just set: e.g.
  `ref.Call("setSelectionRange", at, at)` to put the caret after inserted text.
  A throwing call is logged without stopping the frame (#106).
- gooweetest: `s.Paste`, `s.Drop`, `s.SelectFiles` with `gooweetest.File`
  contents behind `File.Bytes`, `gooweetest.DragOverData()`, and
  `el.Calls()`. `selectionStart`/`selectionEnd` follow `Input`,
  `setSelectionRange` and value writes.
- `test/e2e/clipboard.mjs` (part of `make e2e`): paste, drop and `ref.Call`
  in headless Chrome.

### Fixed
- `h.OnPaste`, `h.OnCut` and `h.OnCopy` always passed `""` in the browser:
  their payload had no value. They now pass the pasted text and the selected
  text; a test dispatching only `"value"` still works.
- gooweetest answered `ref.Get` reads where they occurred in the batch; it now
  answers them after the batch's writes, as the browser does.

## [0.2.3] — 2026-11-06

A patch release with what building a real app on goowee (Dure: a workspace
UI with forms, threads and live updates) turned up: networking without
`net/http` (`goowee/net`, plus a fake for `gooweetest`), a loading
placeholder that the app replaces when it mounts, and three API
ergonomics fixes. Upgrade from 0.2.2 without code changes. Two behavior notes
(see Changed): a client-only mount now replaces whatever `#root` holds, and a
hidden page keeps running frames on a timer.

### Added
- **`goowee/net`: fetch and WebSocket without `net/http`** (#103, ADR-026).
  `net.Fetch(ctx, net.Request{…})` wraps the browser's `fetch`: method,
  headers, a JSON or byte body, credentials mode, aborted when `ctx` ends, so
  it fits `hooks.UseResource`. `Response.JSON` decodes the body and returns
  a `*net.StatusError` for a non-2xx status. `net.Dial(url, net.SocketOptions{…})`
  opens a WebSocket whose `OnOpen`/`OnMessage`/`OnClose` callbacks run on the
  render loop (no `core.Schedule`), with optional reconnection with jittered
  backoff and a `ReconnectURL` for replay cursors. About 80 KB raw (12 KB
  brotli) in an app that uses both, where `net/http` costs ~7 MB. Outside the
  browser `Fetch` returns `net.ErrUnavailable`; `net.SetTransport` swaps the
  network.
- `gooweetest.NewFakeNet(t)`: an in-memory `goowee/net` for component tests:
  route handlers (`n.Handle("GET /api/x", …)`, `gooweetest.JSONResponse`),
  recorded requests, and the server end of each socket the app dials
  (`n.Socket("/ws")` → `Send`, `Received`, `Close`).
- `h.RawS(sig)`: raw HTML that follows a `*core.Signal[string]`, replacing the
  wrapper div's content in place when it changes (#105).
- `h.Empty()`: the node that renders nothing, for functions that return
  `core.Node` (`h.Group` is an item, not a node) (#105).
- `test/e2e/net.mjs` (part of `make e2e`): `goowee/net` in headless Chrome
  against a real HTTP and WebSocket server.

### Changed
- `core.SetKey` returns the node it keys, so a keyed row is built inline:
  `return core.SetKey(Card(m), m.ID)` (#105). Existing calls compile
  unchanged.
- On a client-only mount (no server-rendered nodes), the app replaces
  whatever `#root` holds, so a "Loading…" placeholder shown while the WASM
  downloads goes away with the first frame instead of staying above the app
  (#104). Keep portal targets outside `#root`. The example and Pages shells
  now show one.
- While the page is hidden, the frame loop runs on a timer, since browsers
  don't fire `requestAnimationFrame` in background tabs: socket messages,
  resource results and other scheduled updates apply then (at the browser's
  throttled timer rate) instead of piling up until the tab is shown.

### Fixed
- The API reference listed `Group` and `Nodes` with the wrong signatures,
  `Dynamic` with children, and no `Fragment`; the getting-started shell now
  uses `goowee.boot()` (#105).

## [0.2.2] — 2026-10-19

A patch release that closes the smaller deferred roadmap items: SVG content
created by re-renders renders again, focus survives keyed reorders, portals
keep their state, mutation encoding is 3.3× faster in WASM, the reference
server serves brotli-compressed wasm, and performance is tracked release over
release. No public API changes — upgrade from 0.2.1 without code changes. One
behavior note: a plain component inside a portal is now preserved across
re-renders, as everywhere else (see Changed).

### Changed
- **Portals are reconciled on re-render** instead of torn down and rebuilt: a
  component, form or focused input inside `h.Portal` keeps its state when the
  component holding the portal re-renders, keyed children reorder in place,
  and portals sharing a target keep their content apart. Each portal keeps an
  empty comment (its end anchor) after its content in the target; a changed
  target still rebuilds (ADR-017, amended). As everywhere else (ADR-001), a
  plain component inside a portal now keeps the plain values it was first
  called with — pass signals or use `core.ComponentWithProps`.
- Mutation batches are encoded by a hand-written JSON encoder instead of
  `json.Marshal`, with byte-identical output (fuzzed against
  `encoding/json`): 3.3× faster in WASM on a 5,000-update frame
  (6.1 ms → 1.9 ms).
- `cmd/ssr-server` serves a pre-built `main.wasm.br` (`make brotli`) to
  browsers that accept brotli — 27% smaller than gzip — ignoring a copy older
  than `main.wasm`, and honours `q=0` in `Accept-Encoding`.

### Fixed
- SVG elements created by a re-render were created in the HTML namespace and
  didn't render: rows an `h.For` adds inside an `<svg>`, content of a scope
  rooted at an SVG child, children added to a reused `<svg>`. Re-rendered
  content now inherits the namespace of where its scope is mounted, and
  `<foreignObject>` children are HTML.
- A keyed reorder that moved the row holding the focused input dropped focus
  and the text selection. The runtime moves live nodes with `moveBefore` where
  the browser has it, and otherwise restores focus and selection after the
  batch (an explicit `ref.Focus`/`Blur` in the batch still wins).

### Added
- `make perf-record VERSION=…` / `make perf-compare` (`scripts/perf.sh`): the
  benchmark suite plus the example wasm's raw/gzip/brotli size, recorded per
  release in `docs/perf/` and compared with benchstat (v0.2.1 and v0.2.2
  recorded).
- Mutation-transport measurement: `BenchmarkLayoutTickFrame`,
  `BenchmarkEncodeLayoutTick`, and `test/e2e/transport.mjs` for the browser
  side (roadmap 3.4).

## [0.2.1] — 2026-09-28

A patch release: two additive router helpers, and the tutorial site brought
in line with 0.2. No breaking changes — upgrade from 0.2.0 without code
changes.

### Added
- `r.LinkTo(to, items...)` — `Link` with arbitrary content (an icon plus
  text, several spans); `r.SetBasePath(base)` for servers rendering under a
  URL prefix and for tests (the browser still reads `<base href>` in
  `BindHistory`).

### Fixed
- The tutorial site's hand-rolled links (header, step navigation, back links,
  tutorial index, landing CTA) ignored the `/goowee/` base path and hijacked
  cmd/ctrl-click, so opening one in a new tab hit GitHub's 404; they now use
  `r.Link`/`r.LinkTo`. The site's AI-agent rules, error-boundary lesson and
  Getting Started page are updated for 0.2 (and Getting Started now shows the
  `index.html` shell).
- E2E harness: wait up to 20 s for Chrome's debug port (current Chrome takes
  ~8 s headless on macOS; the 5 s wait caused the "could not reach Chrome
  DevTools endpoint" cold-start failures).
- The Pages workflow redeploys on `internal/**` changes (the filter still
  named the pre-0.1 `dom/` path).

## [0.2.0] — 2026-09-26

The second-review release. A fresh review of v0.1.0 found silent-staleness,
lifecycle, placement and event-model bugs under the hardened surface; each was
reproduced with a test and fixed (issues #53–#83). Highlights: reactive regions
place and keep their content correctly (end anchors, adoption), `OnMount` runs
after the DOM exists, components can receive new props, event handlers bubble,
signal writes are batched, SSR renders concurrently behind a ready-made
`ssr.Handler`, and app authors get a headless test harness (`gooweetest`).
Design decisions are recorded as ADR-020 – ADR-025.

### Upgrading from 0.1

- **`hooks.UseResource`'s fetch takes a context:** `func(ctx context.Context)
  (T, error)`. Add the parameter and pass it to your request; it is cancelled
  when a newer load starts or the component unmounts. (#75)
- **Handlers bubble.** A click now reaches every ancestor's handler, innermost
  first. Where a parent handler used to be shadowed by a child's, call
  `e.StopPropagation()` in the child (or use the `StopPropagation()` option).
  `focus`/`blur`/`scroll` handlers fire for the target only — use
  `OnFocusIn`/`OnFocusOut` to hear descendants. (#65, #66)
- **`OnMount` / `UseEffect` run after the DOM is applied**, one step later than
  before. Remove any `core.Schedule` you used to wait for an element; refs and
  `bridge.Element` work directly in `OnMount`. (#60)
- **`core.Computed` is lazy:** while nothing observes it, it doesn't recompute
  on dep changes (reads are still always current). Don't put side effects in a
  compute function — use `hooks.Watch`.
- **Component identity is name + key**, and `h.For` remounts a plain
  `core.Component` row whose item changed. To keep row state across edits,
  build rows with `core.ComponentWithProps`. (#57)
- **SSR output** gains a `<!--/{id}-->` anchor after each `Show`/`For`/
  `Switch`/`Route`/`UseScope` region, and node ids after a region shift by one.
  Only matters if you parse the HTML.
- **`router.Link`** leaves cmd/ctrl/shift/alt and middle clicks to the browser.
- **`javascript:`/`vbscript:` URLs** in `href`/`src`/`action`/… are replaced
  with `about:blank#blocked`; handle such actions with `OnClick` instead.
- **Removed:** `ssr.HydrationMeta`, `ssr.SlotRef`, `Renderer.Meta`,
  `Renderer.RenderWithMeta`.

### Added
- `ssr.Handler(ssr.HandlerOptions{Page, Document, Fallback, ClientOnly})` —
  serve server-rendered documents: concurrent renders, the page's status (404
  when the router falls through), the client-rendered fallback when a render
  panics or a path is client-only, gzip. `ssr.RoutedPage(app)`,
  `router.NewURL`, `Router.NotFound`. The reference server uses it and no
  longer hard-codes its SSR routes (param routes like `/greet/:name` are now
  server-rendered, unknown paths get 404). (#77)
- `gooweetest` — headless component tests for app authors: `Render(t, node)`
  mounts into an in-memory DOM that applies the real mutation stream, with
  CSS-subset finders, `Click`/`Input`/`Submit`/`KeyDown`/`Dispatch` (DOM
  bubbling, prevent-default reporting), effects after each frame, `WaitFor`
  for async resources, portals/head/focus inspection. Also
  `Scheduler.HasWork` and `DOMRenderer.Unmount`. (#76)
- `core.Batch(fn)` — group signal writes so each written signal notifies once,
  after `fn` returns; derived state never observes a half-applied update. Event
  handlers, `core.Schedule` callbacks, `ref.Get` replies (and effects) are
  batched automatically (ADR-020). (#73)
- `core.ComponentWithProps(name, props, func(*core.Signal[P]) core.Node)` — a
  component whose preserved instance receives new props through a signal and
  keeps its state; `ComponentNode.HasProps` / `Adopt` for renderers. (#57)
- `bridge.Element(ref) js.Value` (js/wasm): the live DOM element behind a ref,
  for handing to a JavaScript library (maps, charts, editors, `<canvas>`) from
  `OnMount` (ADR-022). The dashboard example draws a canvas chart with it.
  (#53)
- `e.PreventDefault()` / `e.StopPropagation()` on `core.EventData`, decided per
  event while the handler runs; typed accessors `Code`, `Repeat`,
  `IsComposing`, `InputType`, `CtrlKey`/`ShiftKey`/`AltKey`/`MetaKey`,
  `OffsetX/Y`, `Button(s)`, `PointerID`, `PointerType`, `Pressure`,
  `DeltaX/Y/Z`, `DeltaMode`, `Touches`, `ChangedTouches`; `core.NewEventData`.
  Helpers `OnMouseEnter`, `OnMouseLeave`, `OnPointerDown/Move/Up`, `OnWheel`,
  `OnContextMenu`, `OnLoad`, `OnError`. `router.Link` takes extra items;
  `router.InAppClick`. A tutorial "Events" lesson. (#67, #68)
- Dev-mode dependency checking (`?goowee-dev` → `core.SetDevChecks`): a
  signal read while a `Show`/`For`/`Switch`/`Route`/`UseScope` renders or a
  `Computed` computes, but not declared as a dep, is reported once per read
  site with its `file:line`. `Signal.Peek()` reads without being counted;
  `core.RenderScope` for renderers. The example app is audited clean by a
  test. (#74)
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
- `Scheduler.QueueEffect` / `RunEffects` / `PendingEffects`, and
  `core.LogRecoverEffect` for a panicking effect.
- `core.SetFileReader` hook (installed by the bridge) and `core.ErrNoFileReader`
  for SSR/tests; `core.LogRecoverRead` for a panicking read callback.
- `h.SrcS(sig)` — bound `src`, alongside `HrefS`/`ClassS`/…. (#55)

### Changed
- **Breaking:** `hooks.UseResource`'s fetch takes a `context.Context`
  (`func(ctx context.Context) (T, error)`), cancelled when a newer load starts
  (dep change, `Refetch`) or the component unmounts. Migrate by adding the
  parameter and passing it to your request. (#75)
- `core.Computed` is lazy: it subscribes to its deps only while observed and
  recomputes on `Get` when unobserved (reads are always current). It no longer
  registers a disposer on the current component.
- Reactive regions get an end anchor: SSR emits `<!--/{id}-->` after each
  region's content and the client keeps an empty comment there; node ids after
  a region shift by one (ADR-021).
- `focus`/`blur`/`scroll` handlers fire for the target element only (a
  container's `OnFocus` no longer fires for descendants; use `OnFocusIn`).
- A panic while a re-render diffs through an `h.ErrorBoundary` now renders the
  fallback (it used to keep a half-updated subtree); a later successful render
  restores the child (amends ADR-018).
- SSR renders run concurrently: `ssr.Renderer.Render` no longer holds a
  process-wide lock (server renders track no component frames; hooks see
  `EnvServer` via `runtime.ServerRender`) (ADR-024). (#72)
- Signal notification no longer allocates or looks subscribers up by id:
  `Set` is O(N) in subscribers (was O(N²)) and unsubscribe is O(1) — 1,000
  subscribers: ~505 µs → ~1 µs per `Set`. A panicking `core.Schedule` callback
  is now contained and logged (`recover.scheduled`) instead of aborting the
  frame. (#69)
- `h.For` renders only new and changed items (unchanged rows are reused as-is),
  the keyed diff's LIS is O(n log n) with an O(n) fast path when nothing moved,
  and `core.FlatTree` is shallow (children are flattened as the renderer
  reaches them). Appending one row to a 5,000-row list: 14 ms → 1.3 ms. (#70)
- Removing a subtree sends one `RemoveNode` per DOM root (plus portal/head
  content inside it) instead of one per descendant; the JS runtime forgets the
  removed subtree's nodes (and parked file handles) itself. Clearing 100
  five-node rows: 500 mutations → 100. `goowee.nodeCount()` exposes the tracked
  node count (the E2E test checks it for leaks). (#71)
- `h.ShowResource` shows exactly one view — loading, else error, else data
  (it used to show the data view alongside the loading view).
- `runtime/goowee.js`: an `InsertBefore` goes to its reference node's actual
  parent.
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

### Removed
- `ssr.HydrationMeta`, `ssr.SlotRef`, `Renderer.Meta` and
  `Renderer.RenderWithMeta` — unused leftovers of the pre-ADR-008 hydration
  design (they only cost allocations on every SSR render). (#79)

### Security
- `javascript:`/`vbscript:` URLs in URL attributes (`href`, `src`, `action`,
  `xlink:href`, …) are replaced with `about:blank#blocked` on the server and
  the client; attribute names are validated identically in body, head and DOM
  (and `xlink:href`/`xml:lang`/`data-a_b` are now allowed). (#78)

### Fixed
- Content a reactive region renders as several roots (a `For` list, a
  multi-root `Show` branch) is placed before the region's following siblings
  — appended rows used to land after a trailing footer — and a region
  replaced by a plain element is rendered (it used to render nothing). (#83)
- A nested `Show`/`For`/`Switch` inside a subtree that re-renders is adopted
  instead of torn down and rebuilt: components inside keep their state, and
  toggled content stays under its real parent. (#59)
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
- Event handlers bubble through ancestors, innermost first, until one stops
  propagation — they used to stop at the first handler, so a parent never saw
  a child's click and `StopPropagation()` did nothing (ADR-023). (#65)
- Non-bubbling events (`invalid`, `mouseenter`/`mouseleave`, `load`/`error`,
  media events, …) are listened for in the capture phase and reach their
  target's handler — `h.OnInvalid` never fired before. (#66)
- `router.Link` leaves cmd/ctrl/shift/alt and middle clicks to the browser
  (open in new tab/window) instead of navigating in-app. (#67)
- `h.Textf`/`core.Computed` created inside a re-rendering region no longer
  leak a subscription per re-render, and `Watch`/`UseEffect`/`OnMount` called
  in a region's render function are disposed with that render. (#58)
- `h.ErrorBoundary` releases the effects, bindings, handlers and scope
  subscriptions of whatever its child mounted before panicking. (#61)
- Router query params decode `%XX` escapes (`?q=go%20wasm` → `go wasm`), via
  `net/url`.
- The reference server's static-file path is cleaned (no `..` escapes).
- A bound or static `textContent` property is rendered as the element's
  content by SSR.
- Coalesced `scroll`/`pointermove` keep the latest event per element, so two
  containers scrolling in the same frame both update. (#68)
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

### Documentation
- `docs/guides/serving.md`: "What not to import" — `net/http` adds ~7 MB to
  the WASM binary (10.0 MB vs 2.7 MB for a minimal program, Go 1.25), with a
  context-aware `fetch` helper to use instead; `UseResource` points at it.
  (#54)
- `docs/plans/roadmap-1.0.md` refreshed to the current state, with a summary of
  the second review and what's left before 1.0.
- Guides, API reference and AGENTS.md corrected where they had drifted from
  the code (`ssr.Render`'s two results, `EventData` accessors, handler
  options, `Watch` timing, core type listings).

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

[Unreleased]: https://github.com/yogisalomo/goowee/compare/v0.2.7...HEAD
[0.2.7]: https://github.com/yogisalomo/goowee/compare/v0.2.6...v0.2.7
[0.2.6]: https://github.com/yogisalomo/goowee/compare/v0.2.5...v0.2.6
[0.2.5]: https://github.com/yogisalomo/goowee/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/yogisalomo/goowee/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/yogisalomo/goowee/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/yogisalomo/goowee/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/yogisalomo/goowee/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/yogisalomo/goowee/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yogisalomo/goowee/releases/tag/v0.1.0
