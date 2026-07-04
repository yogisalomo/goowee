# Codex Implementation Plan: Go WASM Signal-Based UI Framework (DOM + SSR)

## Goal

Build a minimal but working UI framework in Go that runs in the browser via WebAssembly and targets the DOM as its only rendering backend.

The framework must support:

- Fine-grained reactivity via signals
- Hook-based developer API (React-like ergonomics)
- Frame-batched DOM updates (requestAnimationFrame loop)
- Server-side rendering (SSR) with hydration
- Lenient hydration model
- Manual dependency declaration (NO automatic tracking)
- No virtual DOM diffing as primary update mechanism
- Minimal JavaScript bridge (event forwarding + DOM mutation only)

---

# Core Architecture

```
Application Code (Go)
    ↓
Hooks Layer (UseState, UseEffect)
    ↓
Signal System (manual dependency declaration)
    ↓
Scheduler (frame-based batching)
    ↓
DOM Binding Layer (signal → DOM node bindings)
    ↓
Mutation Queue (batched DOM operations)
    ↓
JS Bridge (thin layer only)
    ↓
Browser DOM
```

---

# 1. Signal System (CORE)

## Requirements

- Manual dependency declaration — the developer lists which signals an effect depends on
- Signals are the core reactive primitive
- Signals notify subscribers on value change
- Must support cleanup of subscriptions

## API

```go
type Signal[T any] struct {
    value       T
    subscribers []func()
}

func NewSignal[T any](initial T) *Signal[T]

func (s *Signal[T]) Get() T
func (s *Signal[T]) Set(value T)
func (s *Signal[T]) Subscribe(fn func()) func() // returns unsubscribe function
```

## Effects

```go
func Effect(deps []SignalAccessor, fn func() func())
```

Rules:

* Executes once initially
* Re-executes when any dependency changes
* `fn` returns `func()` — the cleanup function (may return nil)
* Cleanup is called before re-execution and on component unmount

---

# 2. Hook Layer (Developer API)

Hooks are syntactic sugar over signals.

## API

```go
func UseState[T any](initial T) (*Signal[T], func(T))
func UseEffect(deps []core.SignalAccessor, fn func() func())
func UseScope(fn func() core.Node, deps ...core.SignalAccessor) core.Node
```

## State model (Solid-style, run-once)

A component's setup function runs **once**, when it mounts. It is not
re-executed on updates, so this is Solid's model, not React's — there is no
hook-slot table and no call-order rule. `UseState` simply creates a signal;
the closures in the returned tree capture it. Updates happen at a finer grain
than the component: signal **bindings** update one node, and **scopes**
(`UseScope` / `h.Show` / `h.For` / `h.Switch`) re-run a small closure and diff.

Each mounted component gets a `ComponentFrame` (see `core/component_tree.go`),
which is the **ownership/disposal scope**: effects (`OnMount`/`Watch`/
`UseEffect`) and `Computed`/`Watch` subscriptions created during setup register
on the frame, and unmounting the component disposes the frame. The frame stack
lives on a per-render `RenderContext` (not a global), so concurrent SSR renders
stay isolated. See `docs/component_tree.md` for the full lifecycle and
reconciliation design.

`UseScope` returns a `ScopeNode` that re-executes a render closure when
dependencies change and diffs the old vs new tree. Components inside the tree
are matched by name and **preserved** across re-renders (their state and
effects survive); only added components mount and only removed ones unmount.

## Rules

* Components run once on mount — no hook-order rules; call `UseState`/effects
  wherever you like, including conditionally.
* State lives in signals captured by closures, not in per-render slots.
* Hooks work outside a component context too (state simply isn't owned by any
  frame, so it won't be auto-disposed).

---

# 3. Component Model

* Pure functional components only
* Must be deterministic

Example:

```go
func Counter() Node
```

Note: Components are wrapped with `core.Component(name, fn)` to create a `ComponentNode`:

```go
core.Component("Counter", func() core.Node { ... })
```

## Node Types

```go
// Node is the universal return type for all components.
type Node interface {
    nodeMarker()
    String() string
}

// ElementNode represents a DOM element.
type ElementNode struct {
    ID       int                // set by renderer
    Tag      string
    Props    map[string]any     // string | SignalAccessor | EventHandler | bool | ...
    Children []Node
}

// TextNode represents a text leaf. Value may be a literal string
// or a *Signal[T] for reactive text.
type TextNode struct {
    ID    int  // set by renderer
    Value any  // string | *Signal[T]
}

// FragmentNode groups multiple children without a wrapper element.
type FragmentNode struct {
    Children []Node
}

// ComponentNode wraps a named component function for re-execution.
type ComponentNode struct {
    Name   string
    Render func() Node       // called each render
    Prev   Node              // last rendered inner tree (set by renderer)
    Frame  *ComponentFrame   // frame from last render (set by renderer)
}

func Component(name string, render func() Node) *ComponentNode

// ScopeNode enables structural re-rendering when deps change.
// On first render, it sets up signal subscriptions. On dep change,
// it re-runs Render, diffs old vs new tree, and enqueues mutations.
type ScopeNode struct {
    Render func() Node
    Deps   []SignalAccessor
    Prev   Node               // expanded tree from last render
    Frames []*ComponentFrame  // component frames from last render (for cleanup)
    Unsubs []func()           // signal subscription cancellations
}
```

This type hierarchy enables both DOM and SSR renderers.

## Component Identity

Each component instance is tracked by a `ComponentFrame` in a lightweight tree. During render, `PushComponent()` pushes a `ComponentFrame` onto a global stack with a path like `"root/0/1"`. This path is used for hook isolation, unmount cleanup, and hydration matching.

See [`docs/component_tree.md`](component_tree.md) for the full design.

## Structural Re-rendering: UseScope

`UseScope` is the mechanism for structural updates (e.g., signal-controlled `if`):

```go
func UseScope(fn func() Node, deps ...core.SignalAccessor) core.Node
```

Usage:

```go
count, setCount := hooks.UseState(0)
show, setShow := hooks.UseState(true)

greeting := hooks.UseScope(func() core.Node {
    if show.Get() {
        return &core.ElementNode{
            Tag: "p",
            Props: map[string]any{"textContent": "Hello!"},
        }
    }
    return nil
}, show)
```

**Behavior:**
1. First render: executes `Render()`, assigns node IDs, stores tree in `Prev`.
2. Subscribes to each dep signal.
3. On signal change: re-executes `Render()`, calls `FlatTreeWithFrames()` to expand components, diffs `Prev` vs new tree, enqueues mutations via scheduler.
4. Runs cleanup for old component frames (components that were removed by the diff).

### Tree Diff Rules (implemented in `dom/renderer.go`)

| Old child | New child | Action |
|---|---|---|
| `ElementNode` | `ElementNode`, same tag | Reuse node ID, recurse children, emit per-property diffs |
| `ElementNode` | `nil` | Emit `MutRemoveNode` |
| `nil` | `ElementNode` | Recursively emit create + append for subtree |
| `TextNode` | `TextNode` | Compare resolved values, emit `MutSetProperty` if different |
| `ElementNode` | `ElementNode`, diff tag | Remove old subtree, emit create for new |
| Old child removed | — | Emit `MutRemoveNode` |
| — | New child at middle position | Emit `MutInsertBefore` |

The diff reuses existing node IDs from the old tree (e.g., `new.ID = old.ID`), so the DOM renderer never re-allocates for existing nodes.

## Utility Functions

```go
// FlatTree expands ComponentNodes into their inner elements.
// FragmentNodes are flattened into parent children lists.
func FlatTree(n Node) Node

// FlatTreeWithFrames is like FlatTree but also collects all ComponentFrame
// pointers created during flattening.
func FlatTreeWithFrames(n Node) (Node, []*ComponentFrame)

// CollectIDs returns all non-zero node IDs from a tree (for cleanup).
func CollectIDs(n Node) []int
```

## Props Flow

Props use **Option A: Static Snapshots**. Props are plain values passed at call time. If a parent needs to pass updated props, the parent uses `UseScope` to re-call the child component on signal changes.

Signal props are also supported natively via `SignalAccessor`: props of type `*Signal[T]` are automatically bound by the renderer (the `BindingRegistry` subscribes to them), making the child's DOM reactive without re-rendering the child component.

---

# 4. DOM Binding System

## Core Principle

Signals bind directly to DOM nodes.

No virtual DOM diffing.

## Signal Accessor Interface

To let `BindingRegistry` hold heterogeneous signal types without generics, signals implement a common accessor interface:

```go
type SignalAccessor interface {
    Value() any
    Subscribe(func()) func()
}
```

`Signal[T]` implements `SignalAccessor` with a separate method name to avoid Go's no-overloading restriction:

```go
// Get returns the typed value (for user code).
func (s *Signal[T]) Get() T { return s.value }

// Value satisfies SignalAccessor (for the binding layer).
func (s *Signal[T]) Value() any { return s.value }
```

## Binding Mechanism

A `BindingRegistry` maps node IDs to signal subscriptions:

```go
type Binding struct {
    NodeID   int
    Signal   SignalAccessor
    Property string // e.g., "textContent", "className", "value"
}

type BindingRegistry struct {
    bindings map[int][]Binding // nodeID → list of bindings
}

func (r *BindingRegistry) Bind(nodeID int, signal SignalAccessor, property string)
func (r *BindingRegistry) Unbind(nodeID int)
func (r *BindingRegistry) GetBindings(nodeID int) []Binding
```

## Behavior

* `Bind()` registers the binding AND subscribes to the signal. The subscriber callback enqueues a mutation via the scheduler rather than firing eagerly.
* `Bind()` returns an unsubscribe function for cleanup.
* On signal update → the scheduler flushes queued mutations at the next frame.
* `Unbind(nodeID)` removes all bindings for a node and unsubscribes from each signal.
* Called during cleanup (component unmount or effect re-run).

## Event Handler Registry

In addition to property bindings, each DOM node may have event handlers registered during rendering:

```go
type EventHandler func(event EventData)

type DOMNode struct {
    ID     int
    Events map[string]EventHandler // "click" → handler
}

type NodeRegistry struct {
    nodes map[int]*DOMNode
}

func (r *NodeRegistry) RegisterHandler(nodeID int, eventType string, handler EventHandler)
func (r *NodeRegistry) Dispatch(nodeID int, eventType string, data string)
```

Event handlers are a separate path from property bindings — they are set up during initial render and hydration, and torn down during unmount. The JS bridge never knows about specific handlers; it only forwards `(nodeID, eventType, data)` triples.

## Rendering Pipeline (Client)

The DOM renderer walks the `Node` tree produced by a component and emits an initial batch of mutations:

```go
type DOMRenderer struct {
    nextID    int
    Bindings  *BindingRegistry
    Scheduler *Scheduler
    Registry  *NodeRegistry
}

func (r *DOMRenderer) Render(node Node) ([]Mutation, int) {
    var muts []Mutation
    rootID := r.renderNode(node, &muts)
    return muts, rootID
}

// renderNode returns the allocated node ID for the subtree root.
func (r *DOMRenderer) renderNode(n Node, muts *[]Mutation) int
```

The pipeline:

```
Component returns Node tree
    ↓
DOMRenderer walks tree depth-first
    ↓
For each ElementNode:
  - Assign monotonically increasing nodeID
  - Emit MutCreateElement(nodeID, tag)
  - For each prop:
    - If value is string → emit MutSetAttribute or MutSetProperty
    - If value is bool + property → emit MutSetProperty
    - If value is SignalAccessor → BindingRegistry.Bind(nodeID, signal, key)
    - If value is EventHandler → NodeRegistry.RegisterHandler(nodeID, eventType, handler)
  - Recurse into children:
    - Emit MutAppendChild(parentNodeID, childNodeID)
    - Process child subtrees
For each TextNode:
  - Emit MutCreateElement(nodeID, "#text")
  - If value is string → MutSetProperty(nodeID, "textContent", val)
  - If value is *Signal → Bind + MutSetProperty
For each ComponentNode:
  - PushComponent(), call Render(), recurse, PopComponent()
  - Store ComponentFrame with RootNodeIDs
For each ScopeNode (first render):
  - Call Render(), FlatTreeWithFrames(), render flattened tree
  - Subscribe to each Dep signal for re-render
  - Store Prev tree
    ↓
Mutation queue flushed via scheduler
    ↓
JS bridge applies mutations to real DOM
```

---

# 5. Scheduler (Frame-Based Batch System)

## Requirements

* All DOM updates are queued
* Flush occurs once per animation frame (requestAnimationFrame)
* Prevents excessive DOM thrashing

### Flow

```
Signal update
    ↓
Queue mutation
    ↓
requestAnimationFrame tick
    ↓
Flush all DOM mutations
```

---

# 6. Mutation System

All DOM updates are expressed as batched operations.

## Mutation Types

```go
type MutationType int

const (
    MutCreateElement MutationType = iota
    MutRemoveNode
    MutSetAttribute
    MutSetProperty
    MutAppendChild
    MutInsertBefore
)
```

## Mutation struct

```go
type Mutation struct {
    Type    MutationType
    NodeID  int    // target node (or parent for MutAppendChild)
    Key     string // property name, attribute name, or "tag" for MutCreateElement
    Value   any    // value (string, int, bool, etc.)
    ChildID int    // used by MutAppendChild and MutInsertBefore
}
```

## Supported operations:

| MutationType     | Description                              |
|------------------|------------------------------------------|
| MutCreateElement | Create a new DOM element                 |
| MutRemoveNode    | Remove a DOM node                        |
| MutSetAttribute  | Set an HTML attribute on a DOM node      |
| MutSetProperty   | Set a JS property (e.g., textContent, checked, value, className, disabled) |
| MutAppendChild   | Append a child node to a parent          |
| MutInsertBefore  | Insert a child node at a specific position (used by diff for reorder) |

### Property vs Attribute Dispatch

The Go renderer decides which mutation type to emit based on the property key:

| Go renderer input          | Mutation type     | JS bridge operation         |
|----------------------------|-------------------|-----------------------------|
| `"class"`, `"id"`, `"href"`| `MutSetAttribute` | `el.setAttribute(key, val)` |
| `"textContent"`, `"checked"`, `"value"`, `"className"`, `"disabled"`, `"innerText"` | `MutSetProperty` | `el[key] = val`             |

All mutations MUST be batched per frame.

---

# 7. JS Bridge (Minimal Layer)

## Responsibilities

JavaScript must ONLY:

* Receive DOM mutation batches from WASM and apply them
* Forward DOM events to WASM
* Trigger requestAnimationFrame loop
* Bootstrap WASM runtime

NO application logic in JS.

## Import Boundary Rule

Application code must never import `syscall/js`. Only the `/bridge` package uses it. This keeps the WASM-JS surface area minimal and contained.

## Bootstrap Sequence

1. Browser loads `wasm_exec.js` (Go's standard WASM support).
2. Browser fetches and instantiates the WASM binary via `Go` instance.
3. The WASM `main()` function runs, which calls `bridge.Init()`.
4. `bridge.Init()` registers event handlers on the JS global object and starts the `requestAnimationFrame` scheduler loop.
5. The Go `main()` blocks on a channel to keep the WASM instance alive.

### Main Package

```go
package main

import "github.com/yourorg/gowee/bridge"

func main() {
    done := make(chan struct{})
    bridge.Init()
    <-done
}
```

### Bridge Initialization

```go
// bridge/js.go
package bridge

import "syscall/js"

func Init() {
    js.Global().Set("handleEvent", js.FuncOf(func(this js.Value, args []js.Value) any {
        nodeID := args[0].Int()
        eventType := args[1].String()
        data := args[2].String()
        dispatchEvent(nodeID, eventType, data)
        return nil
    }))
    startScheduler()
}
```

This avoids `//export` complications entirely and follows the actual Go WASM convention. Application code never imports `syscall/js` — only the `/bridge` package does.

---

# 8. SSR System (Server-Side Rendering)

## Requirements

* Render components to HTML string on server
* NO reactive graph during SSR
* Must produce hydration metadata

## Output

* HTML with `data-node-id` attributes for hydration mapping
* Hydration metadata as JSON

## Hydration Metadata Schema

```go
type SlotRef struct {
    ComponentPath string // e.g., "root/0" (frame path)
    HookIndex     int    // which hook call in the component
}

type HydrationMeta struct {
    NodeMap map[int]string       // nodeID → component path
    Deps    map[int][]SlotRef    // nodeID → [{componentPath, hookIndex}]
}
```

The SSR renderer walks the same `Node` types as the DOM renderer, including `ComponentNode` and `ScopeNode`:

```go
type Renderer struct {
    nextID int
    Meta   HydrationMeta
}

func (r *Renderer) Render(n core.Node) string
func (r *Renderer) RenderWithMeta(n core.Node, path string, hooks []core.SignalAccessor) (string, HydrationMeta)
```

On each `ComponentNode`, it calls `PushComponent()` / `PopComponent()` to maintain the frame stack. `ScopeNode` is rendered by calling its `Render()` function.

HTML output includes `data-node-id="N"` attributes on elements. The metadata maps node IDs to component paths and signal dependencies. The rendered HTML and metadata are embedded in the server response.

Note: Full client-side hydration (replaying the component tree and subscribing signals) is partially implemented — the SSR output with metadata is generated, but the WASM client-side hydration reconnection is deferred.

## Renderer Abstraction

The DOM renderer (client) and SSR renderer (server) both consume the same `Node` tree (see [Section 3](#3-component-model)). They should share a walker that produces a backend-agnostic intermediate representation, then each backend materializes it differently:

```
Node tree
    ↓
Shared walker (emit events: createElement, setProperty, appendChild, setText, ...)
    ├── DOM backend → Mutations → JS bridge
    └── SSR backend → strings.Builder → HTML
```

For MVP, the SSR renderer can be a standalone `walkNode(buf, node)` function that serializes directly to HTML string. The abstraction is added when it becomes clear that the two backends diverge.

---

# 9. Hydration System (Lenient)

## Status

The SSR renderer generates `data-node-id` attributes and hydration metadata (see Section 8). Full client-side hydration — where WASM boots, replays the tree, and reconnects signals to existing DOM nodes — is **partially implemented**. The metadata schema and SSR output are complete. The client-side reconnection logic is deferred.

## Behavior (Target)

* Match DOM nodes via node IDs (from `data-node-id` attributes)
* Attach signals after SSR render using the hydration metadata
* Recover gracefully from mismatches (NO hard failure)

## Mismatch Handling (Target)

When SSR HTML differs from what the client-side component would render:

1. Log a warning to the console via JS bridge.
2. Force a full re-render of the mismatched subtree.
3. Continue hydrating the rest of the tree normally.

## Implementation Progress

| Component | Status |
|-----------|--------|
| SSR data-node-id generation | ✅ Done |
| HydrationMetadata struct + generation | ✅ Done |
| NodeMap + Deps mapping | ✅ Done |
| Client-side tree replay + signal reconnection | ❌ Deferred |

The streaming HTML server (`cmd/ssr-server/main.go`) uses SSR to render pages, but client hydration is not yet connected.

---

# 10. Event System

## Handler Registration

Event handlers are not set via HTML attributes. Instead, the DOM renderer registers them in a `NodeRegistry` during component rendering:

```go
func Button(props ButtonProps) Node {
    return dom.Element("button", dom.Props{
        "onclick": func(ev EventData) {
            props.OnClick()
        },
    })
}
```

The renderer calls `nodeRegistry.RegisterHandler(nodeID, "click", handler)` for each event prop. These handlers are torn down during unmount via the `ComponentNode` tree walk.

## Flow

```
DOM Event
    ↓
JS Bridge (handleEvent)
    ↓
WASM dispatchEvent(nodeID, eventType, data)
    ↓
NodeRegistry.Dispatch(nodeID, eventType, data)
    ↓
Event handler calls UseState setter
    ↓
Queued DOM Mutation
    ↓
Frame Flush
```

The JS bridge remains generic — it only forwards `(nodeID, eventType, data)` triples. All handler logic lives in Go.

---

# 11. Error Handling Strategy

## Rules

| Scenario                                   | Behavior                                          |
|--------------------------------------------|---------------------------------------------------|
| Effect panics during execution             | `recover()` catches it, log error via JS bridge   |
| SSR/client render mismatch                 | Log warning, force re-render of mismatched subtree|
| Component returns nil                       | Treat as empty fragment, log warning              |
| Signal Set called with nil (for nil-able T)| Allowed — notify subscribers with zero value      |
| Hook called outside component context       | Panic with clear error message                    |

No full error recovery system for MVP — just predictable, debuggable behavior when things go wrong.

---

# 12. Project Structure

```
/core
    signal.go           // Signal[T], SignalAccessor
    node.go             // Node, ElementNode, TextNode, FragmentNode, ComponentNode, ScopeNode, EventData, FlatTree, CollectIDs
    component_tree.go   // ComponentFrame, PushComponent, PopComponent, CurrentComponent
    scheduler.go        // Scheduler, Mutation, MutationType
    binding.go          // Binding, BindingRegistry

/hooks
    use_state.go        // UseState
    use_effect.go       // UseEffect, RunFrameCleanup
    use_scope.go        // UseScope

/dom
    renderer.go         // DOMRenderer (Render, diffNode, ScopeNode re-render)
    node_registry.go    // NodeRegistry, DOMNode

/ssr
    renderer.go         // Renderer (ssr renderer, hydration metadata)

/bridge
    bridge.go           // Bridge interface, NoopBridge
    wasm.go             // Init, scheduler, mutation dispatch (js+wasm build only)

/html
    html.go             // HTML element helpers (Div, Span, P, etc.), VirtualList

/router
    router.go           // Router, Navigate, Link, Route

/examples
    counter/
        main.go         // WASM entry point (go:build js && wasm)
        app/
            app.go      // Full demo app with routes: home, counter, form, todos, stopwatch, dashboard

/cmd
    ssr-server/
        main.go         // HTTP server with SSR + static file serving
```

---

# 13. Testing Strategy

## Unit Tests (Pure Go, No WASM)

These test core logic without browser dependencies:

* Signal system — subscribe, notify, unsubscribe, cleanup
* Scheduler queue — enqueue, flush ordering, frame batching
* Mutation queue — batch accumulation, clear on flush
* Hook state management — slot allocation, component isolation
* SSR HTML rendering + hydration metadata generation — component to string, node ID assignment, attribute serialization
* Component tree — push/pop stack, parent/child traversal, unmount walk
* Node type — construction, prop access, serialization to/from DOM and SSR

Run with: `go test ./core/... ./hooks/... ./ssr/...`

## Integration Tests (Requires WASM Build)

These test the full pipeline end-to-end:

* Counter renders in browser, click updates DOM
* SSR produces HTML, hydration activates interactivity
* Event flow: click → signal update → DOM mutation

Run with: `GOOS=js GOARCH=wasm go test ./...` (or a small test harness that loads the WASM binary in a headless browser).

---

# 14. MVP Scope

## Implemented:

* ✅ Signal system (generics, subscribe/unsubscribe)
* ✅ UseState / UseEffect / UseScope hooks
* ✅ ComponentNode + ComponentFrame tree
* ✅ DOM renderer (initial render + structural diff)
* ✅ Frame batching system (scheduler)
* ✅ JS bridge (minimal, WASM build)
* ✅ SSR HTML renderer (with hydration metadata)
* ✅ Event handling (click → signal → DOM update)
* ✅ Keyed list reconciliation via diff (position-based)
* ✅ Router package
* ✅ HTML element helpers
* ✅ VirtualList (windowed rendering)
* ✅ Server-side streaming (cmd/ssr-server)

## Example app:

* Multi-page demo with routes: home, counter, form, todos, stopwatch, dashboard
* SSR renders initial HTML for every route
* Client-side WASM takes over after load
* Property-level reactivity (signals bound to DOM)
* Structural reactivity (UseScope with diff)
* VirtualList with windowed rendering (100 items)

## Deferred:

* ❌ Full client-side hydration reconnection (metadata generated, replay not connected)
* ❌ Automatic dependency tracking
* ❌ Compiler-based optimizations

---

# 15. Phased Rollout

| Phase | Scope                                      | Status |
|-------|--------------------------------------------|--------|
| 1     | Signal system + scheduler + hooks + component tree | ✅ Done — all unit tests pass |
| 2     | DOM binding + JS bridge + mutations        | ✅ Done — counter renders, click updates DOM |
| 3     | SSR renderer + hydration system            | 🟡 Partial — SSR HTML + metadata done, client hydration deferred |
| 4     | UseScope + structural diff + router         | ✅ Done — UseScope, diff, router, VirtualList |
| 5     | Polish: examples, SSR server, integration tests | ✅ Done — multi-page demo, SSR HTTP server, test coverage |

Each phase is a checkpoint. If a phase reveals unexpected complexity, adjust scope before moving to the next.

---

# 16. Explicit Non-Goals

DO NOT implement (but some are done):

* ✅ Virtual DOM diffing engine — **not implemented** (only minimal structural diff for ScopeNode re-renders)
* ❌ Automatic dependency tracking — **not implemented** (manual declaration required)
* ❌ Compiler-based optimizations — **not implemented**
* ✅ Routing system — **implemented** in `/router` package
* ❌ Animation system — **not implemented**
* ❌ CSS-in-Go system — **not implemented**
* ❌ Complex layout engine — **not implemented**
* ✅ List reconciliation — **implemented via position-based diff** (not keyed, but structural diff handles list insertions/removals correctly within ScopeNode re-renders)

---

# 17. Success Criteria

The framework is successful if:

* SSR renders static HTML correctly
* Hydration activates interactivity in browser
* Signal updates trigger minimal DOM mutations
* No full tree re-render occurs
* Frame batching is respected
* Counter example works end-to-end

---

# 18. Key Constraints

* Go + WebAssembly only
* DOM is the only rendering target
* Application code must never import `syscall/js` — only `/bridge` package uses it
* Minimal JS bridge only
* Strict separation of concerns

---

# 19. Core Philosophy

This is NOT:

* A virtual DOM framework
* A compiler-driven UI system
* A canvas rendering engine

This IS:

* A signal-driven reactive UI runtime
* With manual dependency declaration
* SSR-first architecture
* DOM-native rendering via minimal JS bridge

---

# 20. Known Gaps & Open Questions

These are design tensions that the current plan acknowledges but does not fully resolve. Each must be addressed before or during the relevant phase.

## 20.1 UseScope — Structural Re-rendering

**Status:** ✅ Resolved. `UseScope` is implemented via `ScopeNode` in `hooks/use_scope.go`, with tree diff in `dom/renderer.go`. The diff uses position-based comparison with node ID reuse. See Section 3 for details.

## 20.2 List Reconciliation

**Status:** ✅ Resolved via position-based structural diff in `ScopeNode` re-renders. Lists within a `UseScope` re-render correctly (additions, removals, reorder via `MutInsertBefore`). The `VirtualList` in `html/html.go` demonstrates this with windowed rendering.

**Limitation:** Position-based matching means shifting all items changes child indices. For most practical cases within a `ScopeNode`, the diff correctly matches elements by position and type.

## 20.3 Renderer Abstraction

**Status:** Separate walkers in `/dom/renderer.go` and `/ssr/renderer.go`. They share the same `Node` types from `/core/node.go` but each walks independently. No shared walker abstraction yet.

## 20.4 Props Bridge (Cross-Component Reactivity)

**Status:** ✅ Both patterns work. Static snapshots for normal args. Signal props via `SignalAccessor` interface — the renderer's `BindingRegistry` automatically subscribes to any `SignalAccessor` passed as a prop value. No wrapper needed.

## 20.5 Event Data Serialization

**Gap:** When JS forwards events to WASM, event data (mouse coordinates, key codes, input value) must be serialized across the boundary. The plan specifies `eventData string` but does not define the schema.

**Proposed approach:** Use a JSON-serialized map for MVP:

```go
type EventData struct {
    Type   string            `json:"type"`
    Target int               `json:"target"` // nodeID
    Data   map[string]any    `json:"data"`   // e.g., {"value": "hello", "key": "Enter"}
}
```

**Open question:** JSON serialization every event has overhead. For high-frequency events (mousemove, scroll), a more efficient binary protocol may be needed — but this is out of scope for MVP.

## 20.6 Component Root Node Tracking

**Gap:** When a component is unmounted (e.g., `if show` becomes false), the framework needs to know which DOM nodes to remove. A component may return an `ElementNode`, a `FragmentNode`, or `nil`. The `ComponentNode.DomNodeID` field tracks the root, but fragments have multiple roots.

**Proposed approach:** Store a `[]int` of root node IDs per component instead of a single `int`. For `ElementNode`, it's a single-element slice. For `FragmentNode`, it's all direct root IDs. For `nil`, it's empty.

## 20.7 Testability of WASM-Dependent Code

**Status:** ✅ Resolved. `Bridge` interface with `NoopBridge` for tests. WASM-only code in `bridge/wasm.go` (guarded by `//go:build js && wasm`). All core, hooks, dom, and ssr tests run with `go test` natively.

## 20.8 ComponentFrame vs ComponentNode Duality

**Gap:** The framework has both `ComponentNode` (in `core/node.go`) — a node type in the VNode tree — and `ComponentFrame` (in `core/component_tree.go`) — a hook state tracking structure. The `ComponentNode` points to a `ComponentFrame` via its `Frame` field. This duality is functional but confusing.

## 20.9 No Stale Frame Cleanup on ComponentNode Re-render

**Gap:** When a `ComponentNode` is rendered again (e.g., inside a `UseScope`), its `Frame` field is overwritten. The old `ComponentFrame`'s hooks may have active subscriptions that are never cleaned up unless the `RunFrameCleanup` walk from the parent `ScopeNode` catches them. This is fragile — if a `ComponentNode` is re-rendered outside a `ScopeNode` (e.g., directly by the DOM renderer), no cleanup runs for the previous frame.

## 20.10 SSR/Meta Path vs Frame Path Consistency

**Gap:** The SSR renderer's path generation in `renderNodeWithMeta` passes the component path through from the caller but does not use the `ComponentFrame.Path` generated by `PushComponent()` inside `ComponentNode` handling (it renders inline without pushing to the frame stack). This means SSR paths may not match client-side frame paths for deeply nested components.
