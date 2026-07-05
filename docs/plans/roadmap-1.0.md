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

**1.1 Concurrency model.** Signals have no locking; they are correct *only*
because Go WASM is single-threaded today. A timer/goroutine/fetch that calls
`Set` concurrently with a render is a data race waiting to happen (and will
break if Go WASM ever gets threads). Define the rule ("signals are read/written
on the render loop only") and provide a safe cross-goroutine update path — e.g.
route external updates through a scheduler channel/`Post(fn)` that runs on the
frame loop. Document it prominently; the stopwatch `OnMount` timer is the
canonical example. **M**

**1.2 Hydration mismatch recovery.** Hydration trusts SSR/DOM id parity and, on
a miss, creates a bare node and logs — it does not recover. Any non-deterministic
server output (dates, locale, timezone, auth-dependent content) will corrupt the
hydrated tree. Add per-subtree render-and-replace on mismatch, a
`suppressHydration`-style escape hatch for intentionally dynamic nodes, and
guidance for rendering deterministic markup. **M–L**

---

## P2 — Feature completeness for real UIs

**2.1 SVG / namespaced elements.** No `createElementNS` support — that alone
rules out icons and charts, i.e. most real UIs. Add an SVG element set and
namespace-aware creation in the renderer + bridge. **M**

**2.2 Async data & error boundaries.** Real apps load data and fail. Provide an
idiomatic async primitive (a "resource"/async signal with loading/error
states) and error boundaries (recover + fallback UI) so a panic in one subtree
doesn't blank the page. **L**

**2.3 Forms, inputs, focus.** Audit and complete controlled-input coverage
(value/checked/select/textarea), form submission ergonomics, and focus/
selection preservation across keyed reorders and scope re-renders. **M**

**2.4 Refs / DOM escape hatch & portals.** A way to get at a real DOM node
(measure, focus, integrate a non-goowee widget) and render into a different
container (modals/tooltips). **M**

---

## P3 — Performance & footprint (adoption-deciding, not correctness)

**3.1 Bundle size.** ~3.7 MB raw / ~1.0 MB gzip today. Evaluate TinyGo
(the reflect-free core helps), document gzip/brotli serving, and explore
route-level code splitting/lazy loading. **L**

**3.2 Boot latency.** WASM boot is the cost SSR-first is meant to hide (now that
hydration works). Measure TTI, document streaming instantiation, and consider a
"interactive-when-ready" story for large apps. **M**

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
**5.3 Router & a11y.** Nested routes, route guards, lazy routes; accessibility
helpers (focus management, ARIA conventions). **M**

---

## Definition of 1.0

1.0 should mean: a developer can `go get` a versioned release, follow the docs
to build a real SSR app with data loading, forms, SVG, and error handling;
signals are safe to update from timers/fetches; hydration survives realistic
content; bundle size and boot are documented and acceptable; and the public API
is stable with a deprecation policy. Everything through **P2** is the bar;
**P3** makes it competitive; **P4–P5** make it pleasant and durable.
