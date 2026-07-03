# Goowee Architecture & Plan Review

*Reviewer: Claude (Fable 5) — 2026-07-02*
*Scope: `docs/plan-v1.md`, `docs/component_tree.md`, and the full implementation as of commit `b3562c8`.*

This review compares what the plan documents claim against what the code actually does, and makes recommendations for both the architecture and the plan itself. Findings are ordered by severity. File references point at the current source.

---

## Executive Summary

1. **The framework has an identity conflict: the docs describe React semantics, the implementation has Solid semantics.** Hook state does not survive re-renders — and mostly doesn't need to, because components run once and closures capture signals. Recommendation: commit explicitly to the "components run once" model (SolidJS-style), delete the misleading hook-slot narrative, and redesign `UseEffect` around mount/cleanup lifecycle instead of render-time execution. (§1)
2. **SSR is not safe for concurrent requests.** The render stack is a package-level global, and effects (including user goroutines) execute during server rendering. This will corrupt state and leak goroutines under real traffic today. (§3)
3. **Subscription lifecycle leaks by design.** `BindingRegistry.Unbind` never unsubscribes from signals, and `Signal.Subscribe`'s reflect-based unsubscribe can remove the *wrong* subscriber. Long-lived signals accumulate ghost subscribers as the UI churns. (§2)
4. **Renders are synchronous and unbatched; only DOM mutations are batched.** N signal writes per frame trigger N full scope re-renders and diffs. The scheduler should batch *renders*, not just their output. (§4)
5. **Several concrete diff and SSR bugs** — a typed-nil panic path in `diffNode`, removed props never unset, stale event handlers on reused elements, unescaped attribute values (XSS), and a broken hydration-path generator. (§5, §6)

None of this diminishes what's here: the mutation-queue + thin-bridge design is sound, the package layout is clean, the diff reuses node IDs correctly in the happy path, and the test discipline (everything runs under plain `go test`) is genuinely good. The issues below are the gap between "demo works" and "framework holds up."

---

## 1. The Core Architectural Question: React Semantics vs Solid Semantics

### What the docs claim

`plan-v1.md` §2 says:

> On re-render, the component path (e.g., `root/counter/0`) resolves to the same `ComponentNode`, so hooks read from the same slots. This is the same model React uses.

### What the code does

`hooks/use_state.go:5-13` never reads a slot. It creates a fresh signal and appends:

```go
func UseState[T any](initial T) (*core.Signal[T], func(T)) {
    sig := core.NewSignal(initial)
    if frame := core.CurrentComponent(); frame != nil {
        frame.Hooks = append(frame.Hooks, sig)   // always append, never resolve
    }
    ...
}
```

And `component_tree.md` (Key Properties) admits it: *"Frame-per-render — each render pass creates new `ComponentFrame` instances."* There is no path-based resolution anywhere in the codebase. If a `ComponentNode` re-renders, every `UseState` returns a brand-new signal initialized to `initial` — **state is lost on every re-render**.

The demo works anyway, and the reason is the important insight: **components effectively run once per mount**. Re-rendering happens only inside `UseScope` closures, which capture the signals from the enclosing component body. That is exactly SolidJS's model, not React's:

- React: component function re-runs on every update; hooks need positional slots to persist state.
- Solid: component function runs once; reactivity lives in fine-grained subscriptions; there are no hook rules because there is no re-execution.

The current design is *accidentally* Solid — and Solid's model is genuinely the better fit for this framework (no VDOM, fine-grained signal bindings, manual deps). But because the docs promise React semantics, the code carries React-shaped machinery that doesn't do anything:

- `ComponentFrame.Hooks` slot storage is written but never read back (`hooks/use_state.go:8`).
- The "no conditionals before hooks" rule (`plan-v1.md` §2 Rules) is unnecessary — call order doesn't matter when slots are never resolved by position.
- Gap 20.9 ("no stale frame cleanup on ComponentNode re-render") exists precisely because frames pretend to be persistent identities but are actually per-render throwaways.

### Consequences that bite today

`RunFrameCleanup` runs for **all** old frames on every scope re-render (`dom/renderer.go:209-211`), not just removed ones. Combined with frame-per-render, this means any `UseEffect` inside a component under a `UseScope` is torn down and re-executed on *every* dep change of the scope — regardless of the effect's own deps. Effects have no stable identity.

And because there is no mount/unmount lifecycle, the example app resorts to raw goroutines: `examples/counter/app/app.go:399-406` spawns an infinite `for { time.Sleep(...) }` loop in the component body. Navigate to the stopwatch page five times → five immortal goroutines all calling `setElapsed`. (This also runs on the SSR server — see §3.)

### Recommendation

**Commit to the Solid model explicitly.** Concretely:

1. Rename/reframe the docs: components are setup functions that run once per mount. State lives in signals captured by closures. Delete the React hook-slot narrative and the call-order rules.
2. Replace `UseState`'s vestigial slot-append with nothing — it only needs to register the signal with the current frame *if* you want devtools/hydration to enumerate it.
3. Redesign `UseEffect` into two primitives with clear timing:
   - `OnMount(fn func() (cleanup func()))` — runs once after the component's nodes are attached; cleanup on unmount. This is what the stopwatch needs.
   - `Watch(deps []SignalAccessor, fn func())` — re-runs on dep change, cleanup before re-run. No render-time ambiguity.
4. Keep `ComponentFrame` but give it one honest job: an ownership scope for disposal (subscriptions, effects, bindings created during setup get registered on the frame; unmount disposes the frame). This resolves gaps 20.8 and 20.9 in one move — `ComponentNode` is the tree node, `ComponentFrame` is the disposal scope, and nothing pretends to be a hook-slot store.

If you instead want true React semantics (components re-run, state persists), the work is much larger: `UseState` must resolve slots by `(frame path, call index)` against a *persistent* frame tree keyed by position+component identity, and every diff must reconcile frames rather than recreate them. I don't recommend this path — it buys re-render ergonomics you've already engineered around, at the cost of the entire "no VDOM, fine-grained updates" premise.

---

## 2. Reactive Core: Subscription Lifecycle Is Leaky and Unsound

### 2.1 `Unbind` never unsubscribes (leak + ghost mutations)

`core/binding.go:26-37`: `Bind` subscribes to the signal but discards the unsubscribe function. `Unbind` only deletes the registry map entry:

```go
func (r *BindingRegistry) Bind(nodeID int, sig SignalAccessor, prop string) {
    r.bindings[nodeID] = append(...)
    sig.Subscribe(func() {                      // unsubscribe fn dropped
        r.scheduler.Enqueue(r.GetMutationsFor(nodeID)...)
    })
}
func (r *BindingRegistry) Unbind(nodeID int) {
    delete(r.bindings, nodeID)                  // signal still holds the closure
}
```

Every removed DOM node leaves a permanent subscriber on its signal. For long-lived signals (e.g. `router.Path`, app-level stores), subscribers grow without bound as the UI churns. The ghost closures fire on every future `Set` — they enqueue nothing (the map entry is gone) but they're still O(dead nodes) work per update, and they pin the registry in memory. Note the plan (§4) and `spike-divergences.md` D2 both *specify* that `Bind` returns/stores the unsubscribe — the implementation just doesn't do it.

**Fix:** store unsubs per nodeID (`unsubs map[int][]func()`) and call them in `Unbind`. `emitRemoveTree` (`dom/renderer.go:364-373`) already calls `Unbind`, so the fix is contained.

### 2.2 Reflect-based unsubscribe removes the wrong subscriber

`core/signal.go:25-36` identifies subscribers by `reflect.ValueOf(fn).Pointer()`. For closures, that returns the **code pointer**, which is identical for every closure created from the same source literal. Example: two `ScopeNode`s subscribing to the same signal both register `func() { r.reRenderScope(v) }` (`dom/renderer.go:159`) — same code pointer, different captured `v`. Unsubscribing scope B can remove scope A's subscription. Silent, order-dependent, brutal to debug.

**Fix:** token-based subscriptions —

```go
type Signal[T any] struct {
    value  T
    subs   map[int]func()
    nextID int
}
func (s *Signal[T]) Subscribe(fn func()) func() {
    s.nextID++
    id := s.nextID
    s.subs[id] = fn
    return func() { delete(s.subs, id) }
}
```

(Or keep a slice of `{id, fn}` pairs if notification order matters.) This also drops the `reflect` import, which helps if you ever evaluate TinyGo for binary size (§9).

### 2.3 Notification is not reentrancy-safe and has no equality skip

`core/signal.go:18-23`: `Set` iterates `s.subs` while subscribers may unsubscribe (shifting the backing array mid-range → skipped subscribers) or call `Set` again (unbounded recursion, no cycle guard). And `Set` always notifies even when the value is unchanged, so e.g. setting the same route path re-renders the whole route scope.

**Fix:** copy the subscriber list before iterating; add an equality skip (either constrain a `NewComparableSignal` variant on `comparable`, or accept an optional `Equals func(a, b T) bool`); consider a simple re-entrancy depth guard that defers nested notifications to the scheduler (which is where §4 goes anyway).

---

## 3. SSR Cannot Survive Production Traffic

### 3.1 Global render stack + concurrent handlers = data race

`core/component_tree.go:11` — `var renderStack []*ComponentFrame` is package-global mutable state. `cmd/ssr-server/main.go:26-33` renders inside `http.HandleFunc`, and `net/http` serves each request on its own goroutine. Two concurrent requests interleave pushes/pops on the same stack: corrupted paths, crossed hook frames, and a data race the race detector will flag on the first parallel benchmark. There's also no `defer`-based unwinding — a panic mid-render leaves the stack dirty for every subsequent request.

**Fix:** thread a render context instead of using globals. The renderers already own state (`nextID`, registries); the frame stack belongs there too:

```go
type RenderContext struct {
    stack []*ComponentFrame
}
// ComponentNode render goes through ctx.Push()/ctx.Pop()
```

This requires `ComponentNode.Render` to receive the context (`Render func(ctx *RenderContext) Node`) or — less invasive — a goroutine-local workaround. The explicit context is the right call; it also gives you a natural home for the environment flag in §3.2 and for hydration state later. Do this before any hydration work; it touches the same signatures.

### 3.2 Effects and user code run during SSR

The SSR renderer executes component functions (`ssr/renderer.go:126-135`), which means:

- `UseEffect` runs its body immediately at render time (`hooks/use_effect.go:19`) and subscribes to deps — on the server, per request, never cleaned up (`RunFrameCleanup` is never called server-side).
- User code like the stopwatch goroutine (`app.go:399`) leaks a goroutine per request to `/stopwatch`, each polling signals forever.

React deliberately does not run effects on the server, and this framework needs the same rule.

**Fix:** put an `Env` (client/server) on the render context; `OnMount`/`Watch` (§1) register but do not execute when `Env == Server`. Document that component bodies must be side-effect-free (setup only) — the goroutine-in-body pattern in the example app should be rewritten as `OnMount` and held up as the canonical timer example.

### 3.3 HTML generation bugs

- **Attribute values are not escaped** — `ssr/renderer.go:80`: `fmt.Fprintf(buf, ` %s="%s"`, key, actual)`. Any signal or prop value containing `"` breaks out of the attribute; with user-supplied data that is XSS. Text content is escaped (`escapeHTML`) but quotes aren't handled there either (fine for text, not for attrs). Prop *keys* are also emitted raw. Escape attribute values (`&`, `<`, `>`, `"`) and validate/whitelist keys.
- **Void elements get closing tags** — `<input ...></input>`, `<br></br>` (`ssr/renderer.go:97-105`). Browsers parse `</br>` as a *second* `<br>`, so the server DOM won't match the client's — a guaranteed hydration mismatch. Add a void-element set (`br`, `hr`, `img`, `input`, `meta`, `link`, ...) and self-close.
- **Static bool props are dropped in SSR but set in the DOM renderer** — `ssr/renderer.go:77-95` handles only `string` and `SignalAccessor`; `dom/renderer.go:68-73` also handles `bool`. `disabled: true` renders enabled on the server, disabled on the client. Unify the prop-serialization rules in one shared table (see §6).
- **Hydration path generation is broken** — `ssr/renderer.go:100` builds every child's path as `path+"/"+itoa(len(v.Children))`: the same segment for all siblings (it uses the child *count*, not the index). Combined with `ComponentNode` handling that pushes a frame but ignores `frame.Path` (gap 20.10, confirmed), `HydrationMeta.NodeMap` contents are currently meaningless. The `SlotRef.HookIndex` computation (`hookIdx+sigIdx` counting signal props per element) also has no relationship to actual hook indices. This whole metadata layer needs to be redesigned against the frame tree, not patched — see §6.

---

## 4. Scheduling: Batch Renders, Not Just Mutations

The plan (§5) promises "frame-batched updates," but only the *mutation queue* is frame-batched. The expensive work — scope re-render + diff — runs synchronously inside `Signal.Set` (`Set` → subscriber → `reRenderScope`, `dom/renderer.go:195-214`). Consequences:

- N `Set` calls in one frame → N full re-renders and diffs of the same scope. The VirtualList scroll handler (`html/html.go:88-92`) does exactly this: scroll events can fire faster than frames, and each one re-renders the list synchronously.
- Mutations from successive re-renders of the same scope stack up in the queue, including obsolete intermediate states. Nothing coalesces them.
- A `Set` from inside an event handler blocks the handler on render work.

**Fix — dirty-scope scheduling:**

1. Signal subscription for a scope marks it dirty in the scheduler: `sched.MarkDirty(scope)`.
2. The rAF flush (`bridge/wasm.go:23-34`) first re-renders all dirty scopes (deduplicated, parent-before-child so a parent re-render that unmounts a dirty child cancels it), *then* sends the mutation batch.
3. Add last-write-wins coalescing on `(NodeID, Key)` for `MutSetProperty`/`MutSetAttribute` while flushing.

Two smaller scheduler items:

- **The rAF loop spins forever** even when idle (`bridge/wasm.go:24-33`), waking the WASM module every 16ms. Schedule a frame only when work exists: the scheduler gets an `onWork func()` callback, set by the bridge, that calls `requestAnimationFrame` once and clears a `scheduled` flag on flush.
- **The initial-render contract is split**: first render *returns* mutations (`Render` → caller enqueues, `examples/counter/main.go:25-26`) while re-renders *enqueue* internally. Pick one (enqueue internally everywhere) — the split already caused the duplicated ScopeNode re-render logic between `renderNode` (`dom/renderer.go:129-150`) and `reRenderScope` (`:195-214`), which have drifted (only one wraps `Enqueue` in a `len > 0` check; both exist to do the same job).

---

## 5. Diff Correctness Bugs

All in `dom/renderer.go` unless noted.

1. **Typed-nil panic on node-type change.** `diffNode`'s type-mismatch branches pass the *failed type assertion result* to `renderNode`:

   ```go
   new, ok := newNode.(*core.ElementNode)
   if !ok || old.Tag != new.Tag {
       r.emitRemoveTree(old, muts)
       return r.renderNode(new, muts)   // new is (*ElementNode)(nil) when !ok
   }
   ```

   When `!ok`, `new` is a typed nil; `renderNode` enters `case *core.ElementNode` with `v == nil` and panics at `v.ID = id`. A scope that swaps a `<p>` for a text node crashes the app. Same pattern at the `TextNode` (`:299-304`) and `FragmentNode` (`:315-320`) cases. Fix: pass `newNode`, not `new`.

2. **Removed props are never unset.** The prop loop iterates only `new.Props` (`:237`). A prop present in old but absent in new (e.g. `class` dropped) stays in the DOM forever. Iterate old props too and emit removals (`MutRemoveAttribute` is missing from the mutation set — add it).

3. **Event handlers and signal props are ignored for reused elements.** The diff handles only `string` and `bool` values (`:242`, `:256`). When an element is reused (`new.ID = old.ID`):
   - The new render's event handler closures are discarded; the node keeps the handler registered at first render, which captures *old* signals/variables. Under the current "components run once" reality this mostly coincides, but the moment a scope closure creates per-render closures over loop variables (todo lists do), handlers go stale.
   - New `SignalAccessor` props are never bound, and old bindings are never rebound. If a scope re-render produces a different signal instance for the same prop, updates silently stop.

   Fix: in the element-reuse path, re-register handlers (`Registry.RegisterHandler` overwrites — cheap) and reconcile bindings (unbind/rebind when the signal instance differs).

4. **`MutInsertBefore` index mismatch with text nodes.** The Go diff emits the child index counting *all* children (`:285-290`), but the JS bridge resolves it via `parent.children[idx]` (`runtime/goowee.js:122`), which contains **elements only** — text-node siblings shift the mapping and children get inserted at wrong positions. Fix: resolve the reference node by child *node ID* instead of index (emit `ChildID` + `RefID`), which is also cheaper and unambiguous.

5. **Reactive text diffing is value-blind.** The `TextNode` case (`:305-313`) compares only `string`↔`string`. Signal-valued text on either side is skipped entirely — a scope that switches a text node from static to signal-backed gets no binding and no update. (spike-divergences D5 flagged exactly this; the resolution didn't land.)

6. **Top-level fragments break mounting.** `renderNode`'s fragment case emits children without any parent attachment (`:114-118`), and the JS bridge's "append the first parentless node to `#root`" heuristic (`goowee.js:131-134`) attaches only the *first* root. A component returning a fragment at the top level loses all subsequent roots. Fix: mount into an explicit container ID passed to `Render` (emit `MutAppendChild{NodeID: rootContainerID, ...}` per root) and delete the JS heuristic — it's app logic living in the bridge, which the plan's own §7 forbids.

### Keyed reconciliation (plan §20.2 is marked "resolved" too early)

Position-based matching is fine until list items carry state or non-trivial subtrees. Remove the first todo and every subsequent item is "matched" against its neighbor: all props re-diffed, handlers stale (bug 3 above), and any per-item component state (once state persistence exists per §1) attributed to the wrong item. This is table stakes for the framework's stated ambitions. Add a `Key any` field to `ElementNode`/`ComponentNode`, and in the diff's child loop match keyed children by key (simple map + longest-increasing-subsequence for minimal moves, or even naive keyed matching first — correctness before optimality). I'd downgrade §20.2 from "resolved" to "positional matching implemented; keyed matching required before lists with identity."

---

## 6. Hydration: Make ID Parity a First-Class Invariant

Client hydration is the one deferred pillar, and the current trajectory makes it harder than it needs to be:

- Reuse-by-node-ID (`goowee.js:86-88`, `preexistingNodes`) requires that the SSR walker and the DOM walker assign **identical ID sequences** for the same tree. They are two independent hand-maintained walkers (`ssr/renderer.go`, `dom/renderer.go`) that already disagree (bool props, fragment handling, scope flattening). Every future change risks silent divergence.
- Text nodes get IDs in SSR metadata but no addressable marker in the HTML (attributes don't exist on text nodes), so the client can't find them. Today the WASM boot does a *full fresh render* over SSR pages — reused elements keep their SSR text children *and* receive newly created text nodes, so hydrated pages should visibly duplicate text (e.g. the nav separators from `html.Text(" | ")`). Worth verifying in a browser; if confirmed, it means SSR pages are currently broken after WASM boot, not just "not yet reactive."
- The hydration metadata (paths, SlotRefs) is unusable as generated (§3.3).

**Recommendations:**

1. **Implement the shared walker the plan already sketches (§8 "Renderer Abstraction," gap 20.3).** One tree traversal that assigns IDs and emits backend-agnostic events; DOM backend materializes mutations, SSR backend materializes HTML. ID parity then holds by construction. The plan defers this "until the backends diverge" — they have diverged; the trigger condition is met.
2. Until then, add a **golden parity test**: render every example page through both walkers and assert the ID-annotated tree shapes match. Cheap, and catches drift immediately.
3. For text nodes, either emit comment markers (`<!--t:42-->text`) or have hydration walk `childNodes` of matched parents positionally instead of addressing text by ID.
4. Simplify the metadata: you likely don't need `NodeMap` for every node — only interactive nodes (event handlers) and signal-bound nodes matter for reconnection. Smaller payload, less to get wrong.
5. Rewrite hydration boot as: walk SSR DOM claiming nodes → attach bindings/handlers → **no create/append mutations at all** for matched subtrees; fall back to render-and-replace per subtree on mismatch (the plan's lenient model, §9 — the design is right, it just needs the parity substrate first).

---

## 7. Events & the JS Bridge

The "no app logic in JS" principle (plan §7) is violated in two places, and both are symptoms of missing Go-side capability:

- **Anchor preventDefault hack** — `goowee.js:29-35` special-cases `<a>` clicks. The framework has no way for Go to declare "this handler consumes the default action." Add it to handler registration: `RegisterHandler(id, "click", handler, PreventDefault)` → the bridge ships a per-node/event flag set to JS once (or includes it in the `handleEvent` response). Then delete the hack.
- **Root-append heuristic** — `goowee.js:131-134`, see §5.6.

Other gaps:

- Only `click`, `input`, `submit`, `scroll` are delegated (`goowee.js:21-77`). `keydown`/`keyup`, `change`, `focus`/`blur` (non-bubbling — need capture like scroll), `pointermove` are all missing; any app needing them today silently gets nothing. Generate the delegation list from what Go actually registers: bridge tells JS "listen for X" on first registration of event type X.
- Event payload contract is implicit: JSON numbers arrive as `float64` in `EventData.Data`, and each handler type-asserts ad hoc (`html.go:89`). Define typed accessors (`ed.Value() string`, `ed.ScrollTop() float64`, `ed.Key() string`) so the schema lives in one place. The plan's open question 20.5 (binary protocol for high-frequency events) stays correctly out of scope — but coalescing scroll/pointermove to one event per frame in the bridge is cheap and pairs with §4.
- `NodeRegistry.Dispatch` should wrap handlers in `recover()` — plan §11 promises this ("Effect panics → recover, log via bridge") but nothing in the codebase calls `recover()` at all. One panicking handler currently kills the whole WASM instance.

---

## 8. Router

- **Nondeterministic route matching** — `router/router.go:49-53` falls back to iterating a Go map for prefix patterns; map order is randomized, so overlapping patterns (`/docs/*`, `/docs/api/*`) match differently run to run. Use an ordered `[]Route` slice, or sort patterns by specificity (longest prefix first).
- No URL params (`/todos/:id`) — fine to defer, but the plan should say so explicitly rather than listing the router as simply "✅ Done."
- Client history integration exists (`examples/counter/main.go:14-22`, nicely done with pushState/popstate) but lives in the example. Move it into `router` behind the same build tag pattern as `bridge`, so every app doesn't reimplement it.

---

## 9. Performance & Footprint (absent from the plan)

The plan has no size/performance section, and for a Go-WASM framework these are adoption-deciding constraints. Suggested additions:

- **Binary size budget.** Go WASM binaries start ~2MB and grow; measure `bin/main.wasm` in CI, track it, and set a budget. Note: dropping `reflect` (§2.2) and avoiding `fmt` in hot paths helps both size and TinyGo compatibility if you later evaluate it.
- **Boot latency.** SSR-first is the right architecture precisely because WASM boot is slow — but that makes hydration (§6) the critical path, not a polish item. Consider stating this dependency in the plan: *SSR without working hydration is a slower way to serve a client-rendered app.*
- **Mutation transport.** JSON-marshal per frame (`bridge/wasm.go:36-42`) allocates on every flush. Fine for MVP; note the upgrade path (reusable buffer → structured clone via `js.ValueOf` → shared ArrayBuffer) so it doesn't need re-litigating later.
- **Benchmarks.** Add `go test -bench` for: signal fan-out (1 signal → N bindings), scope re-render of an M-row list, diff of two K-node trees. These catch regressions from the §2/§4 rework.

---

## 10. Plan Document Improvements

Beyond tracking the fixes above, the plan itself needs maintenance:

1. **Accuracy debt.** Several sections describe intent as fact: §2's hook-slot resolution (not implemented), §4's "`Bind()` returns an unsubscribe function" (it doesn't), §20.2 "resolved" (positional only), §16's odd "✅/❌ DO NOT implement (but some are done)" list (confusing — split into "non-goals" vs "implemented beyond scope"). A plan that overstates the implementation actively misleads contributors; the per-section status tables (§9, §15) are the right pattern — extend them.
2. **Module identity.** `go.mod` says `goowee`, plan §7 says `github.com/yourorg/gowee`, repo directory says `goowee`. Pick the real module path (`github.com/<you>/goowee`) before anything external links against it.
3. **Missing sections worth adding:** concurrency model (what's allowed to touch signals from goroutines? Today: nothing safely — see the stopwatch; after §4, "only via `Set`, which defers to the scheduler" can be made true by routing cross-goroutine sets through a channel into the frame loop); error handling implementation status (§11 is a table of promises with zero `recover()` calls in the code); performance budgets (§9 above).
4. **Success criteria (§17) should become executable.** "Signal updates trigger minimal DOM mutations" is checkable: assert exact mutation lists in tests (the dom tests already do some of this — formalize it as the acceptance suite for §4's scheduler rework).
5. **Testing strategy (§13) lists browser integration tests that don't exist.** Either add a minimal `chromedp` smoke test (serve SSR page → click counter → assert DOM) or mark them deferred. The hydration work (§6) is untestable without one.

---

## 11. Suggested Roadmap (priority order)

**P0 — the core model must be sound before more features:**
1. Decide and document the Solid-style "components run once" model; replace `UseEffect` with `OnMount` + `Watch`; make `ComponentFrame` the disposal scope (§1).
2. Fix subscription lifecycle: token-based `Signal.Subscribe`, stored unsubs in `BindingRegistry`, reentrancy-safe notify, equality skip (§2).
3. Make SSR request-safe: render context instead of globals, no effects server-side (§3.1–3.2). Fix attr escaping (§3.3 first bullet) — it's a security bug.

**P1 — framework viability:**
4. Dirty-scope scheduling + mutation coalescing + on-demand rAF; unify the render/enqueue contract (§4).
5. Fix the five diff bugs; add keyed reconciliation (§5).
6. Shared walker (or at minimum the ID-parity golden test), void elements, SSR bool props; then build client hydration on top (§6).

**P2 — DX and polish:**
7. `Computed` (derived signals with manual deps — trivial given the primitives: subscribe to deps, recompute, notify) — the `textContent: func() string {...}()` pattern in the stopwatch (`app.go:412-417`) shows the need: that IIFE evaluates once and never updates.
8. Event system: preventDefault from Go, full event coverage, typed `EventData` accessors, handler `recover()` (§7).
9. Router: deterministic matching, params, built-in history integration (§8).
10. Performance CI: size budget, benchmarks (§9). Plan-document cleanup (§10).

---

## Closing Note

The strongest thing about this project is that its *instincts* are right: fine-grained signals over VDOM, a thin dumb bridge, SSR as a first-class target, everything testable off-browser. The weakest thing is that the documentation describes a React that the code never built — and the code is better off for not having built it. Naming the actual model (signals + run-once components + scoped re-render regions) and hardening the subscription/ownership story will do more for this framework than any new feature.
