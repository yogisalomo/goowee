# Component Tree Design

## Motivation

The framework needs a way to track component identity across renders. A flat `currentComponentID` counter is insufficient because:

- Components mount and unmount dynamically (conditionals, lists).
- Re-renders must resolve to the same hook state slots.
- Unmount requires walking descendants to unbind signals and remove DOM nodes.
- Hydration needs a stable path from server HTML to client component instance.

The `ComponentNode` tree solves all of these with a single, lightweight structure.

---

## ComponentNode

```go
// ComponentNode represents a single component instance in the tree.
type ComponentNode struct {
    // Path is a unique slash-separated string identifying this instance
    // within the tree, e.g., "root/counter/0" or "root/todo-list/2/todo-item/1".
    Path string

    // Parent links to the enclosing component (nil for root).
    Parent *ComponentNode

    // Children is an ordered list of child component instances.
    Children []*ComponentNode

    // RootNodeIDs tracks the root DOM node IDs for this component.
    // Single element for ElementNode returns, multiple for FragmentNode,
    // empty for nil returns. Used for unmount cleanup and hydration matching.
    RootNodeIDs []int

    // Holds hook state per call position.
    // Index 0 → first UseState/UseEffect call, index 1 → second, etc.
    Hooks []any
}
```

---

## Render Stack

A global stack maintains the current component path during rendering:

```go
var (
    renderStack   []*ComponentNode
    nextComponent int // monotonic counter for unique IDs
)
```

### Push

When a component function begins executing, a new `ComponentNode` is pushed:

```go
func pushComponent() *ComponentNode {
    parent := currentComponent()
    path := childPath(parent, nextComponent)
    nextComponent++
    node := &ComponentNode{
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

When the component function returns, the stack is popped:

```go
func popComponent() {
    renderStack = renderStack[:len(renderStack)-1]
}
```

### Current

Hooks call `currentComponent()` to read/write their slot:

```go
func currentComponent() *ComponentNode {
    if len(renderStack) == 0 {
        panic("hooks: called outside component context")
    }
    return renderStack[len(renderStack)-1]
}
```

### Path Generation

Paths are built by appending the child index to the parent path:

```
root                  → "root"
root/counter          → "root/0"
root/counter/button   → "root/0/0"
```

On re-render, the same path is resolved by walking the tree. If a matching path does not exist, the component is mounting for the first time.

---

## Re-render Resolution

When a component re-renders (triggered by a signal change):

1. The framework knows the component's path (from the signal's subscriber list or effect registration).
2. It walks from the tree root following path segments.
3. The existing `ComponentNode` is found — its `Hooks` slice is reused.
4. `pushComponent()` reuses the existing node (does not create a new one) by matching the path during render walk.

This means the tree is mutated in place during re-renders, not rebuilt from scratch. Only new mount points create new `ComponentNode` instances.

---

## Unmount Lifecycle

When a component is removed (e.g., conditional disappears):

1. The parent component re-renders without the child.
2. The framework detects the old `ComponentNode` is no longer reachable.
3. It walks the subtree:
   - Calls cleanup functions from all `Effect` hooks.
   - Calls `BindingRegistry.Unbind(nodeID)` for every DOM node in the subtree.
   - Calls `NodeRegistry` cleanup to remove event handlers.
   - Removes the subtree from the parent's `Children` slice.
4. The JS bridge receives `MutRemoveNode` mutations for all affected DOM nodes.

---

## Hydration Matching

During hydration, the client replays component rendering to reconstruct the `ComponentNode` tree. Hydration metadata uses component paths (not signal IDs), so the client can match:

| Server                          | Client                           |
|---------------------------------|----------------------------------|
| Renders component at path `root/0/1` | Renders component at path `root/0/1` |
| Outputs `<div data-node-id="5">`     | Creates `ComponentNode{Path: "root/0/1", RootNodeIDs: []int{5}}` |
| Hydration meta: `nodeID 5 → SlotRef{ComponentPath: "root/0/1", HookIndex: 0}` | Finds `ComponentNode` at path `root/0/1`, reads `Hooks[0]`, subscribes signal to DOM node ID 5 |

Paths are deterministic because both server and client execute the same component code with the same conditional logic.

---

## Tree Walk Example

Given this component structure:

```
App (root)
 ├─ Header (root/0)
 └─ Counter (root/1)
     └─ Button (root/1/0)
```

The `ComponentNode` tree:

```
ComponentNode{Path: "root", RootNodeIDs: []int{1}}
  ├─ ComponentNode{Path: "root/0", RootNodeIDs: []int{2}, Parent: root}
  └─ ComponentNode{Path: "root/1", RootNodeIDs: []int{3}, Parent: root}
       └─ ComponentNode{Path: "root/1/0", RootNodeIDs: []int{4}, Parent: root/1}
```

---

## Integration Points

| System                 | How it uses ComponentNode                                      |
|------------------------|----------------------------------------------------------------|
| Hook layer             | `currentComponent().Hooks[slotIndex]` to store/retrieve state  |
| Effect system          | Stores cleanup function in hook slot; runs on unmount subtree walk |
| DOM binding            | `RootNodeIDs` map to `BindingRegistry` entries; `Unbind(nodeID)` on unmount for each |
| Event system           | `NodeRegistry` entries tied to `RootNodeIDs`; cleaned up on unmount |
| Hydration              | Reconstructs tree at render time, matches server paths for signal subscription |
| Scheduler              | Triggered by signal updates from hooks; flushes batched mutations |

---

## Key Properties

- **O(1) hook access** — direct slice index, no map lookup per call.
- **O(depth) path resolution** — path segments are walked; depth is typically < 10.
- **Deterministic paths** — same code → same tree shape → same paths.
- **Scope-safe** — each component only sees its own `Hooks` slice via the stack pointer.
- **No global ID collision** — monotonic counter + tree-local indexing.
