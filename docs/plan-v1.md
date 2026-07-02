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
func Effect(deps []*Signal, fn func() func())
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
func UseEffect(deps []*Signal, fn func() func())
```

## State Isolation

Each component instance has isolated hook state and is represented by a `ComponentNode`. The mechanism:

1. A global render stack tracks the current component tree path. Each component function pushes a `ComponentNode` onto the stack before executing.
2. Each `ComponentNode` holds a `Hooks []any` slice — one slot per `UseState`/`UseEffect` call.
3. `UseState` and `UseEffect` read/write the next slot in the current component's hook slice.
4. On re-render, the component path (e.g., `root/counter/0`) resolves to the same `ComponentNode`, so hooks read from the same slots.

This is the same model React uses (hooks as an ordered list indexed by call position). Deterministic call order is required.

## Rules

* Deterministic call order required — no conditionals before hook calls
* Each component instance has isolated hook state
* Hooks must be called at the top level of a component function, never inside loops or conditions

---

# 3. Component Model

* Pure functional components only
* No classes, no struct components required initially
* Must be deterministic

Example:

```go
func Counter() Node
```

Supports:

* Composition
* Props structs
* Nested components

## Node Type

Every component returns a `Node`. This type is the shared contract between the DOM renderer, SSR renderer, and component authors. It must be defined before any renderer is built:

```go
// Node is the universal return type for all components.
type Node interface {
    nodeMarker()
    String() string // for debugging render output
}

// ElementNode represents a DOM element.
type ElementNode struct {
    Tag      string
    Props    map[string]any   // string → string | SignalAccessor | EventHandler | ...
    Children []Node
}

// TextNode represents a text leaf. Value may be a literal string
// or a *Signal[string] for reactive text.
type TextNode struct {
    Value any // string | *Signal[string]
}

// FragmentNode groups multiple children without a wrapper element.
type FragmentNode struct {
    Children []Node
}
```

This type enables the rendering pipeline (see Section 3.3) and keeps the DOM and SSR renderers in sync.

## Component Identity

Each component instance is tracked by a `ComponentNode` in a lightweight tree. During render, components push their `ComponentNode` onto a global stack, establishing a path like `root/counter/0` that survives across renders. This path is used for hook isolation, unmount cleanup, and hydration matching.

See [`docs/component_tree.md`](component_tree.md) for the full design.

## Component Re-execution Model

This is a **design tension** that must be resolved before implementation:

The plan says "pure functional components" and "no full tree re-render." But structural changes (e.g., a signal controlling an `if` condition) require the component to re-execute and produce a new `Node` tree. The `Effect` hook alone does not re-call the component function.

**Proposed approach:** A `UseScope` primitive that re-executes a render closure when its signal dependencies change:

```go
func UseScope(deps []*Signal, render func() Node) Node
```

Usage:

```go
func Greeting() Node {
    show, setShow := UseState(true)
    name, setName := UseState("world")
    return UseScope([]*Signal{show}, func() Node {
        if !show.Get() {
            return nil
        }
        return dom.Element("div", dom.Props{},
            dom.Text(name.Get()),
        )
    })
}
```

`UseScope` registers the render closure. On dependency change, it re-runs the closure and diffs old vs new `Node` trees, emitting only the structural mutations needed.

### Tree Diff Rules

The diff compares old and new trees and produces mutations. It uses a dedicated node ID allocator per diff pass (not the DOM renderer's sequential IDs):

| Old child | New child | Action |
|---|---|---|
| `ElementNode` | `ElementNode`, same tag | Walk children recursively, emit per-property diffs |
| `ElementNode` | `nil` | Emit `MutRemoveNode` |
| `nil` | `ElementNode` | Recursively emit create + append for the new subtree |
| `TextNode` | `TextNode` | Compare resolved values via `Value()`, emit `MutSetProperty` if different |

**Key detail:** Comparisons use resolved values (calling `SignalAccessor.Value()`) rather than raw node references. This handles the common case where both old and new `TextNode` reference the same `*Signal` — the diff compares the resolved values (e.g., `"loading"` vs `"done"`) rather than the pointer identity.

For the MVP, **structural updates via `UseScope` are deferred**. The initial counter example has no conditional rendering, so Phase 1-2 only need property-level reactivity. `UseScope` is added in Phase 4.

## Props Flow

Props must bridge the gap between a parent re-rendering and a child receiving new values. Two options exist:

### Option A: Static Snapshots (MVP)

Props are plain values, passed at call time:

```go
func App() Node {
    count, setCount := UseState(0)
    return Child(ChildProps{Value: count.Get()}) // snapshot
}
```

**Limitation:** If `count` changes, `Child` won't re-render because it received a static value. The parent would need `UseScope` (see above) to re-call `Child` on change.

### Option B: Signal Props (Preferred)

Props accept signals directly:

```go
func Child(props ChildProps) Node {
    return dom.Element("div", dom.Props{},
        dom.NewText(props.Count), // *Signal[int] → reactive binding
    )
}

// Parent passes the signal, not the value:
func App() Node {
    count, setCount := UseState(0)
    return Child(ChildProps{Count: count})
}
```

**For MVP,** use Option A (static snapshots) for simplicity. The counter example only needs one component. Option B is added when multi-component signal propagation is needed (Phase 4).

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

The DOM renderer walks the `Node` tree produced by a component and emits an initial batch of mutations. This is a one-time tree-to-DOM construction, not a diff:

```go
type DOMRenderer struct {
    NodeRegistry   *NodeRegistry
    BindingRegistry *BindingRegistry
    nextNodeID     atomic.Int64
}

func (r *DOMRenderer) Render(node Node) []Mutation {
    var muts []Mutation
    r.renderNode(node, &muts)
    return muts
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
    - If value is string → emit MutSetAttribute(nodeID, key, val)
    - If value is SignalAccessor → BindingRegistry.Bind(nodeID, signal, key)
    - If value is EventHandler → NodeRegistry.RegisterHandler(nodeID, eventType, handler)
  - Recurse into children:
    - Emit MutAppendChild(parentNodeID, childNodeID)
    - Process child subtrees
For each TextNode:
  - If value is string → emit MutCreateElement(nodeID, "#text") + MutSetProperty(nodeID, "textContent", val)
  - If value is *Signal → same but also BindingRegistry.Bind(nodeID, signal, "textContent")
    ↓
Mutation queue flushed via scheduler
    ↓
JS bridge applies mutations to real DOM
```

This is the **only** path that creates DOM nodes. After initial render, all updates flow through signal → binding → mutation, never through a full re-render.

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
)
```

## Mutation struct

```go
type Mutation struct {
    Type    MutationType
    NodeID  int    // target node (or parent for MutAppendChild)
    Key     string // property name, attribute name, or "tag" for MutCreateElement
    Value   string // serialized value, JSON-compatible
    ChildID int    // used only by MutAppendChild — the child node ID
}
```

## Supported operations:

| MutationType    | Description                              |
|-----------------|------------------------------------------|
| MutCreateElement | Create a new DOM element               |
| MutRemoveNode    | Remove a DOM node                      |
| MutSetAttribute  | Set an HTML attribute on a DOM node    |
| MutSetProperty   | Set a JS property (e.g., textContent, checked, value, className, disabled) |
| MutAppendChild   | Append a child node to a parent        |

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

Signal IDs are not stable across process boundaries (server → client). Instead, the metadata uses the component's tree path and hook slot index:

```go
type SlotRef struct {
    ComponentPath string // e.g., "root/counter/0"
    HookIndex     int    // which hook call in the component
}

type HydrationMeta struct {
    NodeMap map[int]string       // nodeID → component path
    Deps    map[int][]SlotRef    // nodeID → []{componentPath, hookIndex}
}
```

At hydration time on the client:
1. WASM boots and re-renders the component tree, constructing `ComponentNode` instances with matching paths.
2. The hydration system reads the `<script id="hydration-meta">` JSON.
3. For each `(nodeID → slotRef)` entry, it walks the component tree to find the `ComponentNode` at `componentPath`, reads the signal from the `Hooks[hookIndex]` slot, and subscribes it to the DOM node.

This is stable across server and client because the component hierarchy and hook call order are deterministic.

The metadata is embedded in a `<script type="application/json" id="hydration-meta">` tag alongside the HTML.

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

## Behavior

* Match DOM nodes via node IDs (from `data-node-id` attributes)
* Attach signals after SSR render using the hydration metadata
* Recover gracefully from mismatches (NO hard failure)

## Mismatch Handling

When SSR HTML differs from what the client-side component would render:

1. Log a warning to the console via JS bridge.
2. Force a full re-render of the mismatched subtree.
3. Continue hydrating the rest of the tree normally.

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
    signal.go
    effect.go
    scheduler.go
    binding_registry.go
    component_tree.go
    node.go          // Node, ElementNode, TextNode, FragmentNode types

/hooks
    use_state.go
    use_effect.go

/dom
    renderer.go
    bindings.go
    mutations.go
    node_registry.go

/ssr
    renderer.go
    hydration.go

/bridge
    js.go
    events.go
    bootstrap.go

/examples
    counter/
    dashboard/
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

## Must implement:

* Signal system
* UseState / UseEffect hooks
* Basic DOM renderer
* Frame batching system
* JS bridge (minimal)
* SSR HTML renderer
* Hydration system
* Event handling (click → update → DOM update)

## Example app:

* Counter that increments on click
* SSR renders initial HTML
* Hydration activates interactivity
* Updates only patch affected DOM nodes

---

# 15. Phased Rollout

| Phase | Scope                                      | Exit Criterion                                                        |
|-------|--------------------------------------------|-----------------------------------------------------------------------|
| 1     | Signal system + scheduler + hooks + component tree (pure Go)| Signal + scheduler + component tree unit tests pass; a UseState + Effect wiring test increments a counter variable and asserts the value with a mock observer (no DOM dependency) |
| 2     | DOM binding + JS bridge + mutations        | Counter renders in browser; click updates DOM                         |
| 3     | SSR renderer + hydration system            | SSR produces HTML; hydration activates counter                        |
| 4     | Hooks layer polish + integration tests     | UseState/UseEffect API works with counter example end-to-end          |

Each phase is a checkpoint. If a phase reveals unexpected complexity, adjust scope before moving to the next.

---

# 16. Explicit Non-Goals

DO NOT implement:

* Virtual DOM diffing engine
* Automatic dependency tracking
* Compiler-based optimizations
* Routing system
* Animation system
* CSS-in-Go system
* Complex layout engine
* List reconciliation with keys (dynamic lists use static position-based identity; removing the first item shifts all subsequent items' hook state — this is acceptable for MVP but must be addressed for any real application)

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

**Gap:** The plan lacks a mechanism for components to re-execute when their signals change. `Effect` re-runs a callback but does not re-call the component function or produce a new `Node` tree. Without this, conditional rendering (`if show { ... }`) cannot react to signal changes.

**Proposed solution:** `UseScope(deps []*Signal, render func() Node) Node` (see Section 3). Deferred to Phase 4. For Phase 1-2, the MVP counter avoids structural changes entirely — only property-level reactivity via direct signal bindings.

**Open question:** Should `UseScope` perform a minimal tree diff on re-execution, or should it tear down and rebuild the subtree? The former is more efficient but introduces diffing complexity. The latter is simpler but loses non-signal state in the subtree.

## 20.2 List Reconciliation

**Gap:** Component identity uses a positional index in the parent's children list. Removing the first item in a dynamic list shifts all subsequent indices, causing hook state to misalign with the wrong items.

**Proposed solution:** Defer until list rendering is needed. For the MVP counter, no lists exist. When lists are added, either:

- Adopt a React-style `key` prop with reconciliation, or
- Require developers to use a `for`-loop with stable indices (fragile but simple)

**Open question:** Positional identity is used for hook slot allocation. If a key-based reconciliation is added later, the `ComponentNode` path generation must incorporate keys (e.g., `root/0/key:abc/1` instead of `root/0/0/1`).

## 20.3 Renderer Abstraction Timeline

**Gap:** The DOM renderer (`/dom/renderer.go`) and SSR renderer (`/ssr/renderer.go`) both consume `Node` trees but currently have no shared walker. This risks duplicated logic and divergent behavior.

**Proposed solution:** Start with separate walkers for MVP. If property-binding logic or node-type handling diverges, extract a shared walker that emits events consumed by both backends.

**Open question:** Does the shared walker live in `/core` or a new `/render` package? The former avoids another top-level package; the latter keeps concerns separated cleanly.

## 20.4 Props Bridge (Cross-Component Reactivity)

**Gap:** If a parent passes a static value as a child's prop, the child cannot react to changes. If a parent passes a signal, the child must know to call `.Get()` and subscribe. Neither pattern is enforced by the type system.

**Proposed solution:** For MVP, use static snapshots (Option A, Section 3). When multi-component signal propagation is needed, adopt signal props (Option B) and optionally a `From` helper that automatically subscribes in `UseScope`.

**Open question:** Should props be typed generically (`Signal[T]`) or accept any `SignalAccessor`? The former is type-safe but verbose; the latter is flexible but requires runtime type assertions.

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

**Gap:** The JS bridge and event dispatch depend on `syscall/js`, which requires `GOOS=js GOARCH=wasm` to compile. This makes fast unit-test feedback loops impossible for the bridge layer.

**Proposed approach:** Define a `Bridge` interface in `/bridge` that the rest of the framework depends on. Provide a no-op implementation for unit tests and a real implementation backed by `syscall/js` for WASM builds. This allows all non-WASM code (signal, scheduler, hooks, renderer, SSR) to be unit-tested with `go test` natively.
