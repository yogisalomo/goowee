# Spike Divergences

This document captures what was learned during the spike execution that diverges from `plan-v1.md`. Update the plan accordingly before starting production implementation.

---

## D1. Signal Generics vs SignalAccessor Interface

**Plan says:** `Signal[T any]` implements `SignalAccessor` which has `Get() any`.

**Reality:** Go does not allow method overloading. `Signal[T]` cannot have both `Get() T` (typed) and `Get() any` (interface). The compiler rejects this.

**Resolution:** Use a different method name on `SignalAccessor`:

```go
type SignalAccessor interface {
    Subscribe(func()) func()
    Value() any  // was Get() any in the plan
}
```

`Signal[T]` has:
- `Get() T` — type-safe access for users
- `Value() any` — satisfies `SignalAccessor`

Updated files: `plan-v1.md` §4 (SignalAccessor interface), §1 (Signal API).

---

## D2. Binding Auto-Subscription

**Plan says:** `BindingRegistry.Bind()` registers the binding; subscription is separate.

**Reality:** For the reactive path to work, `Bind()` must also subscribe to the signal so that signal changes trigger mutation generation. In production, this subscription should enqueue through the scheduler rather than firing eagerly.

**Resolution:** `Bind()` calls `sig.Subscribe(...)` and routes through the mutation queue. `Unbind()` returns the unsubscribe token and calls it.

Updated files: `plan-v1.md` §4 (Bind method behavior).

---

## D3. AppendChild Requires Both Parent and Child IDs

**Plan says:** `MutAppendChild` just identifies the parent.

**Reality:** The mutation must specify both parent and child node IDs so the JS bridge knows which child to attach to which parent.

**Resolution:** Keep `Mutation.NodeID` as the parent and add the child ID via `Value`:

```go
{Type: MutAppendChild, NodeID: parentID, Value: fmt.Sprintf("%d", childID)}
```

Or add a dedicated `ChildID int` field to `Mutation`.

Updated files: `plan-v1.md` §6 (Mutation struct).

---

## D4. Render Pipeline Must Return Node IDs

**Plan says:** DOM renderer walks Node tree and emits mutations.

**Reality:** The walker must return the generated node ID for each subtree so parent nodes can emit correct `AppendChild` mutations. The plan's pseudocode didn't show this.

**Resolution:** Render functions return `int` (the root node ID):

```go
func (r *DOMRenderer) renderNode(n Node, muts *[]Mutation) int
```

This is already reflected in `dom_renderer.go` from the spike.

Updated files: `plan-v1.md` §4 (Rendering Pipeline code).

---

## D5. UseScope Diff Is Non-Trivial

**Plan says:** UseScope performs a minimal structural diff with 4 rules.

**Reality:** The diff works for basic cases but revealed subtle issues:
- When both old and new `TextNode` reference the **same `*Signal`**, the diff sees them as equal references and doesn't detect value changes. The framework must compare resolved values, not node references.
- Node ID generation during diff requires a separate allocator or reuse strategy — you can't reuse the DOM renderer's sequential IDs because the diff walks both trees simultaneously.
- The diff must handle `nil` carefully (ElementNode → nil emits remove, nil → ElementNode emits create).

**Resolution:** Update UseScope design:
- Compare resolved values (`getValue(v)`) for diffing, not raw node references.
- Use a dedicated node ID allocator per diff pass.
- Keep the minimal rule set (already correct).

Updated files: `docs/component_tree.md` (re-render section), `plan-v1.md` §3 (UseScope).

---

## D6. SSR Path Format

**Plan says:** Component paths use index format: `root/counter/0`.

**Spike used:** `root/greeting/child` (appending `/child` for children).

**Resolution:** Use index-based paths as the plan specifies (`root/0/1`) — this is more stable for re-render resolution because it doesn't depend on element names.

Updated files: `ssr_renderer.go` (path generation in spike → production).

---

## D7. Event Handler Filtering

**Plan says:** DOM renderer registers event handlers via `NodeRegistry`.

**Spike validated:** The renderer correctly skips `func(EventData)` values when emitting mutations. No mutation is emitted for `onclick` or similar props.

**Resolution:** No change needed — this works as designed. Add test coverage for this in production.

---

## D8. Unmount Must Track Multiple Root IDs

**Plan's Known Gap §20.6** flagged this: `ComponentNode` needs `[]int` for root IDs, not a single `int`.

**Spike confirmed:** A component returning a `FragmentNode` with 3 children has 3 root DOM nodes. Unmounting must remove all 3.

**Resolution:** The `RootNodeIDs []int` in `component_tree.md` is correct as already updated.

---

## D9. Node Type Stringer

**Plan says:** `Node` interface has only `nodeMarker()`.

**Spike added:** `fmt.Stringer` to `Node` for debugging. Useful for tests and logging.

**Resolution:** Add `String() string` to the `Node` interface in production too — it's invaluable for debugging render output.

Updated files: `plan-v1.md` §3 (Node type definition).

---

## Summary of Plan Changes Required

| Divergence | File to Update | Change |
|---|---|---|
| D1 | `plan-v1.md` §1, §4 | `SignalAccessor` method: `Value() any` instead of `Get() any` |
| D1 | `plan-v1.md` §1 | Show both `Get() T` and `Value() any` on `Signal[T]` |
| D2 | `plan-v1.md` §4 | `Bind()` auto-subscribes; subscriptions route through scheduler |
| D3 | `plan-v1.md` §6 | `AppendChild` mutation carries child ID |
| D4 | `plan-v1.md` §4 | `renderNode()` returns `int` (node ID) |
| D5 | `plan-v1.md` §3 | UseScope: compare resolved values, separate ID allocator per diff |
| D6 | `plan-v1.md` §8 | Index-based child paths (`root/0/1`) |
| D7 | `plan-v1.md` §4 | No change needed — already correct |
| D8 | `plan-v1.md` §20.6 | Already resolved as `RootNodeIDs []int` |
| D9 | `plan-v1.md` §3 | Add `String() string` to Node interface |
