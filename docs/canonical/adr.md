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

**Status:** Accepted (2026-07-05) — **with a known gap; see roadmap P1.1**

**Context.** Go WASM runs on a single OS thread; goroutines are cooperatively
scheduled.

**Decision.** Signal read/write and notification are not mutex-protected. The
contract is that signals are touched on the render loop.

**Alternatives.** Mutex/atomic-protected signals. Rejected as unnecessary
overhead and complexity given single-threaded WASM.

**Consequences.** Correct today. But mutating a signal from a goroutine (timer,
fetch) concurrently with a render is unsafe and unguarded — a real footgun.
Roadmap P1.1 will add a safe cross-goroutine update path (route through the
scheduler) and document the rule. Revisit locking only if Go WASM gains real
threads.

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
