# Component Tree Design

## Motivation

The framework needs a way to track component identity across renders and provide hook state isolation. A flat `currentComponentID` counter is insufficient because:

- Components mount and unmount dynamically (conditionals, lists).
- Re-renders must resolve to the same hook state slots.
- Unmount requires walking descendants to unbind signals and remove DOM nodes.
- Hydration needs a stable path from server HTML to client component instance.

There are two related but distinct concepts:

1. **`ComponentFrame`** — hook state tracking structure (in `core/component_tree.go`), used during render to isolate `UseState`/`UseEffect` state.
2. **`ComponentNode`** — a VNode type in the tree (in `core/node.go`), a node that wraps a component function for re-execution.

---

## ComponentFrame (Hook State)

```go
// ComponentFrame represents a single component instance in the tree,
// used for hook state isolation.
type ComponentFrame struct {
    // Path is a unique slash-separated string, e.g., "root/0/1".
    Path string

    // Parent links to the enclosing component (nil for root).
    Parent *ComponentFrame

    // Children is an ordered list of child component instances.
    Children []*ComponentFrame

    // RootNodeIDs tracks the root DOM node IDs for this component.
    // Used for unmount cleanup.
    RootNodeIDs []int

    // Holds hook state per call position.
    // Index 0 → first UseState/UseEffect call, index 1 → second, etc.
    Hooks []any
}
```

---

## Render Stack

A global stack maintains the current component frame during rendering:

```go
var renderStack []*ComponentFrame
```

### Push

When `core.Component(name, fn)` or `FlatTree` encounters a `ComponentNode`, it pushes a new frame:

```go
func PushComponent() *ComponentFrame {
    parent := CurrentComponent()
    path := "/"
    if parent != nil {
        path = parent.Path + "/" + itoa(len(parent.Children))
    }
    node := &ComponentFrame{
        Path:   path,
        Parent: parent,
    }
    if parent != nil {
        parent.Children = append(parent.Children, node)
    }
    renderStack = append(renderStack, node)
    return node
}
```

### Pop

```go
func PopComponent() {
    renderStack = renderStack[:len(renderStack)-1]
}
```

### Current

Hooks call `CurrentComponent()` to read/write their slot:

```go
func CurrentComponent() *ComponentFrame {
    if len(renderStack) == 0 {
        return nil
    }
    return renderStack[len(renderStack)-1]
}
```

Note: `CurrentComponent()` returns `nil` when called outside a component context, allowing hooks to work (with un-tracked state) outside components.

### Path Generation

Paths are built by appending the child index to the parent path:

```
/           → root (first component)
/0          → first child of root
/0/1        → second child of first child
```

---

## ComponentNode (VNode Type)

`ComponentNode` is a node in the VNode tree (not to be confused with `ComponentFrame`). It wraps a component name and render function:

```go
type ComponentNode struct {
    Name   string
    Render func() Node     // called each render
    Prev   Node            // last rendered inner tree (set by renderer)
    Frame  *ComponentFrame // frame from last render (set by renderer)
}

func Component(name string, render func() Node) *ComponentNode
```

During rendering (in both DOM renderer and `FlatTree`), encountering a `ComponentNode` triggers:
1. `PushComponent()` — creates a new `ComponentFrame`
2. Call `v.Render()` — executes the component function, which may call hooks
3. Recurse into the returned inner tree
4. `PopComponent()` — pops the frame
5. Store the `ComponentFrame` reference in `v.Frame`

---

## ScopeNode (Structural Re-rendering)

`ScopeNode` is the mechanism for structural updates. It is returned by `UseScope`:

```go
type ScopeNode struct {
    Render func() Node
    Deps   []SignalAccessor
    Prev   Node              // expanded tree from last render
    Frames []*ComponentFrame // component frames from last render (for cleanup)
    Unsubs []func()          // signal subscription cancellations
}
```

During first render:
1. `Render()` is called to produce a tree
2. `FlatTreeWithFrames()` expands components → produces a flat tree + collects `ComponentFrame` pointers
3. The flat tree is rendered to DOM (node IDs assigned, bindings set up)
4. Signals in `Deps` are subscribed to trigger `reRenderScope()`

During re-render (signal change):
1. `Render()` is called again
2. `FlatTreeWithFrames()` produces new flat tree + frames
3. `diffNode(Prev, newTree)` compares old and new trees, emits mutations
4. Old `ComponentFrame` list is walked with `RunFrameCleanup()` to clean up effects
5. `Prev` is updated to the new tree

---

## RunFrameCleanup (Unmount Cleanup)

When a component is removed by the diff (old frames no longer in new frames list):

```go
func RunFrameCleanup(frame *core.ComponentFrame) {
    for _, hook := range frame.Hooks {
        if es, ok := hook.(*effectState); ok {
            for _, unsub := range es.Unsubs {
                if unsub != nil { unsub() }
            }
            es.Unsubs = nil
            if es.Cleanup != nil {
                es.Cleanup()
                es.Cleanup = nil
            }
        }
    }
    for _, child := range frame.Children {
        RunFrameCleanup(child)
    }
}
```

This recursively walks the frame tree, calling effect cleanup functions and unsubscribing from signals.

---

## Re-render Resolution

Re-renders are driven by `ScopeNode` signal subscriptions:

1. A signal in `ScopeNode.Deps` changes.
2. The subscriber callback calls `reRenderScope()`.
3. `reRenderScope()` re-runs the render closure, diffs old vs new tree, enqueues mutations, runs frame cleanup for removed frames.
4. The diff matches nodes positionally (by index in children arrays) and by tag name.
5. Existing node IDs are reused (e.g., `newElement.ID = oldElement.ID`).

This is NOT a path-based re-resolution of individual components. Instead, the `ScopeNode` re-renders its entire subtree and diffs the before/after trees.

---

## Tree Walk Example (DOM Renderer)

Given this component structure used in `Component()`:

```
Component("App")                → Frame path: "/"
 ├─ router.Route(routeFn)       → Frame path: "/0"
 │    └─ counterPage()          → Frame path: "/0/0"
 │         ├─ UseState(count)   → Hooks[0]
 │         └─ UseState(show)    → Hooks[1]
 └─ VirtualList(todos, fn)     → Frame path: "/1"
      └─ Component("VirtualList")
```

The `ComponentFrame` tree:

```
ComponentFrame{Path: "/", Hooks: [count, show]}
  └─ ComponentFrame{Path: "/0", Hooks: []}
       └─ ComponentFrame{Path: "/0/0", Hooks: []}
```

---

## Integration Points

| System                 | How it uses ComponentFrame                                   |
|------------------------|--------------------------------------------------------------|
| Hook layer             | `CurrentComponent().Hooks[slotIndex]` to store/retrieve state |
| Effect system          | Stores cleanup in hook slot; `RunFrameCleanup` on unmount    |
| DOM binding            | `RootNodeIDs` map to `BindingRegistry` entries               |
| Event system           | `NodeRegistry` entries tied to root node IDs                 |
| Hydration              | Component paths in SSR metadata match frame paths            |
| ScopeNode diff         | Tracks old frames for cleanup after re-render                |

---

## Key Properties

- **O(1) hook access** — direct slice index, no map lookup per call.
- **O(depth) path generation** — path segments are built incrementally.
- **Deterministic paths** — same code → same tree shape → same paths.
- **Scope-safe** — each component only sees its own `Hooks` slice via the stack pointer.
- **Frame-per-render** — each render pass creates new `ComponentFrame` instances; old frames are cleaned up by `RunFrameCleanup`.
