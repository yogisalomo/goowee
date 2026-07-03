# Competitor Research: Go WASM UI Frameworks

*Researched 2026-07-03. Statuses verified against project repos/sites at that date; re-verify before quoting externally.*

Goowee is not the first attempt at a Go UI framework for the browser. Three predecessors matter: **Vecty**, **Vugu**, and **go-app**. None of them died dramatically — the instructive failure mode is that (except go-app, partially) they stalled in **permanent experimental status**: years of development without ever becoming something a team could bet on. This document records what each did, what went wrong, and what goowee should do differently.

---

## Snapshot

| | Vecty | Vugu | go-app |
|---|---|---|---|
| Model ported | React (VDOM, struct components) | Vue (SFC templates + codegen) | React-ish (VDOM, PWA-centric) |
| Update mechanism | Virtual DOM diff | Tree sync/diff from codegen'd templates | Virtual DOM diff, component-level `Update()` |
| UI authoring | Pure-Go function/struct API | `.vugu` HTML template files → generated Go | Pure-Go chained builder API |
| SSR / prerendering | None | Planned, never landed as a pillar | Basic prerendering, coarse hydration |
| Reached 1.0? | No (7+ years) | No (7+ years) | Yes-ish (major-version churn instead) |
| Status (2026-07) | Self-described experimental; contributor-spare-time pace | Self-described experimental; slow but ongoing (mage build revamp 2024) | Most active of the three; maintained, niche |

---

## Vecty — the React port

**What it is:** `github.com/hexops/vecty`. Components are structs embedding `vecty.Core` and implementing `Render() ComponentOrHTML`. Full virtual DOM diffed in Go, originally targeting GopherJS, later WASM.

**What went wrong:**

1. **VDOM in the most expensive possible environment.** Every render produces a tree that must be diffed and patched across the `syscall/js` boundary — the exact place where Go WASM pays its highest per-call tax. The architecture imported React's costs without React's ecosystem to justify them.
2. **Paid Go's verbosity without buying type safety.** The API leaned on union-style interfaces (`ComponentOrHTML`, `MarkupOrChild`, `MarkupList`) — effectively `any` with extra steps. Users wrote more code than React *and* got weaker compile-time guarantees.
3. **No SSR story at all.** With multi-second WASM cold boots on slow connections, no server-rendered first paint means no public-facing use cases, which caps the audience at internal tools — an audience Vecty never explicitly courted.
4. **Perpetual pre-1.0.** After 7+ years the README still describes an experimental work-in-progress advancing "slowly and steadily as contributors have time." Nobody builds a product on that sentence.

**Lessons for goowee:** don't port React's runtime model (goowee's signal architecture already avoids this); treat SSR + hydration as a pillar, not a feature; and ship a stability contract — a small, frozen 1.0 API beats a large experimental one.

---

## Vugu — the Vue port

**What it is:** `github.com/vugu/vugu` / [vugu.org](https://www.vugu.org/). Single-file components (`.vugu` files: HTML template + embedded Go), compiled to Go by `vugugen`, synced to the DOM through a diffing runtime.

**What went wrong:**

1. **Bet on the template compiler before the runtime was solid.** The `.vugu` format was the headline feature from day one, but the reactivity/update model underneath stayed experimental. Sugar on top of unstable semantics meant both layers churned.
2. **Shipped a file format without tooling.** No LSP, no formatter, no real syntax highlighting for `.vugu` files. Editing them was worse than writing plain Go and worse than writing real Vue — so the DSL, meant to be the draw, became the adoption barrier. The build pipeline (codegen step, magic comments, later a mage-based build) added friction on top.
3. **Perpetual pre-1.0**, same as Vecty: still self-described as experimental after 7+ years.

**The counter-example that proves the lesson:** [templ](https://templ.guide) later succeeded with essentially Vugu's idea (HTML-ish files compiled to Go) for server-side HTML — because it shipped an LSP, a formatter, and editor extensions as first-class deliverables alongside the compiler. **A template file format is a tooling product, not a parser project.**

**Lessons for goowee:** nail the pure-Go API first — it's the compilation target and the fallback; only add a template format later, with a tooling budget (LSP + formatter + editor extension) committed up front. See `docs/plans/ergonomics-improvements.md` Phase 3.

---

## go-app — the PWA framework

**What it is:** `github.com/maxence-charriere/go-app`. Declarative chained builder API (`app.Div().Class("x").Body(...)`), VDOM diffing, strong progressive-web-app framing (service worker, installability, offline). The most successful and most alive of the three.

**What it got right (worth copying):**

- **The builder API works.** Typed, chainable, autocomplete-friendly UI code in pure Go, with no map-based props and no codegen. go-app is the existence proof that a Go HTML DSL can feel acceptable without angle brackets.
- It shipped, versioned, and kept a real user base — discipline the other two lacked.

**What kept it niche:**

1. **Still a VDOM.** Component-level `Update()` re-renders and full-tree diffing in WASM — coarse-grained reactivity with the same boundary costs as Vecty, just better packaged.
2. **PWA-first identity.** Framing the project around installable PWAs narrowed the perceived use case; "general web framework" adoption never followed.
3. **Breaking churn across major versions** (v6→v7→v8→…) repeatedly invalidated user code and third-party content.
4. **Bus factor of one.** Effectively a single-maintainer project; pace and direction follow one person's availability.

**Lessons for goowee:** adopt the typed-builder ergonomics (this is the direct ancestor of the plan in `docs/plans/ergonomics-improvements.md`); pair them with fine-grained signal updates go-app never had; don't narrow the framework's identity to one deployment style; once public, treat breaking changes as a cost to be budgeted, not a habit.

---

## Cross-cutting lessons → goowee actions

| # | Lesson (all three) | Goowee action |
|---|---|---|
| 1 | VDOM diffing is the wrong center of gravity in Go WASM; the JS boundary is the scarcest resource | Already avoided: fine-grained signals + batched mutation queue. Protect it — keep the subscription/ownership model sound (review P0) so the advantage is real, not just claimed |
| 2 | No/weak SSR+hydration caps you at internal tools | Make hydration the flagship feature, not the deferred one (review §6) |
| 3 | Permanent experimental status kills adoption more surely than any bug | Keep the core small enough for one maintainer; define a 1.0 scope and freeze it; per-phase "done" criteria instead of open-ended feature lists |
| 4 | None showcased Go's unfair advantage — the same types and logic on server and client | Build the flagship demo around shared server/client code (shared validation, typed server calls), not a counter |
| 5 | File-format DSLs without editor tooling are negative-value (Vugu) vs. transformative with tooling (templ) | Pure-Go DSL now; template compiler only later, gated on traction and a committed tooling budget |
| 6 | Builder-style typed APIs are proven viable in Go (go-app) | Adopt and improve: item-based DSL with signal-aware helpers — see ergonomics plan |

---

## Sources

- [hexops/vecty — GitHub](https://github.com/hexops/vecty)
- [vugu/vugu — GitHub](https://github.com/vugu/vugu)
- [vugu.org](https://www.vugu.org/)
- [Go Wiki: WebAssembly](https://go.dev/wiki/WebAssembly)
- go-app: `github.com/maxence-charriere/go-app` (assessment from training knowledge; status not re-verified at research date)
