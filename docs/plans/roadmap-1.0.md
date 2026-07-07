# Road to 1.0 — Production Readiness

*Authored 2026-07-05.*

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

**0.1 Distribution & versioning.** *(license + module path: done.)*
Remaining: adopt semver, cut tagged releases (`v0.x`), and state a stability
policy — v0 means "expect breaking changes." Add a CHANGELOG. Until a real
release exists, nobody can pin a version. **S**

**0.2 User-facing documentation.** The repo has design/plan docs, not usage
docs. Needed: a Getting Started (scaffold → counter → build/serve), a Concepts
guide (signals, run-once components, scopes/`Show`/`For`, SSR + hydration), and
an API reference for the public surface (`h`, `hooks`, `core` signals,
`router`, `ssr`). The README should be the entry point, not a design dump. **M**

**0.3 Public vs internal API boundary.** Decide what is public and freeze it
for v0.x with a deprecation policy. Move genuinely-internal types behind
`internal/` so they can't be imported. Today everything in `core`/`dom`/`ssr`
is exported and importable, which makes every field a de-facto public contract. **M**

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
documented. *Remaining:* fully **automatic** per-subtree render-and-replace on
structural mismatch — deferred (needs renderer DOM access via a build tag or a
JS→Go round-trip; see ADR-016 alternatives).

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
E2E. *Gap:* update-time panics are contained but don't switch to the fallback
UI (ADR-018).

**2.3 Forms, inputs, focus.** ✅ **Mostly done.** `BindSelect` (change-based
`<select>`) and `BindValueLazy` (commit on change) join `BindValue`/
`BindChecked`; form/clipboard/focus events (`OnReset`, `OnInvalid`, `OnPaste`,
`OnCut`, `OnCopy`, `OnFocusIn`, `OnFocusOut`) and a `SelectOnFocus()` handler
option; and **caret preservation** — a `value` write to the focused text field
restores the selection instead of jumping to the end (programmatic updates
still apply). *Remaining:* explicit selection preservation across keyed
reorders (the active-element check covers the common case).

**2.4 Refs / DOM escape hatch & portals.** ✅ **Mostly done** (ADR-017).
`h.Ref`/`h.RefTo` give imperative command handles — `ref.Focus()`, `Blur()`,
`Click()`, `ScrollIntoView()` via a `MutInvoke` command to the bridge (the form
example's "Focus name" button dogfoods it, E2E-checked). `h.Portal(target, …)`
renders into another container (client-side, rendered fresh under hydration).
*Remaining:* ref **reads** (measure/`getBoundingClientRect`) need a value
channel back to Go; portals rebuild children on re-render (no id-keyed
reconciliation) — both deferred, documented in ADR-017.

---

## P3 — Performance & footprint (adoption-deciding, not correctness)

**3.1 Bundle size.** ~3.7 MB raw / ~1.0 MB gzip today. Evaluate TinyGo
(the reflect-free core helps), document gzip/brotli serving, and explore
route-level code splitting/lazy loading. **L**

**3.2 Boot latency.** 🟡 **Measurement landed.** `make boot` records a
median TTI phase split (download+compile / go boot+render / hydrate) in headless
Chromium — see `docs/plans/boot-latency-measurement.md`. Baseline finding:
download+compile of the ~4 MB binary dominates TTI, and the binary is served
*uncompressed* — gzip alone is a 3.7× download cut (folds into 3.1). *Remaining:*
an FCP mark to credit SSR's first-paint benefit, a real-network/throttled
baseline, and CI budget wiring (3.5). **M**

**3.3 Keyed-diff minimal moves.** The keyed reconciler re-inserts every row on a
list change (no longest-increasing-subsequence), so large lists do O(n) DOM
moves per update. Add LIS to move only what changed. **M**

**3.4 Mutation transport.** JSON-marshal per frame allocates; upgrade path is a
reusable buffer → structured clone via `js.ValueOf` → shared `ArrayBuffer`.
Coalesce high-frequency events (scroll/pointermove) to one per frame. **M**

**3.5 Profiling & regression budgets.** Extend the benchmark set (fan-out, list
re-render, deep trees), track numbers across releases, and add allocation
budgets to CI. **S–M**

---

## P4 — Maintainability & robustness

**4.1 Shared SSR/DOM walker.** One traversal that assigns ids and emits
backend-agnostic events (DOM → mutations, SSR → HTML), so id parity holds *by
construction* rather than via the golden test. Retires a whole class of future
divergence. Big refactor of two now-stable renderers — low urgency, high
long-term value. **L**

**4.2 Observability / devtools.** Consistent recover boundaries with structured
logging; a dev-mode inspector for the component/scope tree and signal graph.
Turns "why didn't this update" from guesswork into a tool. **M–L**

**4.3 Testing depth.** Run the browser E2E in CI (Chrome on the runner);
cross-browser smoke; more property tests for the differ and scheduler; consider
fuzzing the differ. **M**

---

## P5 — DX polish

**5.1 Examples & scaffolding.** A handful of real-shaped examples beyond the
counter; a `create-goowee-app`-style scaffold. **M**
**5.2 Templates (gated).** The `.gwx` compiler from the ergonomics plan
(Phase 3) — *only* with LSP + formatter + editor support, per the Vugu lesson
that a template format without tooling is negative value. **L**
**5.3 Router & a11y.** ✅ **Mostly done.** `SubRoute` (nested groups),
`Guard(check, fallback, route)`, `Lazy` (deferred handler init), and
`NavigateReplace`/`Back`/`Forward`; a set of `Aria*` attribute helpers +
`AutoFocus`. *Remaining:* `SubRoute`'s internal matcher isn't most-specific-
ordered like `Route` (fine for simple sub-routes), and nested-route children
aren't state-preserved across sub-navigation — refinements, not blockers.

---

## Definition of 1.0

1.0 should mean: a developer can `go get` a versioned release, follow the docs
to build a real SSR app with data loading, forms, SVG, and error handling;
signals are safe to update from timers/fetches; hydration survives realistic
content; bundle size and boot are documented and acceptable; and the public API
is stable with a deprecation policy. Everything through **P2** is the bar;
**P3** makes it competitive; **P4–P5** make it pleasant and durable.
