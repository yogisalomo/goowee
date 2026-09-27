# Road to 1.0 — Production Readiness

*Authored 2026-07-05. Status refreshed 2026-09-26 after the second review
(issues #57–#83; see "Second review" below).*

The core framework has been hardened end-to-end against the architecture review
(`docs/26-07-02-fable-review.md`, §1–§10): sound reactive core (token-based
subscriptions, reentrancy-safe notify), Solid-style run-once components with
reconciliation, keyed lists, dirty-scope render batching, request-safe SSR,
working claim-based hydration, typed events, `Computed`, deterministic routing
with params + history, plus benchmarks, a WASM size budget, CI, and a headless
browser E2E. That work makes goowee a legitimate **experimental (v0)** project.

This document is what stands between "shareable v0" and "others can build
production apps on it." Items are ordered by how much each blocks **production
usability** — do them top to bottom. Effort tags are rough (S/M/L).

---

## P0 — Adoptability (you can't use what you can't get, version, or learn)

**0.1 Distribution & versioning.** ✅ **Done at v0.1.0.** License + module path
(`github.com/yogisalomo/goowee`), semver adopted, the v0 stability/deprecation
policy written (`docs/api-stability.md`), and a `CHANGELOG.md` covering the
first-release surface. `v0.1.0` is the first tagged release, so downstreams (the
personal site, `goowee-markdown`) can now pin a version instead of a commit.
`v0.2.0` (2026-09-26) ships the second review, with an "Upgrading from 0.1"
guide in the CHANGELOG for its breaking changes; `v0.2.1` (2026-09-28) adds
`r.LinkTo`/`r.SetBasePath` and aligns the tutorial site. **S**

**0.2 User-facing documentation.** ✅ **Done** (#39). Getting Started, a
Concepts guide, and an API reference, kept current with each change (the second
review corrected several snippets that had drifted from the code: `Render`'s
two results, `EventData` accessors, handler options, `Watch` timing). **M**

**0.3 Public vs internal API boundary.** 🟡 **Mostly done.** The public API is
now defined and the v0 stability/deprecation policy written (`docs/api-stability.md`):
public packages are `h`/`hooks`/`router`/`ssr`/`bridge` plus a named subset of
`core`. The **whole `dom` renderer moved to `internal/dom`** so external modules
can't import it, and a single client entry point `bridge.Run(node)` replaced the
old `dom.New()`/`renderer.Render`/`bridge.Init` dance that leaked
`renderer.Scheduler`/`Registry` (dead `NoopBridge`/`Bridge` removed). *Remaining:*
`core` still exports framework internals (scheduler, mutations, render context,
node structs) alongside its public subset — hiding those needs a `core` split
into public/internal halves (an L refactor, related to P4.1); for now the boundary
is documented rather than compiler-enforced. The public `core` subset now also
lists `Batch`, `Peek`, `ComponentWithProps` and `SetKey`. **M**

---

## P1 — Correctness footguns (silent breakage in real apps)

**1.1 Concurrency model.** ✅ **Done.** `core.Schedule(fn)` runs off-loop
updates (timers, network, channels) on the render loop via the scheduler's
mutex-guarded `Post` inbox, drained at flush; the client registers its
scheduler in `bridge.Init`. The rule — *touch signals only on the render loop;
from a goroutine, wrap the update in `core.Schedule`* — is documented (ADR-015)
and demonstrated by the stopwatch ticker. Covered by a `-race` test and the E2E
(the stopwatch advances via a goroutine). *Still a documented rule, not
type-enforced (ADR-015 consequences).*

**1.2 Hydration mismatch recovery.** ✅ **Mostly done** (ADR-016). The escape
hatch shipped: `h.Dynamic()` marks a non-deterministic subtree so hydration
claims the server nodes but re-applies the client's values (client wins), while
deterministic content keeps the boot optimization. Structural mismatches (wrong
tag / missing node) now log a clear `console.error` and best-effort recover
instead of silently corrupting. The hydration contract (deterministic markup)
and the placeholder-plus-`OnMount` pattern for client-only values are
documented. Scope end anchors (ADR-021) are claimed like other nodes, and the
E2E now fails on any hydration-mismatch console error. *Remaining:* fully
**automatic** per-subtree render-and-replace on structural mismatch — deferred
(ADR-016); text inside raw-text elements (`<textarea>`, `<title>`) still can't
be hydrated (ADR-009), so such pages are client-only (`ssr.HandlerOptions.ClientOnly`).

---

## P2 — Feature completeness for real UIs

**2.1 SVG / namespaced elements.** ✅ **Done.** `h.Svg()` carries the SVG
namespace; descendants inherit it (only the root is marked), the renderer emits
it on `CreateElement`, and the bridge creates namespaced nodes with
`createElementNS`. `h.SvgEl` + shape helpers (`Path`, `Circle`, `Rect`, `G`,
`Line`, `Polyline`, `Polygon`, `Ellipse`). The site logo is now inline SVG
(dogfood). Covered by a renderer unit test + an E2E `namespaceURI` check.
*Limitation:* a re-rendering scope whose root is an SVG child (not the `<svg>`
itself) won't inherit the namespace — render SVG as a unit for now.

**2.2 Async data & error boundaries.** _Async data:_ ✅ **Done.**
`hooks.UseResource(deps, fetch)` returns a `Resource[T]` with `Data`/`Loading`/
`Err` signals + `Refetch`; the fetcher runs in a goroutine and its result is
applied on the render loop via `core.Schedule` (with a generation guard so a
stale fetch can't overwrite a newer one). Fetches on mount and on dep change;
client-side (server renders the loading state). The `/async` tutorial page
dogfoods it; unit tests (`-race`) + E2E. _Error boundaries:_ ✅ **Done**
(ADR-018). `h.ErrorBoundary(fallback, child)` catches a render-time panic in
the subtree and shows `fallback(err)` (rolling back renderer state so recovery
is clean); update-time panics are **contained** by a `recover` in
`reRenderScope` (the subtree keeps its previous state, logged) so a panicking
update never blanks the page. `/error` tutorial page dogfoods it; unit tests +
E2E. A panic while a re-render diffs through a boundary now shows the fallback,
and everything a failed walk set up is released (ADR-021, #61). `UseResource`
fetches take a cancellable `context.Context` (ADR-025, #75). *Remaining:*
server-side data loading for resources — deferred by design (ADR-025: it needs
per-request state reachable from hooks, which conflicts with lock-free SSR).

**2.3 Forms, inputs, focus.** ✅ **Mostly done.** `BindSelect` (change-based
`<select>`) and `BindValueLazy` (commit on change) join `BindValue`/
`BindChecked`; form/clipboard/focus events (`OnReset`, `OnInvalid`, `OnPaste`,
`OnCut`, `OnCopy`, `OnFocusIn`, `OnFocusOut`) and a `SelectOnFocus()` handler
option; and **caret preservation** — a `value` write to the focused text field
restores the selection instead of jumping to the end (programmatic updates
still apply). *Remaining:* explicit selection preservation across keyed
reorders (the active-element check covers the common case).

**2.4 Refs / DOM escape hatch & portals.** ✅ **Mostly done** (ADR-017, ADR-019).
`h.Ref`/`h.RefTo` give imperative command handles — `ref.Focus()`, `Blur()`,
`Click()`, `ScrollIntoView()` via a `MutInvoke` command to the bridge — and a
value channel back: `ref.Get(prop, fn)` rides the batch as a `MutRead`, is
answered after that frame's writes, and runs `fn` on the render loop
(measuring, `getBoundingClientRect`, selection). File inputs expose
`e.Files()` metadata and `File.Bytes()` for the contents (#51). The form
example dogfoods all three, E2E-checked. `h.Portal(target, …)` renders into
another container (client-side, rendered fresh under hydration).
`bridge.Element(ref)` hands the live element to a JavaScript library, and
`OnMount` runs after the DOM exists, so that works in one step (ADR-022, #53,
#60). *Remaining:* portals rebuild children on re-render (no id-keyed
reconciliation) — deferred, documented in ADR-017.

---

## P3 — Performance & footprint (adoption-deciding, not correctness)

**3.1 Bundle size.** ~4.7 MB raw / ~1.2 MB gzip for the example app today.
`docs/guides/serving.md` now warns that `net/http` alone adds ~7 MB (#54) and
gives a `fetch` helper instead. TinyGo is a dead end for now
(`docs/plans/tinygo-recover-implementation.md`). *gzip/brotli serving:*
✅ **Done** — documented in `docs/serving.md`; the reference SSR server gzips the
wasm (3.7×), JS/CSS, and SSR HTML, and GitHub Pages gzips via its CDN. *Remaining:*
evaluate TinyGo (the reflect-free core helps) against the *post-compression*
download, and explore route-level code splitting/lazy loading. **L**

**3.2 Boot latency.** 🟡 **Measurement landed.** `make boot` records a
median TTI phase split (download+compile / go boot+render / hydrate) in headless
Chromium — see `docs/plans/boot-latency-measurement.md`. Baseline finding:
download+compile of the ~4 MB binary dominates TTI. The 3.7× gzip download cut it
surfaced is now served (see 3.1 / `docs/serving.md`), and `make boot` supports
`THROTTLE=4g|fast3g|slow3g` to measure it on an emulated network. An **FCP mark**
now lands in `bootTimings()` and the `make boot` report — it credits SSR (server
HTML paints before WASM boots, so SSR's FCP sits well below its TTI, while a
client render's FCP ≈ TTI). Allocation-budget gating is wired in CI (see 3.5),
and the WASM size budget already gates CI. *Remaining:* a CI **TTI** budget stays
deferred — headless-runner timing variance makes an absolute TTI gate flaky. **M**

**3.3 Keyed-diff minimal moves.** ✅ **Done.** The keyed reconciler computes a
longest-increasing-subsequence over retained rows (`lis` / `computeNeedsMove` in
`internal/dom/diff.go`, now O(n log n) with an O(n) no-move fast path) and emits
`InsertBefore` only for rows outside it. `h.For` reuses unchanged rows as-is,
and removals send only subtree roots: appending to a 5,000-row list went from
14 ms to 1.3 ms (#70, #71).

**3.4 Mutation transport.** 🟡 **Partial.** High-frequency events
(scroll/pointermove) are coalesced to one per frame per element (#31, #68). *Remaining:* the
binary-transport upgrade (reusable buffer → shared `ArrayBuffer`) stays deferred —
measure a large-graph/drag workload first; JSON-marshal per frame is fine until
then. **M**

**3.5 Profiling & regression budgets.** ✅ **Done.** Benchmarks cover signal
fan-out, coalesce, list re-render, tree diff, and now deep trees
(`BenchmarkDeepTreeRender`). Allocation-budget tests (`testing.AllocsPerRun`,
tagged `!race`) guard the hot paths — O(1) signal notify (zero allocations;
notification itself went from O(N²) to O(N), #69), bounded coalesce, bounded
diff — and CI runs them in a dedicated non-race step. SSR renders run
concurrently (ADR-024, #72). *Remaining:*
tracking numbers across releases is still manual (no perf dashboard). **S–M**

---

## P4 — Maintainability & robustness

**4.1 Shared SSR/DOM walker.** ✅ **Done** (#41). `internal/walker` does one
traversal that assigns ids and calls a `Visitor`; the DOM renderer materializes
mutations and the SSR renderer materializes HTML, so id parity holds *by
construction*. `TestSSRDOMIDParity` stays as a belt-and-suspenders guard. **L**

**4.2 Observability / devtools.** ✅ **Done.** Consistent recover boundaries
emit structured `core.Log` entries (`recover.*`, `signal.cycle`, `warn` kinds);
the WASM bridge forwards them to `goowee.log()` → `console`. Dev-mode inspector
(`?goowee-dev` or `localStorage`) exposes `goowee.inspect()` for the
component/scope tree and signal graph. See `docs/devtools.md`. **M–L**

**4.3 Testing depth.** ✅ **Mostly done.** The headless-Chrome E2E now runs in
CI — an `e2e` job (`browser-actions/setup-chrome` + `make e2e`) covering
hydration, interactivity, routing/history, refs, async data, error boundary, the
off-loop stopwatch, and the devtools inspector. (Wiring it in also caught a
long-broken `devtools.mjs` — a duplicate `const` made it un-parseable.)
App authors get the same headless setup through the public `gooweetest`
package (#76), and property tests cover keyed lists between siblings and the
LIS. *Remaining:* cross-browser smoke and differ fuzzing. **M**

---

## P5 — DX polish

**5.1 Examples & scaffolding.** 🟡 The tutorial gained an Events lesson and a
canvas chart (JS interop); `ssr.Handler` makes the SSR server a few lines.
*Remaining:* a real-shaped second example app and a `create-goowee-app`-style
scaffold. **M**
**5.2 Templates (gated).** The `.gwx` compiler from the ergonomics plan
(Phase 3) — *only* with LSP + formatter + editor support, per the Vugu lesson
that a template format without tooling is negative value. **L**
**5.3 Router & a11y.** ✅ **Mostly done.** `SubRoute` (nested groups),
`Guard(check, fallback, route)`, `Lazy` (deferred handler init), and
`NavigateReplace`/`Back`/`Forward`; a set of `Aria*` attribute helpers +
`AutoFocus`. `SubRoute` now matches deterministically and merges params (#62); `Link` leaves
modifier/middle clicks to the browser (#67); `Router.NotFound` drives real 404s
under SSR (#77).

---

## Second review (2026-09-26)

A fresh full review found silent-staleness, lifecycle, placement and event bugs
beneath the hardened surface; all were reproduced with tests and fixed:

| Area | Issues | Decision |
|---|---|---|
| Placement & ownership: scope anchors, adoption, lazy `Computed`, boundary cleanup | #58 #59 #61 #83 | ADR-021 |
| Lifecycle: effects after the DOM is applied; `bridge.Element` | #53 #56 #60 | ADR-022 |
| Stale props: `ComponentWithProps`, name + key identity, `For` item changes | #57 | ADR-001 amendment |
| Batching & signal performance | #69 #73 | ADR-020 |
| Events: bubbling, non-bubbling capture, per-event decisions, data | #65–#68 | ADR-023 |
| SSR: lock-free renders, `ssr.Handler`, script-URL blocking | #72 #77 #78 #79 | ADR-024 |
| Resources: cancellable fetch | #75 | ADR-025 |
| Dev-mode undeclared-dependency check | #74 | ADR-002 amendment |
| Smaller fixes (router, differ robustness, list perf, tests harness) | #62–#64 #70 #71 #76 | — |

What's left before 1.0, in order: server-side resource data (needs a design,
ADR-025), the `core` public/internal split (0.3), a scaffold + second example
(5.1), cross-browser smoke + differ fuzzing (4.3), and automatic hydration
mismatch recovery (1.2).

## Definition of 1.0

1.0 should mean: a developer can `go get` a versioned release, follow the docs
to build a real SSR app with data loading, forms, SVG, and error handling;
signals are safe to update from timers/fetches; hydration survives realistic
content; bundle size and boot are documented and acceptable; and the public API
is stable with a deprecation policy. Everything through **P2** is the bar;
**P3** makes it competitive; **P4–P5** make it pleasant and durable.
