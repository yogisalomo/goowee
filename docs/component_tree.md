# Component Tree & Lifecycle Design

## The model: components run once (Solid-style)

Goowee is **not** React. A component function is a **setup function that runs
exactly once, when the component mounts**. It is not re-executed on updates.
State lives in signals created during setup and captured by the closures
(event handlers, scopes, bindings) in the returned tree:

```go
func Counter() core.Node {
    return core.Component("Counter", func() core.Node {
        count, _ := hooks.UseState(0) // created once, on mount
        return h.Button(
            h.OnClick(func() { count.Update(func(c int) int { return c + 1 }) }),
            h.Textf("Count: %d", count), // reacts to count via a binding
        )
    })
}
```

Because the function never re-runs, there are **no hook rules**: no
"call hooks in the same order every render", no "no conditionals before
hooks". `UseState` just creates a signal; call it wherever you like. Updates
happen at a finer grain than the component:

- **Bindings** — a signal bound to a prop/attr/text updates that one node
  when the signal changes (see `BindingRegistry`).
- **Scopes** — a `ScopeNode` (from `UseScope`, or the `h.Show`/`h.For`/
  `h.Switch` helpers) re-runs a small render closure when its deps change and
  diffs the result.

> Historical note: earlier docs described a React-style hook-slot model where
> components re-run and `UseState` resolves state by call position. That was
> never implemented — `UseState` always created fresh state. The framework is
> committed to the run-once model; the machinery below reflects that.

There are two distinct concepts:

1. **`ComponentNode`** — a node in the tree (`core/node.go`) that wraps a
   component's setup function. It is the unit of mount/unmount identity.
2. **`ComponentFrame`** — the **ownership/disposal scope** for one mounted
   component (`core/component_tree.go`). Everything created during setup that
   needs cleanup (effects, `Computed`/`Watch` subscriptions) is registered on
   the frame; unmounting the component disposes the frame.

---

## RenderContext (no globals)

The frame stack lives on a `RenderContext`, not a package global, so
concurrent server renders never share it:

```go
type RenderContext struct {
    Env   Env // EnvClient or EnvServer
    stack []*ComponentFrame
}
```

- The **client** (single-threaded WASM) runs on a default `EnvClient` context.
- Each **server** render runs under its own `EnvServer` context via
  `core.UseContext(ctx, fn)` (serialized), so requests can't corrupt each
  other's frame stack, and a panic mid-render can't leak into the next one.

`PushComponent` / `PopComponent` / `CurrentComponent` operate on the active
context; `CurrentEnv()` reports client vs server. Effects (`UseEffect`,
`OnMount`, `Watch`) are **no-ops under `EnvServer`** — the server produces HTML
only and must not start goroutines or subscriptions.

---

## ComponentFrame

```go
type ComponentFrame struct {
    Path        string            // "/", "/0", "/0/1" — position in the frame tree
    Parent      *ComponentFrame
    Children    []*ComponentFrame
    RootNodeIDs []int             // root DOM node ids of this component
    Hooks       []any             // effect states (for cleanup)
    Disposers   []func()          // Computed/Watch subscription teardowns
}
```

The frame is a **disposal scope**, not a hook-slot store. `RegisterDisposer`
attaches a teardown to the current frame; `RunFrameCleanup` runs a single
frame's disposers and effect cleanups (it does **not** recurse into child
frames — see disposal below).

---

## Mounting a component

`FlatTree` collapses nested fragments into their parent's child list but
leaves `ComponentNode` and `ScopeNode` **intact** — they are reconciled
lazily, not expanded at build time. When the renderer reaches a
`ComponentNode` it mounts it exactly once:

```
1. PushComponent()            → new frame, child of the current frame
2. inner := FlatTree(v.Render())   → run setup once (hooks register on the frame)
3. renderNode(inner)          → materialize the output; nested components/scopes
                                register against this frame while it's on the stack
4. PopComponent()
5. v.Frame = frame; v.Prev = inner  → remember the frame + rendered output
```

After this, the component's output is a **stable, self-updating subtree**: its
reactive parts (bindings, inner scopes) update themselves through their own
subscriptions. The component function does not run again.

---

## ScopeNode & reconciliation

A `ScopeNode` re-renders its closure when a dep changes and diffs the new tree
against the previous one (`ScopeNode.Prev`). The diff treats each node type by
identity:

- **Elements** — matched by tag; attrs/props/binds/handlers/children
  reconciled; node ids reused. Keyed children (`ElementNode.Key`) are matched
  by key.
- **Components** — matched by `Name`. A **matched component is preserved**:
  the renderer reuses its frame and output and does **not** re-run setup, so
  its state and effects survive. A component that is **added** mounts (setup
  runs once); one that is **removed** unmounts (frame disposed).

This is the key property: **a component nested in a scope keeps its identity
even when the scope re-renders for an unrelated reason.** (Previously every
scope re-render tore down and re-ran all nested components, losing their state
and churning their effects.)

Components in a list are matched positionally by `Name` today; keyed matching
applies to elements. Rows that are whole components therefore rely on stable
ordering.

---

## Unmounting & disposal

When a subtree is removed (`emitRemoveTree`), the renderer first walks it with
`disposeReactive`, then emits the DOM removals:

- **ComponentNode** → recurse into its output, then `RunFrameCleanup(frame)`
  (its effects + `Computed`/`Watch` disposers).
- **ScopeNode** → cancel its dep subscriptions (`Unsubs`).
- **Element/Fragment** → recurse into children.

Because the walk visits every component in the removed subtree and
`RunFrameCleanup` cleans exactly one frame, each frame is disposed once (no
double-cleanup, no missed nested frames).

---

## Lifecycle primitives (`hooks`)

| Primitive | When it runs | Cleanup |
|---|---|---|
| `UseState(initial)` | creates a signal once, on mount | — |
| `OnMount(fn func() func())` | once, on mount | the returned func, on unmount |
| `Watch(deps, fn)` | fn on each dep change (not on mount) | subscriptions, on unmount |
| `UseEffect(deps, fn func() func())` | body on mount + on each dep change | before each re-run and on unmount |
| `UseScope(fn, deps...)` | fn on mount + on each dep change; diffs output | scope subscriptions, on unmount |

Use `OnMount` for resources tied to the component's lifetime (timers,
goroutines, external subscriptions) — its cleanup is what stops them on
unmount. Use `Watch` for plain "when these signals change, react". Prefer the
`h.Show`/`h.ShowElse`/`h.Switch`/`h.For` control-flow helpers over raw
`UseScope`.

---

## Key properties

- **Run-once components** — setup executes once per mount; no hook-order rules.
- **Fine-grained updates** — bindings and scopes update without re-running the
  component.
- **Stable identity** — matched components are preserved across scope
  re-renders (state + effects intact).
- **Frame = ownership scope** — everything created in setup that needs cleanup
  is disposed together on unmount.
- **Request-safe SSR** — per-render `RenderContext`; effects don't run on the
  server.
