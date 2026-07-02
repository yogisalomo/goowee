# Half-Day Spike Plan

## Goal

Validate the six highest-risk design decisions before committing to the full implementation. The spike produces runnable Go code (pure Go, no WASM) that proves the core pipeline works end-to-end.

## Risk Surface

| Risk | Why It Matters | Spike Coverage |
|------|---------------|----------------|
| Node type design | Both renderers depend on it; wrong abstraction forces rewrite of both | Define it, consume it in both renderers |
| Rendering pipeline | Walk to mutations vs walk to HTML — these are the only output paths | Implement both from the same Node tree |
| Component re-execution | No mechanism exists for structural changes; UseScope API may be wrong | Prototype UseScope with a conditional render |
| Signal → binding → mutation flow | This is the core reactivity path; if it's wrong, nothing reactive works | Wire signal → binding → mutation with mock flush |
| SSR + hydration metadata | Metadata schema must survive server-to-client transfer | Generate metadata on server, parse on client |
| Property vs Attribute dispatch | Wrong dispatch breaks inputs, checkboxes, etc. | Test both paths with known keys |

## Structure

### Setup (15 min)

Create a single Go module with this structure:

```
spike/
  node.go            -- Node, ElementNode, TextNode
  signal.go          -- Signal[T] with Get/Set/Subscribe
  binding.go         -- BindingRegistry, SignalAccessor
  mutations.go       -- Mutation types + queue
  dom_renderer.go    -- Node tree → []Mutation
  ssr_renderer.go    -- Node tree → HTML string
  usescope.go        -- UseScope prototype
  main_test.go       -- All spike tests in one file
```

All code in package `spike` for iteration speed. No packages, no `go mod init` ceremony.

### Phase 1: Node Type + Both Renderers (45 min)

**Define the types:**

```go
type Signal[T any] struct {
    value T
    subs  []func()
}
func NewSignal[T any](v T) *Signal[T]
func (s *Signal[T]) Get() T
func (S *Signal[T]) Set(v T)
func (s *Signal[T]) Subscribe(fn func()) func()
```

```go
type Node interface { nodeMarker() }

type ElementNode struct {
    Tag      string
    Props    map[string]any
    Children []Node
}

type TextNode struct {
    Value any // string | *Signal[string]
}

type FragmentNode struct {
    Children []Node
}
```

**Implement DOM renderer** — walks Node tree depth-first, assigns node IDs, emits `[]Mutation`:

```go
type Mutation struct {
    Type   MutationType // MutCreateElement, MutSetProperty, MutAppendChild
    NodeID int
    Key    string
    Value  string
}

type DOMRenderer struct {
    nextID int
    Bindings *BindingRegistry
}

func (r *DOMRenderer) Render(n Node) []Mutation
```

`BindingRegistry` captures signal → property mappings for later updates. The initial render produces all create/append/set mutations.

**Implement SSR renderer** — walks the same Node tree, produces HTML string:

```go
type SSRRenderer struct {
    nextID int
    Meta   HydrationMeta
}

func (r *SSRRenderer) Render(n Node) string
```

Each renderer assigns node IDs independently. SSR embeds `data-node-id` attributes. DOM renderer maps node IDs to real DOM nodes via the mutation stream.

**Passing test:**

```go
func TestBothRenderers(t *testing.T) {
    n := &ElementNode{
        Tag: "div",
        Props: map[string]any{"class": "greeting"},
        Children: []Node{
            &TextNode{Value: "hello"},
        },
    }
    dom := &DOMRenderer{}
    muts := dom.Render(n) // → [MutCreateElement(1, "div"), MutSetAttribute(1, "class", "greeting"), MutCreateElement(2, "#text"), MutSetProperty(2, "textContent", "hello"), MutAppendChild(1, 2)]

    ssr := &SSRRenderer{}
    html := ssr.Render(n) // → `<div data-node-id="1" class="greeting">hello</div>`
}
```

### Phase 2: Signal Binding + Reactive Update (45 min)

**Wire the reactive path:**

```go
type BindingRegistry struct {
    bindings map[int][]Binding
}
type Binding struct {
    NodeID   int
    Signal   SignalAccessor
    Property string
}

func (r *BindingRegistry) Bind(nodeID int, sig SignalAccessor, prop string)
func (r *BindingRegistry) OnSignalChange(nodeID int) []Mutation // called by signal subscriber
```

`SignalAccessor` interface:

```go
type SignalAccessor interface {
    Get() any
    Subscribe(func()) func()
}
```

Make `Signal[T]` implement it.

**Test the full reactive loop:**

```go
func TestReactiveUpdate(t *testing.T) {
    count := NewSignal(0)
    n := &ElementNode{
        Tag: "span",
        Props: map[string]any{
            "textContent": count, // SignalAccessor
        },
    }

    renderer := &DOMRenderer{Bindings: NewBindingRegistry()}
    initial := renderer.Render(n)
    // initial has MutCreateElement + MutSetProperty(1, "textContent", "0")

    // Simulate signal change
    count.Set(1)
    // → Bindings.OnSignalChange(1) returns [MutSetProperty(1, "textContent", "1")]
    updates := renderer.Bindings.OnSignalChange(1)

    if len(updates) != 1 { t.Fatal("expected 1 mutation") }
    if updates[0].Value != "1" { t.Fatal("expected updated value") }
}
```

### Phase 3: UseScope + Structural Re-render (60 min)

**Prototype `UseScope`:**

```go
type Scope struct {
    deps   []*Signal
    render func() Node
    last   Node           // cached last output for diff
    cleanup func()
}

func NewScope(deps []*Signal, fn func() Node) *Scope
func (s *Scope) Node() Node // returns current render output
func (s *Scope) OnDepChanged() // re-runs render, returns structural diffs
```

`OnDepChanged` performs a minimal structural comparison:

1. Re-execute `s.render()` to get new `Node` tree.
2. Compare with `s.last`:
   - Same node type, same tag, same children count → emit property mutations only.
   - Different structure → emit remove + create mutations for diverged subtrees.

**Keep the diff simple** — don't try to match React-level reconciliation. Just three rules:

| Old child | New child | Action |
|-----------|-----------|--------|
| `ElementNode` | `ElementNode`, same tag | Walk children, emit per-property diffs |
| `ElementNode` | `nil` | Emit `MutRemoveNode` |
| `nil` | `ElementNode` | Emit create + append |
| `TextNode` | `TextNode` | Emit `MutSetProperty` if value changed |

**Test:**

```go
func TestStructuralUpdate(t *testing.T) {
    show := NewSignal(true)
    scope := NewScope([]*Signal{show}, func() Node {
        if !show.Get() {
            return nil
        }
        return &ElementNode{Tag: "div", Children: []Node{
            &TextNode{Value: "visible"},
        }}
    })

    // Initial render
    n1 := scope.Node() // → ElementNode{div}
    // On toggle
    show.Set(false)
    diffs := scope.OnDepChanged() // → [MutRemoveNode(1)]
    if len(diffs) != 1 { t.Fatal("expected removal") }
}
```

This validates that structural updates work without a full VDOM diff.

### Phase 4: SSR + Hydration Metadata (30 min)

**Extend SSR renderer to emit metadata:**

```go
type HydrationMeta struct {
    NodeMap map[int]string    // nodeID → component path
    Deps    map[int][]SlotRef // nodeID → signal dependencies
}

type SSRRenderer struct {
    nextID int
    Meta   HydrationMeta
}

func (r *SSRRenderer) RenderWithMeta(n Node, path string, hooks []*Signal) (html string, meta HydrationMeta)
```

The SSR renderer assigns node IDs and records which signals each node depends on. The metadata is serialized to JSON alongside the HTML.

**Test:**

```go
func TestHydrationMetadata(t *testing.T) {
    count := NewSignal(0)
    n := &ElementNode{Tag: "button", Props: map[string]any{"textContent": count}}
    ssr := &SSRRenderer{}
    html, meta := ssr.RenderWithMeta(n, "root/0", []*Signal{count})

    // html: <button data-node-id="1" />
    // meta.Deps[1] → [{ComponentPath: "root/0", HookIndex: 0}]

    // Client-side: replay, find ComponentNode at "root/0",
    // read Hooks[0] → count signal, subscribe to node ID 1
    if meta.Deps[1][0].ComponentPath != "root/0" { t.Fatal("bad path") }
}
```

### Phase 5: Integration Walkthrough (15 min)

Write one test that exercises the full pipeline:

```go
func TestCounterEndToEnd(t *testing.T) {
    // 1. Component renders Node tree with signal-bound text + onclick handler
    count := NewSignal(0)
    n := &ElementNode{Tag: "button", Props: map[string]any{
        "textContent": count,
        "onclick": func() { count.Set(count.Get() + 1) },
    }}

    // 2. DOM renderer produces initial mutations
    renderer := &DOMRenderer{Bindings: NewBindingRegistry()}
    initial := renderer.Render(n)

    // 3. Simulate click
    n.Props["onclick"].(func())()

    // 4. Signal change produces update mutations
    updates := renderer.Bindings.OnSignalChange(1)

    // 5. Assert: value updated to "1"
    if updates[0].Value != "1" { t.Fatal("counter not updated") }

    // 6. SSR renderer produces HTML + metadata
    ssr := &SSRRenderer{}
    html, _ := ssr.RenderWithMeta(n, "root/0", []*Signal{count})
    if !strings.Contains(html, "data-node-id") { t.Fatal("missing hydration attr") }
}
```

## What Success Looks Like

At end of half-day, you should have:

1. **All tests pass** — `go test ./spike/...` green.
2. **Node type is stable** — you're confident adding more node types won't break renderers.
3. **Reactive path works** — signal → binding → mutation flow is proven.
4. **Structural updates work** — `UseScope` prototype handles conditional show/hide.
5. **SSR + hydration metadata** — metadata survives rendering and can be re-parsed.
6. **Property/attribute dispatch** — `textContent`, `class`, `checked` all handled correctly.

## What to Throw Away

This spike is **throwaway code**. Do not structure it for reuse. Do not split into packages. Do not add error handling or edge cases. The goal is validation, not production code.

After the spike, you'll know with confidence that the `plan-v1.md` architecture works. The production implementation can start from Phase 1 with the Node type and renderer contract already validated.

## If You Hit a Wall

| Blocked on | Fallback |
|---|---|
| UseScope diff logic too complex | Skip diffing — just tear down and rebuild subtree |
| Hydration metadata schema feels wrong | Remove metadata from spike; test SSR HTML output only |
| BindingRegistry + Mutation queue coupling | Merge into a single `RenderContext` that accumulates both |
| Time running out | Drop Phase 3 (UseScope) and Phase 5 (integration); focus on Phase 1, 2, 4 |
