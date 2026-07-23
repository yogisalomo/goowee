# Implementing `recover()` for TinyGo's WebAssembly target

*Authored 2026-07-23. Status: research / proposal — not yet started. Written from
goowee's perspective (a long-lived, in-browser WASM UI runtime) but scoped as an
upstream TinyGo contribution.*

## Why goowee cares

goowee relies on `recover()` for two load-bearing features:

- **`h.ErrorBoundary(fallback, child)`** — catches a render-time panic and shows
  a fallback instead of blanking the page.
- **Panic containment** — a panicking event handler or scope re-render is
  `recover()`ed so one failure doesn't kill the whole app (`internal/dom`
  `reRenderScope`, `VisitErrorBoundary`; `core.Recover`).

Under **standard Go** (`GOOS=js GOARCH=wasm`) both work. Under **TinyGo wasm**
they don't: TinyGo has never implemented `recover()` for wasm, so a `panic()`
becomes an `unreachable` trap that aborts the whole module. This is the single
blocker that made an otherwise-attractive TinyGo build (≈285 KB gzip vs ≈1.16 MB
for standard Go on our example — ~4×) unshippable for goowee. See
[tinygo-org/tinygo#2914](https://github.com/tinygo-org/tinygo/issues/2914).

This document is a concrete plan to close that gap upstream. **Recommendation:
contribute to TinyGo, do not fork** — the feature is on TinyGo's own roadmap and
the historical blocker (a wasm stack-unwinding primitive) is now resolved.

> Caveat on sources: this plan is grounded in TinyGo 0.41.1's *runtime* sources
> (`src/runtime/panic.go`, `defer.go`, `targets/wasm.json`, `asm_tinygowasm.S`),
> which ship with the toolchain, plus the public Wasm EH spec. The **compiler**
> package (`compiler/*.go`, which lowers `defer`/`panic`/`recover` and decides
> `supportsRecover()`) is compiled into the `tinygo` binary and was read from the
> upstream repo, not the local install — file/function names there should be
> re-verified against the current `main` before implementation.

---

## Background: how TinyGo recover works today, and why wasm was left out

TinyGo does **not** use LLVM landing pads for `recover()` on most targets. It
uses a **`setjmp`/`longjmp` model** with a linear-memory defer frame. From
`src/runtime/panic.go`:

```go
//export tinygo_longjmp
func tinygo_longjmp(frame *deferFrame)
func supportsRecover() bool        // compiler intrinsic
func trap()                        // compiler intrinsic → wasm `unreachable`

func panicOrGoexit(message interface{}, panicking panicState) {
    if panicStrategy() == tinygo.PanicStrategyTrap { trap() }
    if supportsRecover() && !interrupt.In() {
        frame := (*deferFrame)(task.Current().DeferFrame)
        if frame != nil {
            frame.PanicValue = message
            frame.Panicking = panicking
            tinygo_longjmp(frame)   // <-- non-local jump back to the deferring fn
        }
    }
    // no recover available → print and abort:
    printstring("panic: "); printitf(message); printnl(); abort()
}
```

- `setupDeferFrame` pushes a `deferFrame` (with a saved `JumpSP`) onto a
  per-task linked list at the entry of any function containing `defer`.
- `_recover()` reads the current `deferFrame`; `tinygo_longjmp` restores `JumpSP`
  and jumps back into the deferring frame, where `destroyDeferFrame` re-raises if
  still panicking.
- `tinygo_setjmp`/`tinygo_longjmp` are implemented **per-architecture in
  assembly** (`asm_<arch>.S`) by saving/restoring the stack pointer and
  registers.

**Why wasm is the exception.** WebAssembly's call stack is **not in linear
memory** — it is managed opaquely by the VM. You cannot save a stack pointer and
`longjmp` across wasm frames with assembly. Confirming this in the install:

- `src/runtime/asm_tinygowasm.S` is **10 lines** — only `__stack_pointer`
  helpers, **no `tinygo_setjmp`/`tinygo_longjmp`**.
- The compiler therefore makes `supportsRecover()` return **false** for wasm, so
  `panicOrGoexit` skips the longjmp branch and falls through to `abort()` — the
  `unreachable` trap we observe. `_recover()` short-circuits to `return nil`.

So today, on wasm: **`panic` = hard abort; `recover` = no-op.** Deferred
functions after a panic don't even run (matching TinyGo's documented behavior:
"on architectures where recover is not implemented, a panic will always exit the
program without running any deferred functions").

## What changed: the stack-unwinding primitive now exists in wasm

`recover()` needs non-local unwinding. Wasm historically had none — but as of
**WebAssembly 3.0 (2026)** the **Exception Handling** proposal is finalized, with
cross-browser support (all major engines) and mature toolchain support (LLVM,
Binaryen, Wasmtime). Two relevant capabilities fall out of that:

1. **Native wasm EH** — `throw`/`try_table`/`catch` instructions, plus an
   `exnref` value type (needs the `reference-types`/`exnref` feature). LLVM lowers
   C++/Go-style `invoke`/`landingpad` to these.
2. **Wasm SjLj** — LLVM's `-wasm-enable-sjlj` implements C `setjmp`/`longjmp`
   *on top of* wasm EH. This is the crucial one: it can back TinyGo's **existing**
   `tinygo_setjmp`/`tinygo_longjmp` model without changing the runtime's
   panic/defer design.

The blocker on #2914 ("waiting for the wasm EH proposal to mature") is resolved.
What remains is the TinyGo-side implementation.

Reference for the "blocker resolved" claim:
[**Wasm 3.0 Completed** — webassembly.org, 2025-09-17](https://webassembly.org/news/2025-09-17-wasm-3.0/)
(Andreas Rossberg), which lists structured exception handling among the
finalized features.

---

## Phase 0 spike results (local, 2026-07-23)

Run against a stock **TinyGo 0.41.1** install (LLVM 20.1.1, bundled Binaryen
`wasm-opt` v131) and **Node v25.7.0 / V8 14.1**. The compiler can't be rebuilt
here, so this is a *characterization* spike — it pins the exact failure and
proves the two non-compiler halves of EH are already present in TinyGo's own
toolchain and a current runtime. Reproducers in the commit history / `/tmp/spike`.

**1. Current failure — identical under `-scheduler=none` and `-scheduler=asyncify`:**
a `panic()` with a deferred `recover()` prints `panic: boom` and **traps
(`RuntimeError: unreachable`)**. The deferred function **never runs** and
`recover()` **never fires**. So the failure is not scheduler-specific — it is the
`supportsRecover()==false → abort()` path in `panicOrGoexit`.

**2. Toolchain readiness — present.** The Binaryen bundled *inside* TinyGo
(`wasm-opt` v131) advertises `--enable-exception-handling`, and round-trips a
`try_table`/`throw`/`catch` module. LLVM 20.1.1 (which TinyGo builds on) supports
wasm EH and wasm SjLj.

**3. Host readiness — present.** V8 (Node 25) both *validates* and *executes* EH
end-to-end: a hand-written module that `throw`s a tagged `i32(42)`, catches it
via `try_table`, and returns the payload — returns `42`. So the runtime half is
done in current engines (consistent with the cross-browser Wasm 3.0 support).

**Conclusion:** the primitive `recover()` needs, and the runtime to execute it,
**already exist and work in TinyGo's exact toolchain + a current engine**. The
sole missing piece is the **TinyGo compiler wiring** — flip `supportsRecover()`
for wasm and provide `tinygo_setjmp`/`tinygo_longjmp` (via wasm SjLj) — plus the
asyncify-coexistence work. That is the go/no-go story Phase 0 was meant to
establish, and it lands on **go, with asyncify as the remaining unknown.**

---

## Approaches

### Approach A (recommended): back the existing setjmp/longjmp model with wasm SjLj

Reuse everything in `panic.go`/`defer.go` and only provide the missing primitive
for wasm.

- Implement `tinygo_setjmp`/`tinygo_longjmp` for wasm via LLVM wasm SjLj
  (`-wasm-enable-sjlj`), which lowers `setjmp`/`longjmp` calls to wasm EH under
  the hood — the same intrinsics every other target already uses.
- Make `supportsRecover()` return **true** for wasm when EH is enabled.
- The `deferFrame` linked list, `_recover`, `destroyDeferFrame`,
  `panicOrGoexit` — **unchanged**.

**Why preferred:** smallest conceptual diff; reuses the audited defer/recover
runtime; keeps wasm consistent with native targets; least new surface area in the
compiler's defer lowering.

**Cost:** wasm SjLj still compiles down to wasm EH instructions, so it inherits
the same toolchain-feature and asyncify-interaction issues as Approach B (below).
The `JumpSP`-based frame may need reinterpretation since there's no linear-memory
SP to restore — the "jump target" becomes an EH label rather than a saved SP.
This is the main unknown to validate in a spike.

### Approach B: native EH codegen (invoke/landingpad → wasm try/catch)

Change the compiler's defer/panic lowering *for wasm* to emit LLVM
`invoke`/`landingpad`: `panic` → `throw` a Go panic tag; functions with defers
wrap their body in a landing pad; `recover` becomes the catch clause.

**Why not (as first choice):** it's a second, wasm-only lowering path parallel to
the setjmp/longjmp one — more compiler complexity and divergence to maintain, and
a larger review. It is the "more idiomatic wasm" endpoint, but Approach A reaches
working `recover()` with far less churn. Keep B as the fallback if SjLj proves
incompatible with asyncify.

### Comparison

| | A: SjLj-backed | B: native EH codegen |
|---|---|---|
| Runtime changes | minimal (flip `supportsRecover`, add primitive) | minimal |
| Compiler changes | wire wasm SjLj, `supportsRecover(wasm)=true` | new wasm defer/panic lowering |
| Reuses deferFrame model | yes | no (parallel path) |
| Divergence from other targets | low | high (wasm-special) |
| Underlying mechanism | wasm EH (via SjLj) | wasm EH (direct) |
| Asyncify interaction risk | **high (shared)** | **high (shared)** |
| Recommended | ✅ start here | fallback |

---

## The #1 risk: asyncify × exception handling

`targets/wasm.json` sets `"scheduler": "asyncify"`. TinyGo's goroutine scheduler
on wasm uses **Binaryen's Asyncify** pass to unwind/rewind the wasm call stack
into linear memory (that is how goroutines yield). Asyncify **rewrites control
flow of the whole module**, and its historical relationship with wasm EH
instructions is the known minefield: an Asyncify pass that doesn't understand
`try`/`catch`/`throw` can miscompile or reject EH-bearing functions.

This is the make-or-break question and must be de-risked **first**:

1. **Isolate the variable.** Prove `recover()` works with `-scheduler=none`
   (no asyncify) before touching the asyncify path. If A/B work there, the
   feature is sound and the remaining problem is purely asyncify coexistence.
2. **Then test `-scheduler=asyncify`.** Determine whether current
   Binaryen/`wasm-opt` preserves EH through Asyncify (recent Binaryen has EH
   awareness; the exact version TinyGo vendors matters).
3. **Fallbacks if they don't coexist:**
   - Gate wasm `recover()` on a non-asyncify scheduler initially (e.g. document
     that `recover()` requires `-scheduler=none`, or the newer stack-switching
     scheduler if/when available), landing the feature for the single-goroutine
     case first. For goowee specifically this is *almost* enough — the render
     loop is single-threaded — but `core.Schedule`/goroutine timers use the
     scheduler, so it isn't a full answer.
   - Explore the **stack-switching** proposal / JSPI-based scheduler as the
     long-term replacement for asyncify, which sidesteps the pass entirely.

Realistically, **asyncify coexistence is where most of the effort and schedule
risk lives**, not the recover mechanism itself.

---

## Detailed work breakdown

Ordered; each item notes the file(s) and what to verify.

### 1. Target feature flags — `targets/wasm.json` (and `wasi*.json`)
- Add `+exception-handling` (and, if EH-based SjLj needs it, `+reference-types`
  for `exnref`) to `features`; add matching `-m` cflags.
- Note: `-reference-types` and `-multivalue` are currently **disabled**; EH may
  require flipping `reference-types`. Validate the minimal feature set.
- Decide whether EH is on by default for wasm or behind an opt-in
  (`-exceptions`-style flag) during stabilization.

### 2. `supportsRecover()` — compiler
- Currently returns false for wasm. Return **true** for wasm when EH is enabled.
- Locate the compiler intrinsic that resolves `runtime.supportsRecover` (grep the
  `compiler` package for `supportsRecover` / `hasReturnsTwice` / the panic
  strategy plumbing). This is a small, decisive change.

### 3. `tinygo_setjmp` / `tinygo_longjmp` for wasm (Approach A) — compiler + runtime
- Provide wasm implementations. Options:
  - Emit LLVM `llvm.eh.sjlj.setjmp`/`longjmp` or C `setjmp`/`longjmp` compiled
    with `-wasm-enable-sjlj` so LLVM lowers them to wasm EH.
  - Or a small runtime shim that maps `tinygo_setjmp`/`longjmp` onto an EH
    throw/catch of a dedicated internal tag.
- Reconcile the `deferFrame.JumpSP` field with wasm (no linear-memory SP): the
  saved state becomes an EH continuation, not a stack pointer. `setupDeferFrame`
  / `destroyDeferFrame` in `src/runtime/defer.go` and `panic.go` may need a
  wasm-tagged variant.

### 4. `trap()` / panic-strategy interplay — `src/runtime/panic.go`
- Ensure `panicStrategy() == PanicStrategyTrap` still traps (that's an explicit
  opt-in and must be preserved), but the **default** strategy now takes the
  longjmp/EH path instead of `abort()`.
- Verify runtime panics (nil deref, index OOB, divide-by-zero) route through the
  recoverable path too — see #3510. This may be out of scope for a first PR
  (Go's own semantics make some runtime panics recoverable); explicitly state
  the initial scope (explicit `panic()` recover first; runtime panics as a
  follow-up).

### 5. Linker / Binaryen — build pipeline
- `wasm-ld` must keep EH sections/tags; ensure `--allow-undefined` and existing
  ldflags don't strip them.
- `wasm-opt`/Asyncify invocation must run with EH enabled (`--enable-exception-handling`)
  and in an order that preserves EH (see risk section). Locate the `wasm-opt`
  invocation in the builder (`builder/*.go`).

### 6. `wasm_exec.js` / host
- Browsers and Node with Wasm 3.0 support EH natively; confirm the shipped
  `targets/wasm_exec.js` needs no change (it shouldn't — EH is in-module).

### 7. Tests — `testdata/` + CI
- Add wasm to the recover/defer test matrix: explicit panic recovered; deferred
  functions run on panic; re-panic; recover returns the value; nested defers;
  panic across goroutines (asyncify).
- Run under both `-scheduler=none` and `-scheduler=asyncify`.
- TinyGo's CI already runs wasm tests via Node — extend `make test` wasm cases.

---

## Testing strategy (mirror goowee's needs)

Minimum bar for goowee to adopt:

1. `func(){ defer func(){ recover() }(); panic("x") }()` recovers and continues.
2. Panic in a nested call chain unwinds to the right deferring frame.
3. Panic inside a goroutine (asyncify path) is recoverable and doesn't corrupt
   the scheduler.
4. Runtime panic (nil map write / index OOB) — recoverable *or* explicitly
   documented as still-fatal for the first release.
5. Binary-size and speed delta from enabling EH is measured (EH adds some size;
   quantify vs the ~285 KB gzip baseline).

Then: build goowee's example under the patched TinyGo and run our existing
`make e2e-tinygo` harness (hydration, routing, refs, async, **error boundary**,
off-loop stopwatch). That suite already exercises exactly the panic/recover paths
that fail today, so it is a ready-made acceptance test.

---

## Phasing

- **Phase 0 — Spike (de-risk, ~1–2 wks).** Minimal patch: `supportsRecover=true`
  for wasm + SjLj primitive, `-scheduler=none` only. Prove one recover works in
  Node. Answer the asyncify question empirically. *Go/no-go gate.*
- **Phase 1 — Explicit `panic()` recover, non-asyncify.** Full defer/recover
  semantics under `-scheduler=none`; test matrix; docs. Land behind a flag if
  needed.
- **Phase 2 — Asyncify coexistence.** Make it work with the default wasm
  scheduler (or document/deliver the scheduler alternative). This is the hard
  phase.
- **Phase 3 — Runtime panics recoverable (#3510).** Optional follow-up.
- **Phase 4 — Default-on + docs** once stable across schedulers.

## Effort & risk (honest)

- **Skill required:** fluency in TinyGo's compiler/builder internals **and**
  LLVM's wasm EH/SjLj backend + Binaryen. This, not LOC, dominates cost.
- **Estimate:** ~**1–3 months** for someone already fluent in both; materially
  more with ramp-up (we would be ramping). The recover mechanism is the small
  part; **asyncify coexistence is the schedule risk.**
- **Confidence:** the mechanism is very likely feasible (the primitive now
  exists and TinyGo's model is a clean fit). The asyncify interaction is the real
  unknown — Phase 0 exists to resolve it before committing.

## Upstream, don't fork

- #2914 is open, wanted, and names the EH approach. A well-scoped PR is the right
  vehicle; carry a patched toolchain only until merge.
- A fork means owning divergent TinyGo **and** its LLVM/Binaryen toolchain
  forever, for no benefit over a merged PR. Only justified if upstream rejects
  the direction — unlikely, since it is their stated roadmap.
- Highest-leverage first move: post the "blocker is resolved" update on #2914
  (see the drafted comment), and run Phase 0 to bring **data** (a working
  `-scheduler=none` proof) to the thread. A spike branch + demo is far more
  persuasive to maintainers than a design comment alone.

## Open questions to resolve with the compiler source / maintainers

1. Exact compiler location that resolves `supportsRecover` and emits
   `tinygo_setjmp`/`longjmp` (which asm/intrinsic path).
2. Does the vendored Binaryen preserve EH through Asyncify today, and at what
   version? (Determines whether Phase 2 is "wire it up" or "replace the
   scheduler.")
3. Minimal target-feature set for EH-based SjLj (does it require
   `+reference-types`/`exnref`, or is the exception-handling feature alone
   enough for the LLVM version TinyGo uses?).
4. Scope of the first PR: explicit `panic()` only, or also runtime panics
   (#3510)?

## References

- tinygo#2914 — Implement recover for wasm architecture
- tinygo#3510 — runtime panics are not recoverable
- tinygo#2742 — implement `-panic=reset`
- [Wasm 3.0 Completed — webassembly.org, 2025-09-17](https://webassembly.org/news/2025-09-17-wasm-3.0/) (EH finalized)
- WebAssembly/exception-handling — the proposal repo
- LLVM: Wasm EH and Wasm SjLj (`-wasm-enable-sjlj`); Binaryen Asyncify
- Local evidence: `src/runtime/panic.go`, `src/runtime/defer.go`,
  `src/runtime/asm_tinygowasm.S`, `targets/wasm.json` (TinyGo 0.41.1)
