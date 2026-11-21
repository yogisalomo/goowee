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

- `goowee build` takes `--web <dir>` (static files in place of `web/`, e.g.
  pages prerendered with `ssr.Export`) and `--no-compress` (no `.gz`/`.br`, for
  hosts that compress on their own), and builds a main package anywhere inside
  a module. `make pages` now builds the tutorial site with it, so GitHub Pages
  serves the stripped `goowee_nodev` WASM and the runtime under content-hashed
  names (#145). Boot metrics find the wasm download by its resolved URL, so a
  hashed name is measured too.
- Events outside the mouse, keyboard and form families now carry their data
  (#210). Media events (`timeupdate`, `play`, `pause`, `ended`, `seeked`,
  `volumechange`, `progress`, …) carry the playback state when they fired:
  `e.CurrentTime()`, `e.Duration()`, `e.Paused()`, `e.Ended()`, `e.Volume()`,
  `e.Muted()`, `e.Buffered()`, `e.MediaError()` and more. `toggle` carries
  `e.NewState()`, a dialog's `close`/`cancel` `e.ReturnValue()`, transition and
  animation events `e.PropertyName()`/`e.AnimationName()` and
  `e.ElapsedTime()`, an image's `load` `e.NaturalWidth()`/`e.NaturalHeight()`,
  `scrollend` the scroll position, and `input`, `beforeinput` and composition
  events `e.InputData()`. New helpers: `OnToggle`, `OnClose`, `OnCancel`,
  `OnScrollEnd`, `OnBeforeInput`, `OnTransitionEnd`, `OnAnimationEnd`,
  `OnPlay`, `OnPause`, `OnEnded`, `OnTimeUpdate`, `OnSeeked`,
  `OnVolumeChange` and `OnLoadedMetadata`. `beforetoggle` and `loadstart` are
  now delivered to the target only, as they don't bubble.
- `h.Transition(name)` animates nodes inserted or removed by `Show`, `Switch`,
  `For` or a route (#156). The runtime adds `<name>-enter` on insert and holds
  a removed node with `<name>-leave` until `transitionend`/`animationend` (or a
  configurable timeout), then removes and forgets it. Go disposes the subtree's
  reactive state immediately and a leaving node is inert. Reduced-motion users
  skip the animation; `gooweetest` removes at once and records the request in
  `Screen.Transitions()`. The tutorial's todo rows now animate add and remove.
- The router now behaves like a browser navigation (#155). After a route's
  frame is applied, a push scrolls to the top, a `#fragment` scrolls to its
  element, and back/forward restore the position saved in that history entry
  (`history.scrollRestoration` is `"manual"`). Focus moves to the first
  `[data-route-focus]`, `h1` or `main`, and an `aria-live` region announces the
  document title. `router.ScrollOptions` (via `SetNavigationOptions`) can
  disable the behaviour, scope scrolling to a container, or skip focus and
  announcement. `r.NavLink` binds `active`/`aria-current` from one shared
  computed, and `r.BeforeLeave` registers a component-scoped navigation guard
  that is also asked on `beforeunload`. `core.AfterFrame` runs a callback once
  a frame's DOM mutations are applied.
- `h.VirtualList` can key rows by item instead of index (#157): pass a key
  function (`func(T) K`) as the second argument and an estimated item height via
  `VirtualListItemHeight`. Mounted rows are cached and reused by key, so state,
  focus and component instances survive inserts above the viewport; the legacy
  `int` row-height form still works. `VirtualListMeasure()` uses the runtime's
  `ResizeObserver` to use real row sizes with a Fenwick prefix-sum tree;
  `VirtualListHorizontal`, `VirtualListWindowScroll`, `VirtualListOverscan`,
  `VirtualListStickToBottom`, `VirtualListOnEndReached`, `VirtualListKeyFunc`
  and `VirtualListOnHandle` (with `ScrollToIndex`, `ScrollToKey`, `Refresh`)
  cover the remaining large-list cases. Only a changed `[first,last]` range
  re-renders the window.
- More form bindings and targeted DOM updates (#149): `BindNumber`, `BindRadio`,
  `BindSelectMulti` and `BindFiles` cover numeric inputs, radio groups,
  multi-selects and file pickers. `BindNumber` leaves an empty or partial
  number (`""`, `"-"`, `"1e"`) as the user typed it and keeps the signal at
  its last valid value. `ClassIf`/`Classes` toggle individual classes
  and `StyleProp`/`StylePx` set individual CSS properties, so a signal change no
  longer has to rewrite a whole `class` or `style` string and clobber another
  binding's work. `AttrS` (also `HiddenS` and the `Aria*S` helpers) writes a
  single attribute; boolean signals remove the attribute when false, and ARIA
  state helpers render `"true"`/`"false"`. The older `BindAttr` keeps writing
  `"true"`/`"false"` for bool signals, so existing ARIA state bindings don't
  change meaning. SSR renders the initial values of all of these, and
  gooweetest applies the same mutations as the browser. `EventData` gains
  `ValueAsNumber`, `ValueAsNumberOK` and `SelectedValues`.
- Typed derive helpers (#146): `core.Derive(sig, fn)` and `core.Derive2(a, b,
  fn)` build a computed signal from values, so a dependency can't be omitted
  from a hand-written deps slice. `core.Deps(a, b, c)` is the readable spelling
  for `Computed`'s many-dependency case. `h.ShowWhen(sig, cond, then)` and
  `hooks.Watch1(sig, fn)` bring the same shape to conditions and one-signal
  watches. They are thin wrappers over the existing primitives; laziness,
  versions and disposal are unchanged, and ADR-002's explicit dependencies still
  stand. The tutorial lessons and agent rules now use them where they shorten
  code.

### Fixed

- In dev mode, the inspector formatted every signal for its panel, and a
  value it couldn't format without `fmt` (such as `canvas.Fill`'s size)
  logged the "call core.UseFmt" warning and lit the overlay badge, though
  the app never binds that value. The inspector now shows the type's name
  and doesn't warn; binding such a value still warns.

## [0.4.0] — 2026-11-24

A minor release, and the biggest one so far. It adds a command line tool
(`goowee new`, `goowee dev` and `goowee build`), typed server functions,
forms with validation shared by the browser and the server, data loaded on
the server, islands and several roots per binary, static export, a keyed
list that updates one row per change, lazy booting of server-rendered pages,
a dev overlay, a `go vet` analyzer, and TinyGo as a supported target. Under
the hood, DOM updates cross to JavaScript in a binary format, events call
into Go through WebAssembly exports, bound updates no longer allocate, and
the client code no longer links `fmt` or `encoding/json`. A production build
of the smallest app is 660 KB brotli, down from 779 KB in 0.3.0, and about
160 KB with TinyGo.

Upgrading from 0.3.0:
- **Serve the matching runtime.** The binary and `goowee.js` now exchange a
  protocol version at boot, and a binary from this release won't mount
  against an older `goowee.js`: the page reports both versions instead.
  `ssr.Handler` serves the matching one; for static hosting copy it with
  `go run github.com/yogisalomo/goowee/cmd/goowee assets ./web`.
- **Read a browser event's payload with the accessors.** `EventData.Data` is
  set only on `EventData` built with `NewEventData`. Use `Key`, `Value`,
  `Number`, `String`, `Bool`, `Field` or `Fields`. Calling `PreventDefault`
  after the handler returned no longer affects the event.
- **Formatting a bound slice, map or struct needs `fmt` opted in.** goowee's
  client code no longer links `fmt` or `encoding/json`. In the browser, call
  `core.UseFmt(fmt.Sprintf)` if a bound value is a slice, map or struct
  (`Textf` does it for you), and `core.UseJSON(json.Marshal)` if a property
  value or `ref.Call` argument is a struct or typed slice. Without them goowee
  logs a warning. Host builds have both.
- **`router.NewURL` and `ssr.RoutedPage` are host-only.** In the browser use
  `router.New`.
- **Read mutation values with `Val`.** A bound attribute or property update
  carries its value in `Kind`, `Str` and `Bits`; code that read
  `core.Mutation.Value` should call `Val()`.
- **Routes remount when the matched pattern changes**, even if both routes
  render a component of the same name, and `/about/` is the same page as
  `/about`.

### Added
- `test/e2e/mount.mjs` measures mounting a fresh list three ways (per-node
  batch, HTML claimed like hydration, cloned row templates), and
  `BenchmarkMount1000*` its Go side. ADR-032 records why fresh subtrees stay
  per-node for now (#139).
- **The dev overlay** (#148): in dev mode, a badge counts errors and
  warnings, a new error (a contained panic, the cycle guard) opens a panel
  with its panic value, component and Go stack until dismissed, warnings
  list their fix hints, and an inspector shows the component tree, the
  signals and a live, filterable change log. It is written in goowee and
  mounted as a second root; `goowee_nodev` builds leave it out. Recovered
  panics now carry `stack` (and a re-render's `component`) in dev mode
  (`core.PanicFields`), and `devtools` keeps the log (`RecordLog`, `Logs`)
  and can keep a signal out of the change log (`Silence`).
- **`goowee/form`** (#150): a form's fields in one typed value, checked by a
  plain `Validate() form.Errors` method that the server runs too. `form.Use`
  holds the value, errors, touched fields and submit state; `form.Text`,
  `form.Int` and `form.Bool` bind inputs to fields (value, `name`,
  `aria-invalid`, blur-to-touched) without reflection; `f.OnSubmit` validates,
  runs the submit in a goroutine with `Submitting` set, cancels it on unmount,
  and shows a server's field errors (`*rpc.Error` now has `FieldErrors`) on
  their fields. The tutorial's form lesson and `examples/notes`' new-note
  form use it; the notes server checks the same `Validate`. Guide:
  `docs/guides/forms.md`.
- **`core.List` and `h.ForList`** (#140): a keyed list that records each
  change (`Append`, `Insert`, `Update`, `Remove`, `Move`, `Set`), and a `For`
  that applies just that change to its rows. An append mounts one row, an
  update sets one row's item signal, and a removal or move touches one row,
  with the same mutations and allocations at 10,000 rows as at 1,000 (an
  append: 5 mutations; an update: 1). `Set` compares rows by key as `For`
  does. Rows get their item as a signal. `core.ScopeNode.Patch` is the
  renderer hook it uses to change a scope's content in place.
- **`goowee.boot({when})`** (#130): a server-rendered page can boot `"idle"`
  (after load, when the browser is idle), `"visible"` (when an element
  scrolls into view) or `"interaction"` (at the first pointerdown, keydown or
  focusin) instead of at once. What the user does before the boot is queued
  and replayed (#131), including the interaction that starts it. A
  client-only page always boots at once; `?goowee-boot=` overrides the mode
  for trying one; `goowee.bootWhen` reports it, and `bootTimings` gains the
  trigger time and the load event. The shells have a preload hint for
  `main.wasm`, and the tutorial site's server-rendered pages boot on idle
  with a low-priority hint. `make boot` measures each mode, and
  `test/e2e/lazyboot.mjs` checks them in Chrome. The serving guide documents
  the hint and the modes.
- **`goowee new`, `goowee dev` and `goowee build`** (#145), in `cmd/goowee`
  (standard library only):
  - `goowee new <dir> [--ssr] [--tinygo]` writes a working project: an `app`
    package with gooweetest tests, the browser entry point, a page shell with
    a loading message and a preload hint, `AGENTS.md`, and with `--ssr` a
    server that renders each page.
  - `goowee dev` builds, serves with `application/wasm` and gzip, and on
    every change rebuilds and reloads the page; a build error shows over the
    page. Dev mode is on. `--proxy /api=http://…` forwards a path to another
    server; an SSR project's server runs behind it and restarts on change.
  - `goowee build` writes `dist/`: the WASM stripped and without dev tools
    (or built with TinyGo), content-hashed asset names that the pages refer
    to, `.gz` siblings (`.br` with the `brotli` command), and a size report
    by package.
  Getting Started now starts with these three commands. CI runs the whole
  flow, for both kinds of project, in Go and in Chrome (`test/e2e/cli.mjs`).
- TinyGo is a supported, CI-tested build target (#128). With TinyGo 0.44 and
  `-opt z -no-debug -stack-size=1MB`, the example is 299 KB brotli against
  989 KB with Go 1.25, and the minimal app 194 KB against 797 KB. CI builds the
  example with TinyGo and runs the smoke and devtools E2E against it
  (`test/e2e/tinygo.sh`), and `make size-check-tinygo` holds its size. The
  serving guide says when to choose which toolchain, and why the 1 MB stack is
  required.
- Mount anywhere, and islands (#154). `bridge.Mount(selector, node)` mounts a
  root into any element and returns, so one binary can run several roots
  sharing one scheduler and frame loop; `root.Unmount()` removes one, and
  `bridge.Wait()` keeps the program alive. `ssr.Island(name, node, props)`
  renders a component into any server page (html/template, templ) with its
  props as JSON, and `bridge.Islands(factories)` hydrates every island on
  the page independently. `bridge.Run` is now `Mount("#root")` plus `Wait`.
  Guide: "Adding goowee to an existing Go site" (`docs/guides/islands.md`).
- `goowee/rpc`: typed server functions (#151). `rpc.Define[Req, Res](name)`
  in a package both sides import; `rpc.Handle(mux, f, fn)` on the server
  (same-origin and JSON-only by default, `rpc.Before` for auth); `f.Call(ctx,
  req)` and `rpc.UseQuery(f, reqSignal)` on the client; failures are
  `*rpc.Error` values with a code, a message and per-field messages. In the
  server's process a call runs the handler directly; `rpc.Fake` stubs one in
  tests. New example `examples/notes` (list, detail, form, server-rendered)
  and the guide "Talking to your server" (`docs/guides/server.md`).
- Server-side data (#152, ADR-030). `hooks.Load(key, deps, fetch, render)` is
  `UseResource` whose data the server loads too: under `ssr.Handler` and
  `ssr.Export` the server runs each keyed fetch (concurrently, with the
  request's context and `HandlerOptions.DataTimeout`), renders the page with
  the data, and embeds it as JSON; the client starts with it and doesn't
  fetch again. The tutorial's `/async` page uses it.
- `gooweevet`, a `go vet` analyzer for goowee's reactivity rules (#147), in
  its own module so the framework keeps no dependencies:
  `go vet -vettool=$(which gooweevet) ./...`. It reports a `sig.Get()` passed
  to a static helper in a component body (`Text(count.Get())`), a `Computed`,
  `UseEffect`, `Watch` or `UseScope` reading a signal its deps don't list, and
  a signal `Set` or `UseState` setter called in a goroutine or
  `time.AfterFunc` outside `core.Schedule`. CI runs it on the examples.
- `FuzzDiffer` (`gooweetest`): after any sequence of signal writes and list
  edits over a generated tree (elements, SVG, text, fragments, `Show`,
  `Switch`, keyed `For`, components, portals), the document must equal a
  fresh render of the final state, with registries and mounted components
  matching. CI fuzzes it for 45 s per PR (#161).
- `goowee/web` exposes the browser assets that must match a build: `web.RuntimeJS()`, `web.Version`, `web.Handler()` and `web.WasmExecJS()` (#144). `ssr.Handler` serves any path ending in `goowee.js` through it, with an ETag and `Cache-Control: no-cache`, so a server-rendered app cannot load a stale runtime. `wasm_exec.js` stays with the app's static files: the deployed server may have no Go toolchain. `go run github.com/yogisalomo/goowee/cmd/goowee assets ./web` copies both files for static hosting, taking goowee's version from the app's `go.mod`; the guides use it instead of globbing the module cache. The binary and runtime now exchange a protocol version at boot: a mismatch stops before mounting and reports both versions and the command that fixes it.
- `ssr.Export` prerenders a site to static files for hosts without a Go server
  (#153). Each page is rendered with the same `Page` and `Document` functions
  as `ssr.Handler` and written as `<path>/index.html`, plus `404.html` from
  the not-found route. Pages come from `Paths`, optionally crawling in-app
  links, and `Base` serves the site under a sub-path (`ssr.BasePath`, which
  `RoutedPage` passes to the router). Each file is marked with the path it
  was rendered for, and the runtime hydrates it only at that URL. A static
  host's `404.html` served for a page nobody prerendered is dropped and
  rendered on the client instead. The tutorial site is now prerendered
  (`cmd/site-export`, `make pages`).
- `goowee.boot({transport: "json"})` selects the JSON transport, for
  comparing the two or as a workaround; `goowee.transport` reports the one in
  use.
- `test/e2e/transport.mjs` measures both transports in headless Chrome,
  replaying real frames and timing the real WASM app frame by frame;
  `test/e2e/batch.mjs` (part of `make e2e`) checks the runtime's binary
  decoder against `JSON.parse` on a Go-written corpus; the E2E smoke test runs
  on both transports and on an older `goowee.js`.

### Changed
- **An event reaches its handler without decoding its payload** (#135). The
  payload stays the JSON text the runtime sent, and each accessor (`Key`,
  `ClientX`, `Value`, …) finds its field in it without decoding the rest or
  allocating. Delivering a `pointermove` to a handler that reads its position
  allocates nothing in Go besides the payload text itself. `EventData.Data`
  is now set only on `EventData` built with `NewEventData` (tests, custom
  dispatchers); read a browser event's payload with the accessors, the new
  `Number`, `String` and `Bool` for fields without one, `Field`, or `Fields`
  for the whole map. A handler that keeps its `EventData` can no longer
  prevent or stop a later event by calling `PreventDefault` after it
  returned.
- **goowee's client code no longer links `fmt`, `encoding/json`, `log` or
  `net/url`** (#126), so an app pays for them only if it uses them. The
  smallest app (`test/e2e/hello`, now written with `strconv` instead of
  `fmt`) went from 812 KB to 675 KB brotli, and under TinyGo from 199 KB to
  116 KB. Event payloads and read replies are read by a small JSON reader
  (fuzzed against `encoding/json`); `core.Log` in the browser goes to the
  console sink only; the router escapes queries itself; bound values are
  formatted by `core.FormatValue`, which matches `%v`. The serving guide has
  a table of what each helper brings in. CI checks that `hello` links none
  of `fmt`, `log` and `net/url`, and only `encoding/json`'s package init.
  - New: `core.FormatValue`, `core.UseFmt` and `core.UseJSON`. A bound slice,
    map or struct needs `core.UseFmt(fmt.Sprintf)` in the browser (`Textf`
    calls it), and a struct or typed slice as a property value or
    `ref.Call` argument needs `core.UseJSON(json.Marshal)`; without them
    goowee logs a warning. Host builds always have both.
  - `router.NewURL` is no longer in browser builds; use `router.New`.
- Events and frames call into Go through WebAssembly exports instead of
  `syscall/js` callbacks, and the runtime reads each frame's batch where it is
  in the module's memory instead of from a copy (#133, ADR-031). In Chrome a
  click that reaches a Go handler costs about 26 µs instead of 38 µs, and a
  one-update frame about 55 µs instead of 95 µs. Goroutines a handler or an
  effect starts still run right after it. `goowee.boot` sets this up; a page
  that instantiates the module itself can call `goowee.useInstance(go,
  instance)` before `go.run`, or keeps the callbacks. `goowee.boot({calls:
  "callbacks"})` keeps them too, and `goowee.calls` reports which is in use.
  TinyGo builds, and pages with an older `goowee.js`, keep the callbacks.
- **A bound update no longer allocates** (#136). A string, float64, int or
  bool signal hands its value to the binding typed, and the encoders format
  it the way `%v` did, so nothing is boxed or formatted into a new string.
  Each binding keeps its place in the frame's queue, so a second write in the
  same frame replaces its update and the frame needs no coalescing map. The
  scheduler reuses its queue and its re-render bookkeeping from frame to
  frame, and `Set` compares the common types without reflection. A frame of
  6,000 bound updates allocates 17 times instead of about 15,500.
  `BenchmarkLayoutTickFrame` under WASM drops from 9.7 ms to 2.1 ms, and in
  Chrome the 5,000-update layout tick (signal writes and frame) from 13.7 ms
  to 4.5 ms, with a quarter of the garbage collections.
  `core.Mutation` gains `Kind`, `Str` and `Bits` for such values, with `Val`
  to read any mutation's value; code that reads `Value` of a bound
  attribute or property update should use `Val`.
- **DOM updates cross from Go to JavaScript in a binary format** instead of
  JSON (ADR-029, #134). The encoder reuses its buffers, so a frame no longer
  allocates its own payload, and the runtime decodes each batch's strings with
  one call. On a 5,000-update frame in Chrome the payload is 74% smaller
  (255 → 67 KiB) and the frame is about 62% faster (8.9 → 3.4 ms on an M1),
  most of the saving being Go garbage collection the JSON frame's
  allocations paid for. Small frames (1 to 10 updates) cost about the same
  end to end. The runtime sees the same values as before. No API changes. The wasm and `goowee.js` should come from the same goowee
  version: with an older `goowee.js` the bridge falls back to JSON and logs a
  warning.

- A re-render no longer rebinds every bound attribute, property and text it
  rebuilds, or re-sends their values (#137). A binding whose signal is the
  same keeps its subscription and sends nothing. One whose signal changed (a
  `Textf` makes a new one each render) sends its value only if it differs.
  A no-op re-render of 100 rows with a bound class and text went from 200
  mutations and 1,834 allocations to none and about 520 (building the rows).

### Fixed
- `ssr.RoutedPage` is host-only, like `router.NewURL` it calls (#126), so
  packages whose tests import `ssr` (internal/dom) build for WASM again and
  the WASM benchmarks in `docs/perf/README.md` run.
- Moving between two routes whose functions return a component of the same
  name kept the first route's instance on screen: the URL changed but the
  page didn't (#124). A route's content is now keyed by the pattern that
  matched it, in `Route` and `SubRoute`, so a different route remounts and the
  same pattern with new params keeps it.
- Without `runtime.Callers` (TinyGo), the change log reported a write's site
  as ":0" and dependency warnings were cut to one for the whole program. The
  change log now says "unknown", and warnings without a site are kept once
  per signal.
- Found by the new differ fuzzer (#161):
  - A re-render that reused an element but dropped one of its bindings left
    the value that binding set (a bound `class` stayed). The differ now
    removes the attribute, or resets the property.
  - Switching `Switch` or `ShowElse` branches kept the old branch's
    component when the new branch rendered one of the same name, and kept
    the old branch's `For`, portal or boundary content. Branch content is
    now keyed by the branch.
  - A plain component row in a `For` kept its old item when its item
    changed and the `For` was built afresh (inside a parent row that
    re-rendered), or when the row had a duplicate key. Such rows are now
    keyed by their item too, so a changed item remounts them.
- On a server-rendered page, what the user did before the WASM booted was lost
  (#131). Clicks went nowhere, a form submitted natively (reloading the page),
  and text typed into a bound input stayed out of its signal, to be
  overwritten later. `goowee.js` now queues clicks, keydowns and submits on
  server markup until the first frame and dispatches them once each after it.
  Submits are held, then resubmitted, or released natively if the WASM fails
  to load. Form controls whose state the user changed are announced as
  `input` and `change`, so bound signals catch up. `<html data-goowee-pending>`
  marks the page until then.
- Hydration of an empty text binding (`TextS` of `""`): the server writes only
  its marker, and the client found no text node to claim, logging "no server
  node" and rebuilding it. The marker now becomes the empty text node.
- The router treats a trailing slash as the same page (`/about/` is
  `/about`), as static hosts serve directory URLs.

## [0.3.0] — 2026-11-18

A minor release. It adds drawing on `<canvas>` from Go (`goowee/canvas`,
with animation through `core.NextFrame`), persisted signals, a dev-mode change
log, and a build tag that leaves dev tools out of production builds. It also
lands the six quick wins from the November review (#162): cheaper event
dispatch, effects that run once per batch, router URL fixes, a logged
re-render cycle, and a size budget in CI.

Upgrading from 0.2.7:
- **Serve the matching runtime.** Serve this release's `goowee.js` with the
  app. Event dispatch uses the new `core.MutListen` mutation, and canvas
  drawing uses `goowee.draw`. With an older `goowee.js`, events still work the
  old, slower way, a warning is logged, and canvases stay blank. This
  `goowee.js` works with an older app too.
- **Watch and UseEffect run once per batch.** A batch that sets two of their
  deps now runs the callback once, after both writes. Code that counted on one
  run per dependency sees fewer runs.
- **gooweetest steps animations explicitly.** `Flush` doesn't run
  `core.NextFrame` callbacks; call `screen.Frame()` to step them.
- **Query URLs come out sorted.** `SetQueryParam` writes parameters sorted by
  name and keeps the fragment, so a URL a test compared against may change
  order.

### Added
- `hooks.UsePersist(sig, key)`: keeps a signal in localStorage (or, with
  `hooks.PersistOptions{Store: storage.Session}`, sessionStorage) as JSON. It
  loads once the component has mounted, so a server-rendered page hydrates
  with the server's markup; a stored value that no longer decodes is logged
  and left alone.
- Dev mode (`?goowee-dev`) change log: `goowee.changes()` lists each signal
  write with its value before and after and the code that made it (the first
  caller outside goowee, e.g. `app/store.go:31 app.(*Store).AddToCart`); in Go,
  `devtools.Changes()` and `devtools.ClearChanges()`.
- `sig.Named("cart")` labels a signal in the inspector and the change log;
  a no-op without dev mode. Signals in a package-level store are named before
  dev mode is known, and the browser build keeps those names until it is. The
  router names its signals `router.path`, `router.query` and `router.params`.
- `goowee/canvas` (#159): draw on a `<canvas>` from Go. `canvas.View(Size,
  Draw(deps, fn), Label, Attrs, Fallback, NoClear)` records a frame into a
  command buffer whenever a dep changes, and the runtime replays it in one
  call (the #159 spike measured 2–3 ms for 10,000 lines and 10,000 circles,
  against 15–17 ms through `syscall/js`). Coordinates are CSS pixels, scaled for the device pixel ratio.
  `core.Ref.Draw` sends a buffer recorded by hand; gooweetest's
  `Element.Canvas()` and `DrawCount()` check what was drawn. New mutation type
  `core.MutDraw`; the runtime and the app must come from the same goowee version
  (`goowee.canvasVersion`). The tutorial has a Canvas lesson.
- `canvas.Fill()`: a canvas that follows its element's size instead of a fixed
  `Size`, redrawn when it changes. The runtime also replays every canvas when
  the device pixel ratio changes (browser zoom, another screen).
- `core.NextFrame(fn)` runs `fn` at the start of the next frame; call it again
  from `fn` to animate at the display's refresh rate. Frames pause while the
  page is hidden. `canvas.Animate(fn)` draws one picture per frame. In
  gooweetest, `screen.Frame()` steps animations (they no longer advance on
  `Flush`), and `screen.ResizeCanvas(el, w, h)` lays out a `Fill` canvas.
- `canvas.Stack` draws in layers, one canvas each, redrawn independently;
  `canvas.Options` combines options; `canvas.Grid` finds the mark under the
  pointer.
- A custom event's `detail`, when it is a plain object, reaches the handler as
  `e.Data`.
- `examples/graph`: 10,000 nodes and 10,000 edges redrawn every frame at 60 fps
  (also on the tutorial site at `/graph`); `make canvasbench` measures its frame
  time (`docs/perf/canvas.md`). ADR-028 records the canvas design.
- The inspector lists a `core.NewSignal` signal once it is named or written,
  and formats struct values with field names (`{Name:tea Price:3}`).
- `r.Fragment`, the URL's fragment as a signal, following `Navigate`, back and
  forward; and `r.QueryValues(name)`, every value of a repeated parameter
  (`?tag=a&tag=b`).
- `make size-report` and `make size-check` (#129): raw, gzip and brotli sizes
  of the minimal app and the example site, with code bytes per Go package
  (`scripts/wasm-size.mjs`). CI now fails when either program's brotli size
  grows more than 3% over its record in `docs/perf/size-budget.json`, or when
  it links a package on its deny list. This replaces the raw 6 MiB limit.
- `-tags goowee_nodev` builds the app without dev tools (#127): no inspector,
  no change log and no dependency checks, and no `goowee/devtools` code linked.
  It saves about 105 KB of wasm (15 KB after brotli) on the minimal app.
  `goowee.inspect()` and `goowee.changes()` then say the build has none.
  Default builds are unchanged.

### Changed
- Events call into Go only for elements that have a handler for them (#132).
  The renderer marks those with a new `core.MutListen` mutation, and the
  runtime skips every other ancestor while bubbling, building the payload only
  when a handler is on the path. A click with no handler on its path no longer
  crosses into Go at all, and one with a single handler crosses once instead of
  once per ancestor (each crossing is about 20 µs). `handleEvent` returns its
  result as packed bits instead of an object, unless the page's `goowee.js`
  predates 0.3.0. Then it answers in the old format and logs a warning, so
  `PreventDefault` and `StopPropagation` keep working. gooweetest's `Dispatch`
  follows the same rule.

### Fixed
- Router URLs (#142): `SetQueryParam` and `SetQueryParamPush` wrote the
  parameters in random order, left a bare `?` once the last one was removed,
  and dropped the fragment. They now write parameters sorted by name, keep the
  fragment, and omit an empty query. Back and forward set the path, query and
  fragment in one batch, so a `Watch` on the path no longer sees the old query.
- A render that kept dirtying scopes stopped after 100 passes without a word
  and left them dirty, to burn another 100 passes at the next unrelated update
  (#143). The flush now logs `signal.cycle` once, with how many scopes were
  left and, in dev mode, their component paths, and drops them so the next
  frame starts clean.
- `hooks.Watch` and `hooks.UseEffect` ran once per changed dependency when one
  batch wrote several of them (an event handler setting a page and a filter,
  `Navigate` setting the path and query); they now run once per batch, after
  every write (#138). So `UseResource` over two deps that change together
  fetches once instead of starting and cancelling a fetch. An effect that
  writes one of its own deps runs again with the new value, and one that keeps
  doing so is stopped after 1,000 rounds with a `signal.cycle` log. The
  mechanism is public as `core.Coalesce(fn)`.

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

[Unreleased]: https://github.com/yogisalomo/goowee/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/yogisalomo/goowee/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/yogisalomo/goowee/compare/v0.2.7...v0.3.0
[0.2.7]: https://github.com/yogisalomo/goowee/compare/v0.2.6...v0.2.7
[0.2.6]: https://github.com/yogisalomo/goowee/compare/v0.2.5...v0.2.6
[0.2.5]: https://github.com/yogisalomo/goowee/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/yogisalomo/goowee/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/yogisalomo/goowee/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/yogisalomo/goowee/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/yogisalomo/goowee/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/yogisalomo/goowee/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yogisalomo/goowee/releases/tag/v0.1.0
