# Architecture Decision Records

These record decisions that are **settled** — especially ones where a
reasonable alternative exists and isn't wrong. The point is to stop future
maintainers (and future AI sessions) from flip-flopping on choices that were
made deliberately.

**How to use this file.** Treat Accepted ADRs as the default. Don't re-open one
without *new* information (a new requirement, a measured problem, a changed
constraint) — and when you do, add a new ADR that supersedes it rather than
editing history. Record the "why," not just the "what."

Status values: **Accepted**, **Superseded by ADR-N**, **Proposed**.

---

## ADR-001: Solid-style run-once components, not React re-render semantics

**Status:** Accepted (2026-07-05)

**Context.** The original plan described React semantics (components re-run on
update; hooks resolve state by call-order slots). The implementation never did
that — `UseState` always created a fresh signal — and the docs misled
contributors.

**Decision.** Commit to the Solid model: a component's function is a **setup
function that runs exactly once on mount**. State lives in signals captured by
closures; updates happen at finer grain (signal bindings, scopes), not by
re-running the component.

**Alternatives.** True React-style persistent hook-slot tree keyed by
(path, call-index). Rejected: it requires a persistent frame tree and slot
reconciliation, and buys re-render ergonomics we've engineered around — at the
cost of the "no VDOM, fine-grained updates" premise.

**Consequences.** No hook-ordering rules (call hooks conditionally, in loops,
anywhere). Props/params that must change over a component's life are passed as
**signals**, not plain values (see ADR-012). `ComponentFrame` is an ownership/
disposal scope, not a slot store. A matched component is *preserved* across
scope re-renders (state + effects survive).

---

## ADR-002: Manual dependency declaration, not automatic tracking

**Status:** Accepted (2026-07-05)

**Context.** Fine-grained reactivity needs to know which signals a reactive
region depends on.

**Decision.** Dependencies are declared explicitly — `UseScope(fn, deps...)`,
`Watch(deps, fn)`, `Computed(deps, fn)`, and the `h.Show`/`h.For`/`h.Switch`
helpers take their deps.

**Alternatives.** Solid-style automatic tracking (a signal read during a
reactive computation auto-subscribes). Rejected for now: in Go it needs a
global "current computation" register and disciplined read interception, and
it hides the dependency graph. Manual deps are explicit and simple.

**Consequences.** More verbose; forgetting a dep is a silent bug. Acceptable
trade for simplicity and no magic. Could be revisited if a safe auto-tracking
design emerges (would be a new ADR).

---

## ADR-003: Typed Go DSL (`h` package) is the authoring API; templates are deferred

**Status:** Accepted (2026-07-05)

**Context.** Go has no JSX/macros. Authoring options are a pure-Go DSL or a
compiled template format.

**Decision.** The `h` package (typed element/attr/event/binding constructors,
`Text`/`TextS`/`Textf`, control flow) is *the* authoring API. A `.gwx` template
compiler is deferred and, if ever built, compiles **down to `h` calls** — so
`h` is never throwaway.

**Alternatives.** Build the template compiler first. Rejected: a template
format without editor tooling (LSP, formatter) is negative value (the Vugu
lesson); and it would still need `h` underneath.

**Consequences.** Everything is gofmt-able, LSP-native, zero build steps.
Templates are gated on real external demand *and* first-class tooling.

---

## ADR-004: Separate static vs reactive helpers with an `-S` suffix

**Status:** Accepted (2026-07-05)

**Context.** A prop can be a static value or a signal binding.

**Decision.** Static and reactive variants are distinct functions:
`Class("x")` vs `ClassS(sig)`, `Value` vs `ValueS`, etc.

**Alternatives.** Unify with `any` and type-switch at runtime. Rejected: loses
compile-time safety and reintroduces the stringly/`any`-typed prop bag the
typed DSL exists to eliminate. Generics can't help — Go generic type sets can't
union interface types (`string | SignalAccessor` is illegal).

**Consequences.** A few more names to learn, but each is type-checked and
autocompletes. Consistent and predictable.

---

## ADR-005: Thin JS bridge — no application logic in JavaScript

**Status:** Accepted (2026-07-05)

**Context.** A Go-WASM UI framework needs *some* JS glue.

**Decision.** `runtime/goowee.js` does only: apply a mutation batch, forward
delegated DOM events to Go, and claim server-rendered nodes for hydration. All
logic lives in Go, where it's testable off-browser.

**Alternatives.** A richer JS runtime (diffing, routing helpers, etc.).
Rejected: it splits the source of truth, is hard to test, and drifts from Go.

**Consequences.** The bridge is tiny and stable. New capabilities are added
Go-side (e.g. dynamic event registration announced to JS), not by growing the
JS.

---

## ADR-006: Batched rendering via a mutation queue + dirty-scope scheduler

**Status:** Accepted (2026-07-05)

**Context.** Naively, each signal `Set` re-renders its scope and writes the DOM
synchronously — N writes per frame → N renders.

**Decision.** Signal writes mark their scope dirty on the scheduler. A flush
(driven by an on-demand animation frame) re-renders dirty scopes once each,
parent-before-child, then emits one coalesced mutation batch (last-write-wins
per node+key). `Flush()` is the single place batched work happens.

**Alternatives.** Synchronous DOM updates (simpler, but O(writes) renders and
uncoalesced intermediate states); microtask batching (finer but less aligned to
frames). Rejected in favor of rAF batching to match the paint cadence.

**Consequences.** Handlers return fast (render work is deferred). Tests call
`Flush()` to observe results. The rAF loop is scheduled only when work exists.

---

## ADR-007: Per-render `RenderContext`; effects never run on the server

**Status:** Accepted (2026-07-05)

**Context.** The frame stack was a package global; SSR renders concurrently per
request, and executing effects during SSR leaks goroutines/subscriptions.

**Decision.** The frame stack lives on a `RenderContext{Env, stack}`. Server
renders run under a fresh `EnvServer` context (serialized), so requests can't
corrupt each other and a panic can't poison the next render. `UseEffect`/
`OnMount`/`Watch` are no-ops under `EnvServer`.

**Alternatives.** Goroutine-local storage (needs a goid hack; fragile) or
threading a context param through every hook (breaks hook ergonomics).
Rejected. Server renders are serialized for now (fast, string-only); true
parallel SSR can come later without changing the API.

**Consequences.** SSR is request-safe. Client uses the default `EnvClient`
context (single-threaded). Component setup must be side-effect-free (setup
only); resources go in `OnMount`.

---

## ADR-008: Hydration by node-id parity + claim-based reuse; two walkers guarded by a golden test

**Status:** Accepted (2026-07-05)

**Context.** SSR and the client must agree on which DOM node is which to reuse
server-rendered markup.

**Decision.** The SSR and DOM walkers assign **identical node ids** for the same
tree; this parity is enforced by a golden test (`TestSSRDOMIDParity`) across
example pages. On the client, an SSR page is hydrated by *claiming* existing
nodes (`MutHydrate`) and skipping the create/attr/append mutations the server
already produced, while still wiring handlers/bindings.

**Alternatives.** (a) A shared SSR/DOM walker so parity holds *by construction*
— the right long-term design, deferred as a large refactor (roadmap P4.1).
(b) Structural/positional hydration that matches by DOM position instead of id
— avoids parity but needs coordinated Go↔JS DOM walking. We chose id parity +
claim because it's pragmatic and the renderers were already id-based.

**Consequences.** Parity is a *maintained invariant* — keep the golden test
green when touching either renderer. A parity miss falls back to creating a bare
node (logged), not full recovery; robust mismatch handling is roadmap P1.2.

---

## ADR-009: Text nodes get `<!--g{id}-->` hydration markers

**Status:** Accepted (2026-07-05)

**Context.** Text nodes can't carry attributes, so the client couldn't address
server-rendered text to reuse it (leading to duplicated text). Adjacent text
nodes also merge on HTML parse, breaking node correspondence.

**Decision.** SSR emits a `<!--g{id}-->` comment immediately before each text
node. The client claims the marked text node and removes the comment.

**Alternatives.** Walk `childNodes` positionally to match text (no markers) —
more complex and fragile across whitespace/merging. Rejected.

**Consequences.** A few extra comment bytes in SSR output, stripped at
hydration. The marker doubles as a separator that prevents text-node merging.

---

## ADR-010: Signals are single-threaded (no locking)

**Status:** Accepted (2026-07-05). The cross-goroutine gap noted below is
addressed by **ADR-015** (`core.Schedule`).

**Context.** Go WASM runs on a single OS thread; goroutines are cooperatively
scheduled.

**Decision.** Signal read/write and notification are not mutex-protected. The
contract is that signals are touched on the render loop.

**Alternatives.** Mutex/atomic-protected signals. Rejected as unnecessary
overhead and complexity given single-threaded WASM.

**Consequences.** Correct today. Mutating a signal from a goroutine (timer,
fetch) concurrently with a render is unsafe and unguarded — a real footgun,
now resolved by the `core.Schedule` path in ADR-015. Revisit locking only if
Go WASM gains real threads.

---

## ADR-011: Signal equality — interface equality, panic-recover for uncomparable types, `WithEquals` opt-in

**Status:** Accepted (2026-07-05)

**Context.** `Set` should skip notifying when the value is unchanged, but Go
values include uncomparable types (slices, maps, funcs) that panic on `==`.

**Decision.** Default equality is `any(a) == any(b)`, wrapped in
`recover()` → treated as "not equal" (so slice/map/func signals always notify,
which is correct for reference types). `WithEquals` opts into custom equality
(e.g. by id, or value-level for pointer signals).

**Alternatives.** Constrain signals to `comparable` (excludes the common
`Signal[[]T]`); use `reflect.DeepEqual` (pulls in reflect, hurts size/TinyGo).
Rejected.

**Consequences.** Reference-typed signals notify on every `Set` (via the
recover path) unless `WithEquals` is provided — acceptable and documented.
Keeps `reflect` out of the core.

---

## ADR-012: Router — map-based routes, deterministic most-specific matching, reactive params

**Status:** Accepted (2026-07-05)

**Context.** Routes need deterministic matching (Go map iteration is random) and
URL params must work with run-once component preservation (ADR-001).

**Decision.** Keep the `map[pattern]handler` API. Compile non-exact patterns
(`:param`, `/*`) once and match most-specific-first (exact > more literals >
non-wildcard). Params are exposed reactively: `r.ParamSignal(name)` is a derived
signal a component binds to, so a preserved component updates in place when only
the param changes; `r.Param(name)` is a snapshot for handlers.

**Alternatives.** An ordered `[]Route` slice (more flexible, breaks the map
API); passing params as handler arguments (a preserved component wouldn't see
new values — conflicts with ADR-001). Rejected.

**Consequences.** Overlapping routes resolve deterministically. Param-driven
views must *bind* `ParamSignal` (not read `Param` once at setup) to update.
Browser history is wired via `BindHistory()` behind a build tag (stub off-browser).

---

## ADR-013: Keyed reconciliation is correct-but-naive (no LIS yet)

**Status:** Accepted (2026-07-05)

**Context.** Reordering a keyed list needs to move DOM nodes.

**Decision.** Match by key, then re-insert every child in reverse order
(`insertBefore`), which is always correct (moves attached nodes, inserts new
ones). Keys work for elements and components alike.

**Alternatives.** Longest-increasing-subsequence to emit the *minimum* moves.
Deferred (roadmap P3.3): correctness first, optimality later.

**Consequences.** Large list reorders do O(n) DOM moves per update. Fine for
modest lists; LIS is the optimization when it matters.

---

## ADR-014: Development workflow — small PRs, squash-merge, verify under `-race` + E2E

**Status:** Accepted (2026-07-05)

**Context.** The framework was hardened over many focused changes.

**Decision.** Each change is a small branch → PR → squash-merge to `main`, kept
green in CI (vet, `go test -race`, native/SSR/WASM builds, WASM size budget,
benchmarks). Browser-facing changes are validated with the headless E2E
(`make e2e`). Merge without `--delete-branch`, then sync local `main` and delete
the branch (a `--delete-branch` merge once checked out a stale local `main`).

**Alternatives.** Long-lived feature branches / direct commits to `main`.
Rejected: harder to review and to keep bisectable.

**Consequences.** Clean history, each PR independently verified. `main` stays
releasable.

---

## ADR-015: Off-loop state updates go through `core.Schedule`

**Status:** Accepted (2026-07-06)

**Context.** Signals aren't thread-safe (ADR-010). Code that runs *off* the
render loop — a timer goroutine, a network/fetch callback, a channel receiver —
must not call `Set` directly, or it races the renderer (the mutation queue,
subscriber lists, and dirty set are unguarded).

**Decision.** Provide `core.Schedule(fn func())`: it queues `fn` (via the
scheduler's mutex-guarded `Post`) to run on the render loop at the next frame,
where it may `Get`/`Set` signals safely. The client registers its scheduler
once at startup via `bridge.Init` → `core.SetActiveScheduler`; `Schedule` is a
no-op when none is registered (SSR, tests). The rule: **touch signals only on
the render loop; from a goroutine, wrap the update in `core.Schedule`.** The
stopwatch example's ticker is the canonical use.

**Alternatives.** (a) Make `Set` itself defer when off-loop — but `Set` can't
tell where it's called from and is used during renders. (b) A goroutine-local
"am I on the loop" flag — fragile. (c) Full mutex-locked signals — rejected in
ADR-010. (d) Pass the scheduler explicitly to every effect — breaks the
run-once ergonomics. A single client-side "active scheduler" + `Schedule` is
the pragmatic fit; it doesn't reintroduce the SSR global-state problem
(ADR-007) because SSR has no scheduler and never registers one.

**Consequences.** Off-loop updates are batched onto the next frame (a tick of
extra latency, which is fine). Forgetting `Schedule` and calling `Set` from a
goroutine is still possible — it's a documented rule, not enforced by the type
system. Only one active scheduler per client process (the norm).

---

## ADR-016: Hydration trusts SSR/DOM parity; mismatches are opt-in (`h.Dynamic`), not auto-recovered

**Status:** Accepted (2026-07-06)

**Context.** Hydration claims server-rendered nodes by id parity (ADR-008) and
skips the create/attr/text mutations the SSR already applied (the boot
optimization). That is only correct when the server and client render the
**same tree with the same values**. Non-deterministic output — `time.Now`,
locale, timezone, per-request/auth content — makes the client silently display
the stale server value; structural divergence corrupts the tree.

**Decision.** The hydration contract is: **render deterministic markup.** For
the two failure modes:

- **Value differences** (same structure, different text/attrs): opt in with
  `h.Dynamic()` on the subtree. Hydration still *claims* those nodes (no
  re-creation, no duplication) but *re-applies* their attributes/properties/
  text so the client value wins. This is opt-in so deterministic content keeps
  the boot optimization (most nodes emit one claim and nothing else).
- **Structural divergence** (wrong tag / missing node): the JS side logs a
  clear `console.error` naming the node and best-effort creates a bare node so
  the app keeps running. It is *not* automatically repaired.

**Alternatives.** (a) Re-apply every node's values on hydration (robust but
taxes every deterministic node — the common case — for a rare one). (b)
Automatic per-subtree render-and-replace on any mismatch: the client would have
to detect the mismatch (only JS sees the DOM) and re-render the offending
subtree in normal mode — needing either a JS→Go round-trip or giving the
renderer live DOM access behind a build tag. Both are sizable; deferred. The
opt-in `Dynamic` hatch + loud logging covers the practical cases now.

**Consequences.** Non-deterministic *values* have a clean fix (`Dynamic`).
Non-deterministic *structure* degrades loudly rather than silently, and is
documented as a bug to fix with deterministic markup. Full automatic structural
recovery remains a future item (roadmap). The common client-only-value pattern
— render a placeholder on the server, set a signal in `OnMount` — also works
without `Dynamic`, since `OnMount` runs only on the client after hydration.

---

## ADR-017: Refs are imperative command handles; portals are client-side and rebuild

**Status:** Accepted (2026-07-06)

**Context.** Real UIs need to reach the DOM imperatively (focus a field, scroll
into view) and render outside the current subtree (modals, tooltips). Go never
holds the DOM node — it lives in the JS `nodeMap` — so Go can't call methods on
it directly.

**Decision.**
- **Refs** (`h.Ref` / `h.RefTo`) carry the node id, set at render. `ref.Focus()`,
  `Blur()`, `Click()`, `ScrollIntoView()` enqueue a `MutInvoke{id, method}`
  through the active scheduler; the bridge calls `node[method]()`. It's a
  one-way command channel — good for post-render actions (handlers, effects),
  applied on the next frame. **Reads (measuring) are not supported**: they'd
  need a value channel back to Go, deferred until there's a real use.
- **Portals** (`h.Portal(target, …)`) render children into a container matched
  by CSS selector via `MutPortalAppend`. They are **client-side only** — SSR
  doesn't render them, and their content is rendered *fresh* (created, never
  claimed) even during hydration, so it never trips the hydration-mismatch path
  (ADR-016). Because the target is a selector, not a node the diff can key
  against, portal children are **torn down and rebuilt on every re-render**;
  hold portal state in signals outside the portal.

**Alternatives.** For refs: exposing the raw `js.Value` to Go — impossible in
native/SSR builds and couples the renderer to `syscall/js`; a synchronous
`ref.Measure()` — needs a blocking round-trip. For portals: resolving the
target to a node id once and reconciling its children — worth doing later, but
the selector-based rebuild is simple and correct for v0.

**Consequences.** Refs cover the common imperative needs (focus/scroll/click)
without a DOM dependency in the renderer; measuring is a known gap. Portals work
for modals/tooltips but lose child state across re-renders — a documented
limitation, revisitable with id-keyed portal reconciliation.
