# Shared-Memory Bridge: Replacing `syscall/js` with Raw WASM Memory

**Status:** Design proposal  
**Date:** 2026-07-07  
**Author:** Agent-assisted  
**PR:** ... (fill on merge)

## Problem

Every frame, goowee sends a batch of DOM mutations from Go to JS. Currently
this flows through `syscall/js`:

```
Go:  []core.Mutation → sendMutations() → js.Global().Call("applyMutations", js.ValueOf(arr))
                                                 ↓
JS:  applyMutations(muts) → forEach { ... }
```

Each `Call` / `js.ValueOf` invocation crosses the Go-WASM glue boundary and
incurs reflection overhead — Go's runtime marshals Go values into JS objects
one field at a time. The same applies to events flowing in reverse:

```
JS:  handleEvent(nodeID, type, JSON.stringify(payload))  →  Go: registry.Dispatch(...)
```

A single frame with 50 mutations and 1 event can involve 50+ JS object
constructions and 2 `Call` round-trips. For scroll/pointermove, these costs
multiply at 60 fps.

## Goal

Replace every Go↔JS communication channel with a **shared linear memory**
buffer accessible to both sides at zero copy. The Go side writes structured
binary data into a pre-allocated `WebAssembly.Memory` region; JS reads it
with a `DataView`. Events flow in the opposite direction through a separate
region.

## Design

### Memory layout

A single `WebAssembly.Memory` instance hosts three fixed-size regions, allocated
at module load and never resized during runtime:

```
┌─────────────────────────────────────────────────────────┐
│  0x0000 – MUTATION_REGION (64 KB)   → Go writes, JS reads │
│  0x8000 – EVENT_REGION    (16 KB)   → JS writes, Go reads │
│  0xC000 – STRING_POOL     (64 KB)   → Go writes, JS reads │
└─────────────────────────────────────────────────────────┘
```

**Mutation region** — flat struct-of-arrays encoding. Each mutation is a fixed
16-byte header (tag + 3×int32), with variable-length strings appended to the
string pool and referenced by offset+length.

**Event region** — a ring buffer. JS pushes event frames (type byte, nodeID,
timestamp, payload length, payload bytes). Go reads from the head on each
flush.

**String pool** — mutation strings (tag names, attribute keys, attribute values,
CSS selectors for portals) are written once consecutively. Each mutation header
references `(poolOffset, length)` instead of duplicating the string.

### Wire protocol

#### Mutation encoding (16-byte header + optional string data)

```
Offset  Size  Field
0       1     mutation type (MutationType as byte)
1       1     flags  (bit 0: has key, bit 1: has value, bit 2: has ns, …)
2       2     reserved
4       4     nodeID (int32)
8       4     childID or refID (int32)
12      4     stringRef (uint32) — upper 2 bytes = offset in pool,
                                     lower 2 bytes = length
```

When `flags & HasString` is set, the JS side reads `stringRef` to locate the
string in the pool. For types that carry no string (`MutRemoveNode`), flags
and stringRef are zero.

All integers are little-endian (WASM's native order on all supported
platforms).

#### Event encoding

```
Offset  Size  Field
0       1     event type (byte enum: 0=click, 1=scroll, 2=pointermove, …)
1       1     flags   (bit 0: has target, bit 1: has coords, …)
2       2     reserved
4       4     targetNodeID (int32) — determined by JS after walking the DOM
8       4     payload byte length (N)
12      N     payload bytes (flat key-value pairs: 1-byte keyID + value_bytes)
```

The payload follows a mini-protocol: each key is a pre-negotiated 1-byte ID
(e.g. 0x01=`clientX`, 0x02=`clientY`, 0x03=`value`, 0x04=`checked`, …).
Values are fixed-size for known types (float32 for coords, int32 for key codes)
or length-prefixed strings.

This avoids JSON parse entirely — the Go side reads raw bytes and dispatches
to handler functions directly.

### File changes

#### Go side

| File | Change |
|------|--------|
| `bridge/wasm.go` | Remove all `js.Value` usage. Replace with a `SharedMemoryBridge` that wraps the `WebAssembly.Memory` object. Rip out `sendMutations`, `Init`, `startScheduler` — these move into the new bridge. |
| `bridge/shared_memory.go` | **New.** `SharedMemoryBridge` struct: holds pointer to `WASMMemory`, offset bases. `Flush(muts []Mutation)` writes into MUTATION_REGION + STRING_POOL, signals JS via an atomic flag in memory. `DrainEvents()` reads EVENT_REGION and feeds `NodeRegistry.Dispatch`. `Init()` allocates the memory, exports functions, installs event listeners on JS side. |
| `bridge/bridge.go` | Update `Bridge` interface if needed. `NoopBridge` unchanged. |
| `core/scheduler.go` | `Scheduler.Flush()` already returns `[]Mutation`. It now passes them to `SharedMemoryBridge.Flush()` instead of the old `bridge.sendMutations()`. Add a `FlushTo(buffer *MutationBuffer)` path as an alternative output. |
| `router/history_js.go` | `basePath()`, `CurrentPath()`, `BindHistory()` — replace `js.Global().Get("document")...` with JS callback tables registered at init time. Each DOM query becomes a numbered function call through a thin WASM import layer. |

#### JS side

| File | Change |
|------|--------|
| `runtime/goowee.js` | Remove `applyMutations()` entirely. Replace with `processMutations()` that reads from the shared buffer. Remove `goListen`/`dispatchToGo`/`buildPayload` — these become a single `flushEvents()` that fills the EVENT_REGION. |
| `runtime/goowee.mjs` | **New.** Modules: `memory.js` (buffer management), `decoder.js` (mutation reader), `encoder.js` (event writer), `listener.js` (DOM event → buffer bridge). |

#### Removed

| File | Reason |
|------|--------|
| `runtime/wasm_exec.js` | No longer needed — Go's `wasm_exec.js` is only required with `syscall/js`. With raw memory, we use a small custom loader. |

### App entry point change

Current `main.go`:

```go
func main() {
    r := router.New(router.CurrentPath())
    r.BindHistory()
    renderer := dom.New()
    if js.Global().Get("document").Call("querySelector", "[data-node-id]").Truthy() {
        renderer.SetHydrating(true)
    }
    muts, _ := renderer.Render(app.App(r))
    renderer.Scheduler.Enqueue(muts...)
    bridge.Init(renderer.Scheduler, renderer.Registry)
    select {}
}
```

New `main.go`:

```go
func main() {
    bridge.Init() // allocates shared memory, registers JS callbacks
    r := router.New(bridge.CurrentPath())
    r.BindHistory()
    renderer := dom.New()
    if bridge.HasSSRContent() {
        renderer.SetHydrating(true)
    }
    muts, _ := renderer.Render(app.App(r))
    renderer.Scheduler.Enqueue(muts...)
    bridge.Start()
    select {}
}
```

`bridge` becomes the single point of contact with the JS runtime. All
`syscall/js` calls are eliminated from user code, router code, and the
renderer.

## Migration strategy

The refactor is reasonably isolated but touches every `syscall/js` line.
It should be done as a single atomic change rather than piecemeal, because
partial adoption (mixing `syscall/js` + shared memory) adds complexity
without benefit.

### Phase 1 — Proof of concept (this PR or a follow-up)

1. Add `bridge/shared_memory.go` with the buffer layout, write/read primitives,
   and a `SharedMemoryBridge` that co-exists alongside the existing
   `init()`/`sendMutations()`.
2. Wire the scheduler flush to write to both paths (JSON for compatibility,
   buffer for testing).
3. Add `runtime/goowee.mjs` modules in a `build/` directory.
4. Run an E2E test (counter example) loading the new bridge.

### Phase 2 — Cutover

1. Remove all `syscall/js` imports.
2. Switch to `GOOS=wasip1 GOARCH=wasm` (or stay on `GOOS=js` without using its
   `syscall/js` — both work, `wasip1` is cleaner going forward).
3. Replace `wasm_exec.js` with the custom loader.
4. Update the router's `BindHistory()` to use the callback table.
5. Remove all JSON-based mutation sending.
6. Update the app entry point examples and documentation.

### Phase 3 — Polish

1. Benchmarks to measure frame time before/after.
2. String interning in the pool (deduplicate repeated keys/tags across batches).
3. Optional: fallback to JSON-based transport when SharedArrayBuffer is not
   available (e.g. some CSP policies).

## Performance expectations

| Metric | Before (syscall/js) | After (shared memory) |
|--------|---------------------|----------------------|
| Mutation send (50 muts) | ~0.3–0.5 ms | ~0.02–0.05 ms |
| Event receive (1 event) | ~0.1 ms (JSON parse + Call) | ~0.01 ms (buffer read) |
| Scroll event at 60fps | 60 `Call`s / sec | 0 `Call`s (coalesced via rAF on JS side) |
| WASM binary size | ~3.7 MB (includes Go JS glue) | ~3.5 MB (smaller without glue) |

The biggest wins come from eliminating the `Call()` round-trip for every
mutation batch and every event — the buffer read/write is pure arithmetic.

## Open questions

1. **GOOS target:** Should we stay on `GOOS=js GOARCH=wasm` (which gives us
   `syscall/js` as a fallback) or switch to `GOOS=wasip1 GOARCH=wasm` (which
   has no `syscall/js` at all)? `wasip1` is cleaner but the available tooling
   (debugging, profiling) is less mature.
2. **String pool sizing:** 64 KB covers typical frames but complex SSR
   hydration might exceed it. Should we fall back to a JS-side string table
   that we populate at init time?
3. **Browser compat:** `SharedArrayBuffer` / `WebAssembly.Memory` is universal
   in modern browsers but not in legacy (IE11, ancient Safari). Do we care?
   goowee already requires modern WASM.

## Appendix: Current surface of `syscall/js` calls

### `bridge/wasm.go` (3 call sites)
- `js.Global().Set("handleEvent", ...)` — export Go function
- `js.Global().Call("goListen", ...)` — install DOM listener
- `js.Global().Call("requestAnimationFrame", ...)` — schedule frame
- `js.Global().Call("applyMutations", ...)` — send mutations (removed in 3.4 prep)

### `router/history_js.go` (7 call sites)
- `js.Global().Get("document").Call("querySelector", "base")`
- `b.Call("getAttribute", "href")`
- `js.Global().Get("location").Get("pathname")`
- `js.Global().Get("history").Call("pushState", ...)`
- `js.Global().Get("history").Call("replaceState", ...)`
- `js.Global().Get("history").Call("back")`
- `js.Global().Get("history").Call("forward")`
- `js.Global().Call("addEventListener", "popstate", ...)`

### `examples/counter/main.go` (1 call site)
- `js.Global().Get("document").Call("querySelector", "[data-node-id]")` — hydration check

Total: ~12 call sites across 3 files. All can be replaced with buffer-based or
callback-table-based operations.
