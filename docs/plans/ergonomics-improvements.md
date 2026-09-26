# Ergonomics Improvements: Typed DSL, Control Flow, and (Later) Templates

*Authored 2026-07-03. Status: proposed.*
*Prerequisite reading: `docs/26-07-02-fable-review.md` (the review), `docs/learnings/competitor-research.md`.*

This is an **implementation spec**, written so a contributor or code assistant can execute it without making additional design decisions. Where a choice existed, it has been made and the rationale noted. Code blocks are normative unless marked "sketch".

---

## 1. Goal & Motivation

Today's authoring API is the weakest part of goowee:

```go
&core.ElementNode{Tag: "button", Props: map[string]any{
    "textContent": count,
    "onclick": func(ed core.EventData) { setCount(count.Get() + 1) },
}}
```

Problems, in order of severity:

1. **`Props map[string]any` is the root cause of real bugs.** The renderer and differ discover what each prop *means* (attribute? property? signal binding? event handler?) by type-switching at render time — and the differ forgot two cases (event handlers and signal props on reused elements; review §5.3). Classifying props at **construction time** into typed fields makes that bug class unrepresentable.
2. Stringly-typed keys: no autocomplete, no compile-time checking, `"textContent"`/`"onclick"` magic strings everywhere.
3. Structural reactivity requires hand-writing `hooks.UseScope(func() core.Node {...}, deps...)` — verbose and easy to get wrong (forgetting deps).
4. Reactive text formatting is impossible without manual `Computed`-like plumbing (see the stopwatch bug: `examples/counter/app/app.go:412` evaluates an IIFE once and never updates).

Target end state (Phase 1 + 2):

```go
import . "goowee/h"

func CounterPage() core.Node {
    return core.Component("CounterPage", func() core.Node {
        count, _ := hooks.UseState(0)
        show, _ := hooks.UseState(true)

        return Div(Class("counter"),
            Button(
                OnClick(func() { count.Update(func(c int) int { return c + 1 }) }),
                Textf("Count: %d", count),
            ),
            Button(OnClick(func() { show.Update(func(b bool) bool { return !b }) }),
                Text("Toggle")),
            Show(show, func() core.Node {
                return P(Class("greeting"), Text("Hello!"))
            }),
        )
    })
}
```

### Design constraints (why the API looks like this)

- Go has **no macros, no operator overloading, no JSX-style syntax extension**. The only paths are (A) the best possible pure-Go DSL and (B) a compiled template format. This plan does A now (Phases 1–2) and defers B (Phase 3) — B compiles down to A, so A is never throwaway work.
- Go has **no function overloading**, and generic type sets **cannot union interfaces that have methods** (`string | core.SignalAccessor` is illegal). Therefore static and reactive variants are **separate functions with an `-S` suffix**: `Class("x")` vs `ClassS(sig)`. This is deliberate and consistent — do not try to unify them with `any`.
- Everything must remain plain Go: gofmt-able, LSP-native, zero build steps (until Phase 3).

---

## 2. Phase Overview

| Phase | Deliverable | Depends on |
|---|---|---|
| 1 | Typed item DSL in new package `h`; `ElementNode` restructured to typed fields; renderer/differ/SSR/bridge updated; `html` package deleted; examples migrated | — (folds in several review fixes, listed in §3.9) |
| 2 | Control-flow primitives `Show`/`ShowElse`/`Switch`/`For`; keyed reconciliation; `VirtualList` rebuilt on `For` | Phase 1 |
| 3 | *(optional, gated)* `.gwx` template compiler + LSP/formatter/editor tooling | Phase 1–2 stable, external demand |

Phase 1 is a **big-bang breaking change**: `ElementNode.Props` (the map) is removed entirely. No dual/legacy path may remain — dual dispatch paths are how the SSR/DOM renderers drifted apart in the first place. The project has no external users; use that freedom now.

---

## 3. Phase 1 — Typed Item DSL

### 3.1 Core type changes (`core/node.go`)

Replace `ElementNode` and add supporting types:

```go
// ElementNode represents a DOM element. Props are classified at
// construction time into typed collections; there is no map.
type ElementNode struct {
    ID       int
    Tag      string
    Key      any        // used by keyed reconciliation (Phase 2); nil = unkeyed
    Attrs    []Attr     // static HTML attributes  → MutSetAttribute
    Props    []Prop     // static JS properties    → MutSetProperty
    Binds    []Bind     // signal bindings         → BindingRegistry
    Handlers []Handler  // event handlers          → NodeRegistry
    Children []Node
}

// Attr is a static HTML attribute (el.setAttribute).
type Attr struct {
    Name  string
    Value string
}

// Prop is a static JS property (el[name] = value).
// Value must be string, bool, int, or float64.
type Prop struct {
    Name  string
    Value any
}

// BindTarget says whether a signal binding writes a JS property or an
// HTML attribute.
type BindTarget int

const (
    BindToProp BindTarget = iota
    BindToAttr
)

// Bind declares a reactive binding: when Signal changes, the renderer
// re-writes the target property/attribute.
type Bind struct {
    Target BindTarget
    Name   string
    Signal SignalAccessor
}

// Handler is an event handler plus dispatch options.
type Handler struct {
    Event   string // "click", "input", ... (no "on" prefix)
    Fn      func(EventData)
    Options HandlerOptions
}

type HandlerOptions struct {
    PreventDefault  bool
    StopPropagation bool
}
```

Add the **Item** interface and make every node type an Item (this is what lets children and props share one variadic parameter):

```go
// Item is anything that can be applied to an element under
// construction: children, attributes, properties, bindings, handlers.
type Item interface {
    Apply(el *ElementNode)
}
```

Nil-safe `Apply` methods on all node types (a typed-nil `*ElementNode` passed as an Item must be a no-op, not a panic):

```go
func (e *ElementNode) Apply(parent *ElementNode) {
    if e == nil { return }
    parent.Children = append(parent.Children, e)
}
func (t *TextNode) Apply(parent *ElementNode) {
    if t == nil { return }
    parent.Children = append(parent.Children, t)
}
func (f *FragmentNode) Apply(parent *ElementNode) {
    if f == nil { return }
    parent.Children = append(parent.Children, f)
}
func (c *ComponentNode) Apply(parent *ElementNode) {
    if c == nil { return }
    parent.Children = append(parent.Children, c)
}
func (s *ScopeNode) Apply(parent *ElementNode) {
    if s == nil { return }
    parent.Children = append(parent.Children, s)
}
```

**Delete** the old `Props map[string]any` field and every read of it (renderer, differ, SSR, tests). `FlatTree`/`CollectIDs`/`FragmentNode`/`ComponentNode`/`ScopeNode` are otherwise unchanged.

### 3.2 Core: mutations (`core/scheduler.go`)

```go
type Mutation struct {
    Type    MutationType `json:"type"`
    NodeID  int          `json:"nodeId"`
    Key     string       `json:"key,omitempty"`
    Value   any          `json:"value,omitempty"`
    ChildID int          `json:"childId,omitempty"`
    RefID   int          `json:"refId,omitempty"` // NEW: reference node for InsertBefore (0 = append)
}

const (
    MutCreateElement MutationType = iota
    MutRemoveNode
    MutSetAttribute
    MutSetProperty
    MutAppendChild
    MutInsertBefore
    MutRemoveAttribute // NEW = 6 (appended; do not renumber existing values)
)
```

`MutInsertBefore` **no longer uses `Key` as an index** (the index scheme was broken: JS `parent.children` excludes text nodes; review §5.4). It now carries `ChildID` (node to place) and `RefID` (node to insert before; `0` means append at end).

### 3.3 Core: signals & computed

**`Signal.Update`** (`core/signal.go`):

```go
// Update sets the value derived from the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }
```

**Disposers on frames** (`core/component_tree.go`) — needed so `Computed` subscriptions are cleaned up on unmount:

```go
type ComponentFrame struct {
    Path        string
    Parent      *ComponentFrame
    Children    []*ComponentFrame
    RootNodeIDs []int
    Hooks       []any
    Disposers   []func() // NEW: called on unmount, before children
}

// RegisterDisposer attaches a cleanup function to the current component
// frame. No-op outside a component context.
func RegisterDisposer(fn func()) {
    if f := CurrentComponent(); f != nil {
        f.Disposers = append(f.Disposers, fn)
    }
}
```

Update `hooks.RunFrameCleanup` to invoke `frame.Disposers` (before recursing into children), then set `frame.Disposers = nil`.

**`Computed`** (new file `core/computed.go`):

```go
// Computed returns a signal derived from other signals. It recomputes
// whenever any dep changes and notifies its own subscribers. Dep
// subscriptions are registered as disposers on the current component
// frame (if any).
func Computed[T any](deps []SignalAccessor, compute func() T) *Signal[T] {
    sig := NewSignal(compute())
    for _, dep := range deps {
        unsub := dep.Subscribe(func() { sig.Set(compute()) })
        RegisterDisposer(unsub)
    }
    return sig
}
```

### 3.4 Core: `EventData` accessors (`core/node.go`)

JSON numbers decode as `float64`; these accessors are the single place that convention lives. All are nil-safe on a zero `EventData`.

```go
func (e EventData) str(k string) string {
    if v, ok := e.Data[k].(string); ok { return v }
    return ""
}
func (e EventData) num(k string) float64 {
    if v, ok := e.Data[k].(float64); ok { return v }
    return 0
}

func (e EventData) Value() string        { return e.str("value") }
func (e EventData) Key() string          { return e.str("key") }
func (e EventData) Checked() bool        { v, _ := e.Data["checked"].(bool); return v }
func (e EventData) ScrollTop() float64   { return e.num("scrollTop") }
func (e EventData) ScrollLeft() float64  { return e.num("scrollLeft") }
func (e EventData) ClientX() float64     { return e.num("clientX") }
func (e EventData) ClientY() float64     { return e.num("clientY") }

// FormValues returns submitted form fields (submit event).
func (e EventData) FormValues() map[string]string {
    out := map[string]string{}
    if m, ok := e.Data["values"].(map[string]any); ok {
        for k, v := range m {
            if s, ok := v.(string); ok { out[k] = s }
        }
    }
    return out
}
```

### 3.5 New package `h` — the DSL

File layout:

```
/h
    h.go          // El, If, IfElse, Map, Group, Key, Fragment
    elements.go   // element constructors (from table §3.5.2)
    attrs.go      // static attribute + property helpers (tables §3.5.3–4)
    binds.go      // BindAttr/BindProp + named -S helpers + BindValue/BindChecked
    events.go     // On + typed event helpers + HandlerOption
    text.go       // Text, TextS, Textf
```

`h` imports only `core` (and `fmt`, `strconv`). Phase 2 adds `flow.go` and `virtual_list.go`, which also import `hooks` (no cycle: `hooks` does not import `h`).

The old `/html` package is **deleted** at the end of Phase 1.

#### 3.5.1 Constructor and item plumbing (`h/h.go`)

```go
package h

import "goowee/core"

// El builds an element and applies each item. Nil items are skipped,
// which makes conditional helpers (If, Map over empty slices) work.
func El(tag string, items ...core.Item) *core.ElementNode {
    el := &core.ElementNode{Tag: tag}
    for _, it := range items {
        if it != nil {
            it.Apply(el)
        }
    }
    return el
}

// Fragment groups nodes without a wrapper element.
func Fragment(children ...core.Node) *core.FragmentNode {
    return &core.FragmentNode{Children: children}
}

// Key marks the element for keyed reconciliation (Phase 2).
func Key(k any) core.Item { return keyItem{k} }

type keyItem struct{ k any }
func (k keyItem) Apply(el *core.ElementNode) { el.Key = k.k }

// If returns item when cond is true, else nil (skipped by El).
func If(cond bool, item core.Item) core.Item {
    if cond { return item }
    return nil
}

func IfElse(cond bool, a, b core.Item) core.Item {
    if cond { return a }
    return b
}

// Group applies several items as one.
func Group(items ...core.Item) core.Item { return groupItem(items) }

type groupItem []core.Item
func (g groupItem) Apply(el *core.ElementNode) {
    for _, it := range g {
        if it != nil { it.Apply(el) }
    }
}

// Map builds one item per input element (static lists; for reactive
// lists use For from Phase 2).
func Map[T any](xs []T, fn func(T) core.Item) core.Item {
    g := make(groupItem, 0, len(xs))
    for _, x := range xs {
        g = append(g, fn(x))
    }
    return g
}
```

Item implementations used by the helper tables below:

```go
type attrItem struct{ name, value string }

// Apply: "class" concatenates (space-joined) with any existing class
// attr; every other attribute is last-write-wins.
func (a attrItem) Apply(el *core.ElementNode) {
    for i := range el.Attrs {
        if el.Attrs[i].Name == a.name {
            if a.name == "class" {
                el.Attrs[i].Value += " " + a.value
            } else {
                el.Attrs[i].Value = a.value
            }
            return
        }
    }
    el.Attrs = append(el.Attrs, core.Attr{Name: a.name, Value: a.value})
}

type propItem struct {
    name  string
    value any // string | bool | int | float64
}
func (p propItem) Apply(el *core.ElementNode) {
    el.Props = append(el.Props, core.Prop{Name: p.name, Value: p.value})
}

type bindItem struct {
    target core.BindTarget
    name   string
    sig    core.SignalAccessor
}
func (b bindItem) Apply(el *core.ElementNode) {
    el.Binds = append(el.Binds, core.Bind{Target: b.target, Name: b.name, Signal: b.sig})
}

type handlerItem struct{ h core.Handler }
func (hi handlerItem) Apply(el *core.ElementNode) {
    el.Handlers = append(el.Handlers, hi.h)
}
```

#### 3.5.2 Element constructors (`h/elements.go`)

Every constructor is one line: `func Div(items ...core.Item) *core.ElementNode { return El("div", items...) }`. Constructor name = tag name capitalized. Implement exactly this set (current `html` package set plus common additions):

**Normal elements:**
`Div, Span, P, H1, H2, H3, H4, H5, H6, A, Ul, Ol, Li, Dl, Dt, Dd, Form, Button, Label, Select, Option, Optgroup, Textarea, Nav, Main, Section, Header, Footer, Article, Aside, Strong, Em, B, I, U, Small, Code, Pre, Blockquote, Figure, Figcaption, Table, Thead, Tbody, Tfoot, Tr, Th, Td, Caption, Details, Summary, Dialog, Fieldset, Legend, Progress, Iframe, Video, Audio, Canvas`

**Void elements** (same signature; see void handling in §3.7/§3.8):
`Br, Hr, Img, Input, Source, Track, Wbr, Area, Col, Embed`

Add to `core` (used by both renderers):

```go
// core/node.go
var VoidElements = map[string]bool{
    "br": true, "hr": true, "img": true, "input": true, "source": true,
    "track": true, "wbr": true, "area": true, "col": true, "embed": true,
}
```

SVG is out of scope (needs `createElementNS`); note this in the package doc.

> Implementation note: this file may be hand-written directly from the list above, or generated by a small `go:generate` tool with the list inline. Hand-writing is acceptable; there is no behavior beyond the one-liner.

#### 3.5.3 Static attribute helpers (`h/attrs.go`)

Each is `func Name(v string) core.Item { return attrItem{"<html-name>", v} }` unless a type is noted (int-typed helpers use `strconv.Itoa`).

| Helper | HTML attribute | Type |
|---|---|---|
| `Class` | `class` (concatenating — see attrItem) | string |
| `ID` | `id` | string |
| `Href` | `href` | string |
| `Src` | `src` | string |
| `Alt` | `alt` | string |
| `Title` | `title` | string |
| `Style` | `style` | string |
| `Placeholder` | `placeholder` | string |
| `Name` | `name` | string |
| `Type` | `type` | string |
| `HtmlFor` | `for` (named to avoid clash with `For` control flow) | string |
| `Rel` | `rel` | string |
| `Target` | `target` | string |
| `Role` | `role` | string |
| `Action` | `action` | string |
| `Method` | `method` | string |
| `Autocomplete` | `autocomplete` | string |
| `Min`, `Max`, `Step`, `Pattern`, `Accept` | same, lowercased | string |
| `Width`, `Height` | same | string |
| `Rows`, `Cols`, `TabIndex`, `Colspan`, `Rowspan` | same, lowercased (`tabindex`, `colspan`, `rowspan`) | int |
| `AriaLabel` | `aria-label` | string |

Generic escape hatches:

```go
func Attr(name, value string) core.Item          // any attribute
func Data(name, value string) core.Item          // → "data-"+name
func Aria(name, value string) core.Item          // → "aria-"+name
```

#### 3.5.4 Static property helpers (`h/attrs.go`)

| Helper | JS property | Type |
|---|---|---|
| `Value` | `value` | string |
| `Checked` | `checked` | bool |
| `Disabled` | `disabled` | bool |
| `Selected` | `selected` | bool |
| `ReadOnly` | `readOnly` | bool |
| `Multiple` | `multiple` | bool |
| `Required` | `required` | bool |

Generic: `func Prop(name string, v any) core.Item` (document: v must be string/bool/int/float64).

There is **no `TextContent` helper** — text is always expressed with `Text`/`TextS`/`Textf` child nodes. (Rationale: one way to do it; text children survive diffing and SSR uniformly.)

#### 3.5.5 Signal binding helpers (`h/binds.go`)

```go
func BindProp(name string, sig core.SignalAccessor) core.Item {
    return bindItem{core.BindToProp, name, sig}
}
func BindAttr(name string, sig core.SignalAccessor) core.Item {
    return bindItem{core.BindToAttr, name, sig}
}

// Named conveniences (reactive variants of the common cases):
func ClassS(sig core.SignalAccessor) core.Item    { return BindAttr("class", sig) }
func StyleS(sig core.SignalAccessor) core.Item    { return BindAttr("style", sig) }
func HrefS(sig core.SignalAccessor) core.Item     { return BindAttr("href", sig) }
func ValueS(sig core.SignalAccessor) core.Item    { return BindProp("value", sig) }
func CheckedS(sig core.SignalAccessor) core.Item  { return BindProp("checked", sig) }
func DisabledS(sig core.SignalAccessor) core.Item { return BindProp("disabled", sig) }
```

Two-way binding sugar (composite items):

```go
// BindValue = ValueS(sig) + OnInput(sig.Set): input reflects the signal
// and writes back to it.
func BindValue(sig *core.Signal[string]) core.Item {
    return Group(
        ValueS(sig),
        OnInputE(func(e core.EventData) { sig.Set(e.Value()) }),
    )
}

func BindChecked(sig *core.Signal[bool]) core.Item {
    return Group(
        CheckedS(sig),
        OnInputE(func(e core.EventData) { sig.Set(e.Checked()) }),
    )
}
```

#### 3.5.6 Event helpers (`h/events.go`)

```go
type HandlerOption func(*core.HandlerOptions)

func PreventDefault() HandlerOption {
    return func(o *core.HandlerOptions) { o.PreventDefault = true }
}
func StopPropagation() HandlerOption {
    return func(o *core.HandlerOptions) { o.StopPropagation = true }
}

// On is the generic form. Event names have no "on" prefix.
func On(event string, fn func(core.EventData), opts ...HandlerOption) core.Item {
    h := core.Handler{Event: event, Fn: fn}
    for _, o := range opts {
        o(&h.Options)
    }
    return handlerItem{h}
}
```

Typed conveniences. The `-E` suffix means "gives you the raw `EventData`"; the plain form extracts the payload most handlers want:

| Helper | Signature | Event | Notes |
|---|---|---|---|
| `OnClick` | `func(fn func(), opts ...HandlerOption)` | click | |
| `OnClickE` | `func(fn func(core.EventData), opts ...HandlerOption)` | click | |
| `OnDblClick` / `OnDblClickE` | as above | dblclick | |
| `OnInput` | `func(fn func(value string))` | input | `fn(e.Value())` |
| `OnInputE` | raw | input | |
| `OnChange` / `OnChangeE` | `func(fn func(value string))` / raw | change | |
| `OnSubmit` | `func(fn func(values map[string]string), opts ...HandlerOption)` | submit | **always adds `PreventDefault()`**; `fn(e.FormValues())` |
| `OnKeyDown` | `func(fn func(key string))` | keydown | `fn(e.Key())` |
| `OnKeyDownE`, `OnKeyUp`, `OnKeyUpE` | as pattern | keydown/keyup | |
| `OnFocus` / `OnBlur` | `func(fn func())` | focus / blur | delegated with capture (§3.8) |
| `OnScroll` | `func(fn func(scrollTop float64))` | scroll | capture; `fn(e.ScrollTop())` |
| `OnScrollE` | raw | scroll | |

Implementation pattern (normative for all of them):

```go
func OnClick(fn func(), opts ...HandlerOption) core.Item {
    return On("click", func(core.EventData) { fn() }, opts...)
}
func OnSubmit(fn func(map[string]string), opts ...HandlerOption) core.Item {
    return On("submit",
        func(e core.EventData) { fn(e.FormValues()) },
        append(opts, PreventDefault())...)
}
```

#### 3.5.7 Text helpers (`h/text.go`)

```go
func Text(s string) *core.TextNode  { return &core.TextNode{Value: s} }

// TextS renders a signal's value as text and updates when it changes.
func TextS(sig core.SignalAccessor) *core.TextNode {
    return &core.TextNode{Value: sig}
}

// Textf is fmt.Sprintf for text nodes. Any argument implementing
// core.SignalAccessor is treated as reactive: the text recomputes when
// it changes. With no signal arguments it is a plain static text node.
func Textf(format string, args ...any) core.Node {
    var deps []core.SignalAccessor
    for _, a := range args {
        if s, ok := a.(core.SignalAccessor); ok {
            deps = append(deps, s)
        }
    }
    if len(deps) == 0 {
        return &core.TextNode{Value: fmt.Sprintf(format, args...)}
    }
    compute := func() string {
        resolved := make([]any, len(args))
        for i, a := range args {
            if s, ok := a.(core.SignalAccessor); ok {
                resolved[i] = s.Value()
            } else {
                resolved[i] = a
            }
        }
        return fmt.Sprintf(format, resolved...)
    }
    return &core.TextNode{Value: core.Computed(deps, compute)}
}
```

### 3.6 DOM renderer changes (`dom/renderer.go`)

#### 3.6.1 `renderNode` — ElementNode case (replaces the prop type-switch)

```go
case *core.ElementNode:
    if v == nil { return 0 }          // typed-nil safety
    id := r.allocID()
    v.ID = id
    *muts = append(*muts, core.Mutation{Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: v.Tag})

    for _, a := range v.Attrs {
        *muts = append(*muts, core.Mutation{Type: core.MutSetAttribute, NodeID: id, Key: a.Name, Value: a.Value})
    }
    for _, p := range v.Props {
        *muts = append(*muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: p.Name, Value: p.Value})
    }
    for _, b := range v.Binds {
        r.Bindings.Bind(id, b)
        *muts = append(*muts, bindMutation(id, b)) // initial value
    }
    for _, hd := range v.Handlers {
        r.Registry.RegisterHandler(id, hd.Event, hd.Fn, hd.Options)
    }
    for _, child := range v.Children {
        childID := r.renderNode(child, muts)
        if childID != 0 {
            *muts = append(*muts, core.Mutation{Type: core.MutAppendChild, NodeID: id, ChildID: childID})
        }
    }
    return id
```

with the shared helper:

```go
func bindMutation(nodeID int, b core.Bind) core.Mutation {
    if b.Target == core.BindToAttr {
        return core.Mutation{Type: core.MutSetAttribute, NodeID: nodeID,
            Key: b.Name, Value: fmt.Sprintf("%v", b.Signal.Value())}
    }
    return core.Mutation{Type: core.MutSetProperty, NodeID: nodeID,
        Key: b.Name, Value: b.Signal.Value()}
}
```

Delete `isProperty()` and the `class`→`className` normalization entirely — the item helpers already decided attr-vs-prop, and `class` is now always an attribute.

The `TextNode`, `FragmentNode`, `ComponentNode`, `ScopeNode` cases are unchanged except: guard every case against nil typed pointers (`if v == nil { return 0 }`).

#### 3.6.2 `BindingRegistry` rework (`core/binding.go`)

Fixes review §2.1 (unsubs never stored) and makes bindings target-aware. Full replacement:

```go
package core

type boundBinding struct {
    bind  Bind
    unsub func()
}

type BindingRegistry struct {
    bindings  map[int][]boundBinding
    scheduler *Scheduler
}

func NewBindingRegistry(scheduler *Scheduler) *BindingRegistry {
    return &BindingRegistry{bindings: make(map[int][]boundBinding), scheduler: scheduler}
}

// Bind subscribes to b.Signal; on change it enqueues a mutation for
// THIS binding only. The unsubscribe fn is stored for Unbind.
func (r *BindingRegistry) Bind(nodeID int, b Bind) {
    unsub := b.Signal.Subscribe(func() {
        r.scheduler.Enqueue(mutationForBind(nodeID, b))
    })
    r.bindings[nodeID] = append(r.bindings[nodeID], boundBinding{bind: b, unsub: unsub})
}

// Unbind removes all bindings for a node AND cancels their signal
// subscriptions.
func (r *BindingRegistry) Unbind(nodeID int) {
    for _, bb := range r.bindings[nodeID] {
        if bb.unsub != nil { bb.unsub() }
    }
    delete(r.bindings, nodeID)
}

func mutationForBind(nodeID int, b Bind) Mutation {
    if b.Target == BindToAttr {
        return Mutation{Type: MutSetAttribute, NodeID: nodeID, Key: b.Name,
            Value: fmt.Sprintf("%v", b.Signal.Value())}
    }
    return Mutation{Type: MutSetProperty, NodeID: nodeID, Key: b.Name,
        Value: b.Signal.Value()}
}
```

(Delete the old `Binding` struct and `GetMutationsFor`; `dom.bindMutation` may simply call `core.mutationForBind` — export it as `MutationForBind` if sharing, or duplicate the 6 lines. Prefer exporting.)

> **Prerequisite:** the stored `unsub` functions above are only correct once `Signal.Subscribe` uses token-based removal instead of reflect pointers (review §2.2) and notification is reentrancy-safe (review §2.3). Both are fully specced in `docs/plans/signal-core-fixes.md` — implement that plan first (it is step 1 of the checklist in §6).

#### 3.6.3 `NodeRegistry` rework (`dom/node_registry.go`)

```go
type HandlerEntry struct {
    Fn      func(core.EventData)
    Options core.HandlerOptions
}

type DOMNode struct {
    ID     int
    Events map[string]HandlerEntry
}

type NodeRegistry struct {
    nodes map[int]*DOMNode

    announced map[string]bool
    // OnNewEventType is called the first time a handler for a new event
    // type is registered (and replayed by bridge.Init via EventTypes).
    // The bridge uses it to install the JS document-level listener.
    OnNewEventType func(eventType string, capture bool)
}

// eventUsesCapture: non-bubbling events must be delegated with
// capture=true at the document level.
var eventUsesCapture = map[string]bool{
    "focus": true, "blur": true, "scroll": true,
}

func (r *NodeRegistry) RegisterHandler(nodeID int, event string,
    fn func(core.EventData), opts core.HandlerOptions) {
    n := r.nodes[nodeID]
    if n == nil {
        n = &DOMNode{ID: nodeID, Events: map[string]HandlerEntry{}}
        r.nodes[nodeID] = n
    }
    n.Events[event] = HandlerEntry{Fn: fn, Options: opts} // overwrite = re-register
    if !r.announced[event] {
        r.announced[event] = true
        if r.OnNewEventType != nil {
            r.OnNewEventType(event, eventUsesCapture[event])
        }
    }
}

func (r *NodeRegistry) RemoveHandler(nodeID int, event string) {
    if n := r.nodes[nodeID]; n != nil { delete(n.Events, event) }
}

func (r *NodeRegistry) Remove(nodeID int) { delete(r.nodes, nodeID) }

// EventTypes returns all announced event types (for bridge.Init replay,
// since initial render happens before the bridge is initialized).
func (r *NodeRegistry) EventTypes() []string { /* keys of r.announced */ }

// EventCapture reports the capture flag for an event type.
func EventCapture(event string) bool { return eventUsesCapture[event] }

// Dispatch runs the handler for (nodeID, event) if present. It returns
// the handler's options and whether a handler ran, so the bridge can
// tell JS to preventDefault/stopPropagation and to keep bubbling if
// unhandled. Panics in handlers are recovered and logged.
func (r *NodeRegistry) Dispatch(nodeID int, event string, dataJSON string) (core.HandlerOptions, bool) {
    n := r.nodes[nodeID]
    if n == nil { return core.HandlerOptions{}, false }
    entry, ok := n.Events[event]
    if !ok { return core.HandlerOptions{}, false }

    var data map[string]any
    _ = json.Unmarshal([]byte(dataJSON), &data)

    defer func() {
        if rec := recover(); rec != nil {
            log.Printf("goowee: panic in %s handler for node %d: %v", event, nodeID, rec)
        }
    }()
    entry.Fn(core.EventData{Type: event, Target: nodeID, Data: data})
    return entry.Options, true
}
```

#### 3.6.4 Diff rewrite — element reconciliation

Replace the prop-diff section of `diffNode`'s `*core.ElementNode` case with four explicit reconciliations. This **fixes review §5.2 (removed props never unset) and §5.3 (handlers/binds ignored)** by construction.

```go
new.ID = old.ID

// --- Attrs: set changed/new, remove missing ---
oldAttrs := map[string]string{}
for _, a := range old.Attrs { oldAttrs[a.Name] = a.Value }
for _, a := range new.Attrs {
    if ov, ok := oldAttrs[a.Name]; !ok || ov != a.Value {
        *muts = append(*muts, core.Mutation{Type: core.MutSetAttribute, NodeID: old.ID, Key: a.Name, Value: a.Value})
    }
    delete(oldAttrs, a.Name)
}
for name := range oldAttrs { // left over = removed
    *muts = append(*muts, core.Mutation{Type: core.MutRemoveAttribute, NodeID: old.ID, Key: name})
}

// --- Props: set changed/new, zero missing ---
oldProps := map[string]any{}
for _, p := range old.Props { oldProps[p.Name] = p.Value }
for _, p := range new.Props {
    if ov, ok := oldProps[p.Name]; !ok || ov != p.Value {
        *muts = append(*muts, core.Mutation{Type: core.MutSetProperty, NodeID: old.ID, Key: p.Name, Value: p.Value})
    }
    delete(oldProps, p.Name)
}
for name, ov := range oldProps { // removed props get their zero value
    var zero any = ""
    if _, isBool := ov.(bool); isBool { zero = false }
    *muts = append(*muts, core.Mutation{Type: core.MutSetProperty, NodeID: old.ID, Key: name, Value: zero})
}

// --- Binds: unbind all old, bind all new (simple, always correct) ---
// Optimization (same signal instance for same target+name → keep) may
// be added later; do the simple version first.
r.Bindings.Unbind(old.ID)
for _, b := range new.Binds {
    r.Bindings.Bind(old.ID, b)
    *muts = append(*muts, core.MutationForBind(old.ID, b))
}

// --- Handlers: re-register all new (fixes stale closures), remove gone ---
newEvents := map[string]bool{}
for _, hd := range new.Handlers {
    r.Registry.RegisterHandler(old.ID, hd.Event, hd.Fn, hd.Options)
    newEvents[hd.Event] = true
}
for _, hd := range old.Handlers {
    if !newEvents[hd.Event] { r.Registry.RemoveHandler(old.ID, hd.Event) }
}
```

#### 3.6.5 Diff rewrite — children placement (two-pass, RefID-based)

Extract a helper used by both the `ElementNode` and `FragmentNode` cases:

```go
// diffChildren reconciles child lists (positional matching in Phase 1;
// keyed matching added in Phase 2 — see plan §4.2).
func (r *DOMRenderer) diffChildren(parentID int, old, new []core.Node, muts *[]core.Mutation)
```

Behavior (normative):

1. **Pass 1 (forward):** pair children positionally. For each index `i`, call `diffNode(oldChild, newChild, muts)`. `diffNode`'s signature changes to `(id int, created bool)` — `created` is true when the new node was rendered fresh (old was nil, or type/tag mismatch forced remove+create). Surplus old children → `emitRemoveTree`.
2. **Pass 2 (reverse):** walk new children from last to first, remembering `refID` (the ID of the *following* new child; `0` initially). For every child whose `created` flag is true, emit `MutInsertBefore{NodeID: parentID, ChildID: id, RefID: refID}`. Every child (created or not) updates `refID` to its own ID as the pass moves left.
   - `RefID == 0` means append at end (`insertBefore(child, null)`).
   - Reused children never move under positional matching, so no mutation is emitted for them. (Keyed matching in Phase 2 emits moves too.)
3. `diffNode`'s type-mismatch branches must pass the **original interface value** (`newNode`), never the failed type-assertion result — this fixes the typed-nil panic (review §5.1):
   ```go
   // WRONG (current code):   return r.renderNode(new, muts)
   // RIGHT:                  return r.renderNode(newNode, muts)
   ```
4. `TextNode` diff: compare resolved values. If both values are plain strings, emit on inequality (as now). If either side is a `SignalAccessor`: unbind old ID, and if the new value is a signal, `Bind(old.ID, Bind{BindToProp, "textContent", sig})` + emit current value; if the new value is a string, emit it. (Fixes review §5.5.)
5. Delete the `AppendChild`-in-diff emission and the index-carrying `InsertBefore` (both replaced by pass 2).

#### 3.6.6 Top-level mounting (kills the JS root heuristic; review §5.6)

`DOMRenderer.Render` gains an explicit mount target:

```go
// Render renders n and attaches its root node(s) to the container.
// containerID 0 means "the reserved root container" (see bridge §3.8:
// the JS side registers document.getElementById("root") as node 0).
func (r *DOMRenderer) Render(n core.Node) ([]core.Mutation, int) {
    var muts []core.Mutation
    rootID := r.renderNode(n, &muts)
    if rootID != 0 {
        muts = append(muts, core.Mutation{Type: core.MutAppendChild, NodeID: 0, ChildID: rootID})
    } else if frag, ok := n.(*core.FragmentNode); ok {
        for _, c := range frag.Children {
            if id := nodeID(c); id != 0 {
                muts = append(muts, core.Mutation{Type: core.MutAppendChild, NodeID: 0, ChildID: id})
            }
        }
    }
    return muts, rootID
}
```

JS registers the root container as `nodeMap[0] = document.getElementById("root")` at startup and the "append first parentless node" heuristic is deleted.

### 3.7 SSR renderer changes (`ssr/renderer.go`)

Rewrite `renderNodeWithMeta`'s `ElementNode` case to walk typed fields. Rules:

1. **Escape attribute values.** Add:
   ```go
   func escapeAttr(s string) string {
       s = strings.ReplaceAll(s, "&", "&amp;")
       s = strings.ReplaceAll(s, "<", "&lt;")
       s = strings.ReplaceAll(s, ">", "&gt;")
       s = strings.ReplaceAll(s, `"`, "&quot;")
       return s
   }
   ```
   Every attribute value goes through it. Attribute *names* are written as-is but must match `^[a-zA-Z][a-zA-Z0-9-]*$`; skip (and `log.Printf`) any that don't.
2. **Attrs:** ` name="escaped-value"` for each `core.Attr`.
3. **Props → attributes** via this table (props not in the table are skipped in SSR — they are runtime-only):
   | Prop name | SSR output |
   |---|---|
   | `value` | `value="…"` |
   | `checked`, `disabled`, `selected`, `multiple`, `required` (bool) | bare attribute if true, nothing if false |
   | `readOnly` (bool) | `readonly` if true |
4. **Binds:** resolve `b.Signal.Value()`; `BindToAttr` → escaped attribute; `BindToProp` → apply the same prop table with the resolved value (`fmt.Sprintf("%v", …)` for strings). Keep the existing `Meta.Deps` bookkeeping.
5. **Handlers:** skipped (never serialized).
6. **Void elements:** if `core.VoidElements[v.Tag]`, write `<tag …>` with **no closing tag** and **skip children** entirely.
7. Keep `data-node-id` emission as-is (hydration metadata redesign is out of scope here; see review §6).
8. Delete the old prop type-switch, `isBoolAttr` (subsumed by the table above), and the `class`→`className` special-casing.

### 3.8 Bridge & JS runtime changes (`bridge/wasm.go`, `runtime/goowee.js`)

**Go side (`bridge/wasm.go`):**

```go
func Init(sched *core.Scheduler, registry *dom.NodeRegistry) {
    // handleEvent(nodeID, eventType, dataJSON) → {handled, preventDefault, stopPropagation}
    js.Global().Set("handleEvent", js.FuncOf(func(this js.Value, args []js.Value) any {
        opts, handled := registry.Dispatch(args[0].Int(), args[1].String(), args[2].String())
        return map[string]any{
            "handled":         handled,
            "preventDefault":  opts.PreventDefault,
            "stopPropagation": opts.StopPropagation,
        }
    }))

    // Dynamic event delegation: announce every event type Go registers.
    announce := func(eventType string, capture bool) {
        js.Global().Call("goListen", eventType, capture)
    }
    registry.OnNewEventType = announce
    for _, t := range registry.EventTypes() { // replay types registered pre-Init
        announce(t, dom.EventCapture(t))
    }

    startScheduler(sched)
}
```

**JS side (`runtime/goowee.js`):** replace the four hard-coded listeners with:

```js
const listening = {};
window.goListen = function (type, capture) {
    if (listening[type]) return;
    listening[type] = true;
    document.addEventListener(type, e => dispatchToGo(type, e), capture);
};

function buildPayload(type, e) {
    switch (type) {
        case "click": case "dblclick":
        case "pointerdown": case "pointerup": case "pointermove":
            return {clientX: e.clientX, clientY: e.clientY, button: e.button,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "input": case "change": {
            const t = e.target;
            if (t.type === "checkbox" || t.type === "radio")
                return {value: t.value, checked: t.checked};
            return {value: t.value};
        }
        case "submit": {
            const values = {};
            for (const el of e.target.elements) {
                if (!el.name) continue;
                values[el.name] = (el.type === "checkbox" || el.type === "radio")
                    ? (el.checked ? "on" : "off") : el.value;
            }
            return {values};
        }
        case "keydown": case "keyup":
            return {key: e.key, code: e.code,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "scroll":
            return {scrollTop: e.target.scrollTop, scrollLeft: e.target.scrollLeft};
        default:
            return {};
    }
}

// Walk up the goowee-node ancestor chain until some node's Go handler
// claims the event (delegation done in JS because only JS knows the
// DOM parent chain).
function dispatchToGo(type, e) {
    const payload = JSON.stringify(buildPayload(type, e));
    let el = e.target;
    while (el) {
        if (el._nodeID !== undefined) {
            const r = handleEvent(el._nodeID, type, payload);
            if (r && r.handled) {
                if (r.preventDefault) e.preventDefault();
                if (r.stopPropagation) e.stopPropagation();
                return;
            }
        }
        el = el.parentElement;
    }
}
```

This also fixes a latent delegation bug: previously a click on a child element of a button reached only the child's nodeID (which has no handler) and was dropped.

Other `goowee.js` changes:

- `nodeMap[0] = document.getElementById("root")` at startup (see §3.6.6); delete the "append first parentless node" block.
- **Delete the anchor `preventDefault` hack** (lines 29–35) — `router.Link` now passes `PreventDefault()` (§3.9).
- Mutation switch: `case 5` (InsertBefore) becomes `const ref = mut.refId ? nodeMap[mut.refId] : null; if (parent && child) parent.insertBefore(child, ref);`. Add `case 6` (RemoveAttribute): `el.removeAttribute(mut.key)`.

### 3.9 Migrations in-repo

**`router/router.go` — `Link` and the 404 node:**

```go
func (r *Router) Link(to, text string) *core.ElementNode {
    return h.A(
        h.Href(to),
        h.OnClickE(func(core.EventData) { r.Navigate(to) }, h.PreventDefault()),
        h.Text(text),
    )
}
// 404 fallback inside Route():
return h.P(h.Text("404 — page not found"))
```

(`router` gains an import of `goowee/h`; no cycle — `h` does not import `router`.)

**`examples/counter/app/app.go`:** rewrite every page with the `h` API. This is the acceptance exercise for the DSL: if anything in the app cannot be expressed cleanly, the API is missing a helper — add it to the tables above rather than working around it.

**Delete `/html`** (`html/html.go`). Port `VirtualList` to `h/virtual_list.go` using the new props API (Phase 1: mechanical port keeping `UseScope`; Phase 2: rebuild on `For`).

**Tests to rewrite** (they construct `ElementNode` with the old map): `core/core_test.go`, `dom/dom_test.go`, `hooks/hooks_test.go`, `ssr/ssr_test.go`, `router/router_test.go`.

**Review fixes folded into this phase** (same code, same PR — do not defer):

| Review ref | Fix | Where in this plan |
|---|---|---|
| §2.1 | Bindings store & call unsubs | §3.6.2 |
| §2.2 | Token-based `Signal.Subscribe` | §3.6.2 note |
| §2.3 | Copy subscriber list before notify | §3.6.2 note |
| §5.1 | Typed-nil panic in diff | §3.6.5 (3) |
| §5.2 | Removed attrs/props unset | §3.6.4 |
| §5.3 | Handlers/binds reconciled on reuse | §3.6.4 |
| §5.4 | InsertBefore by RefID, not element index | §3.2, §3.6.5, §3.8 |
| §5.5 | Signal-aware text diff | §3.6.5 (4) |
| §5.6 | Explicit root mounting | §3.6.6, §3.8 |
| §7 | preventDefault from Go; handler recover; dynamic event types | §3.6.3, §3.8 |
| §3.3 (SSR) | Attr escaping, void elements, bool props | §3.7 |

### 3.10 Phase 1 test plan

New/updated tests (names indicative; all plain `go test`, no WASM):

- `h`: `TestElAppliesItemsInOrder`, `TestNilItemsSkipped`, `TestTypedNilNodeItemSkipped`, `TestClassConcatenation`, `TestIfElseMapGroup`, `TestTextfStatic`, `TestTextfReactive` (change a dep signal → computed text signal notifies with new formatted value), `TestOnSubmitForcesPreventDefault`.
- `core`: `TestSignalUpdate`, `TestComputedRecomputesAndDisposes` (dispose via `RunFrameCleanup` → further dep changes don't notify), `TestSubscribeTokenRemovesOnlyItself` (two closures from one literal on the same signal), `TestSetNotifiesCopiedList` (subscriber unsubscribing mid-notify doesn't skip others).
- `dom`: `TestRenderTypedFields` (attrs→SetAttribute, props→SetProperty, binds→Bind+initial mutation, handlers→registry), `TestDiffRemovesStaleAttrsAndProps`, `TestDiffRebindsSignals` (binding follows the new signal instance), `TestDiffReplacesHandlers` (click after re-render hits the new closure), `TestDiffTypeChangeNoPanic` (element→text and text→element), `TestDiffChildrenInsertBeforeRefID` (insert in middle: mutation carries RefID of the following sibling), `TestUnbindCancelsSubscription` (after removal, `Set` enqueues nothing), `TestDispatchReturnsOptionsAndRecovers`.
- `ssr`: `TestAttrEscaping` (`"` in a value), `TestVoidElements` (`<br>`, `<input>` — no closing tag, children dropped), `TestBoolPropsAsAttributes` (`Disabled(true)` → `disabled`; false → absent), `TestBindSerialization`.
- `router`: `TestLinkHasPreventDefault`.

### 3.11 Phase 1 definition of done

- [ ] `ElementNode.Props` map no longer exists; `grep -r "map\[string\]any" core/ dom/ ssr/ h/` shows no prop-map usage (EventData.Data is the only allowed one).
- [ ] `/html` package deleted; `h` package covers every element/attr/event the examples need.
- [ ] All tests in §3.10 pass; all pre-existing tests rewritten and passing.
- [ ] `GOOS=js GOARCH=wasm go build ./examples/counter` succeeds; `go build ./cmd/ssr-server` succeeds.
- [ ] Manual smoke test via `make` targets: every route renders, counter increments, form two-way binding works, todos add/remove, stopwatch runs, anchor navigation works **without** the JS anchor hack.
- [ ] All review fixes in the §3.9 table verifiably closed (each has a test).

---

## 4. Phase 2 — Control Flow & Keyed Reconciliation

### 4.1 Control-flow primitives (`h/flow.go`)

These make `UseScope` an internal mechanism most users never touch. All wrap `hooks.UseScope`; a `nil` from the render closure is normalized to an empty fragment (the differ handles empty fragments; raw `nil` roots are not allowed from flow primitives).

```go
package h

import (
    "goowee/core"
    "goowee/hooks"
)

// Show renders then() while cond is true, nothing otherwise.
func Show(cond *core.Signal[bool], then func() core.Node) core.Node {
    return ShowElse(cond, then, nil)
}

// ShowElse renders then() while cond is true, otherwise() when false.
func ShowElse(cond *core.Signal[bool], then, otherwise func() core.Node) core.Node {
    return hooks.UseScope(func() core.Node {
        if cond.Get() {
            return orEmpty(then)
        }
        return orEmpty(otherwise)
    }, cond)
}

// Switch re-renders on sig changes, choosing cases[sig.Get()] or def.
func Switch[T comparable](sig *core.Signal[T], cases map[T]func() core.Node, def func() core.Node) core.Node {
    return hooks.UseScope(func() core.Node {
        if fn, ok := cases[sig.Get()]; ok {
            return orEmpty(fn)
        }
        return orEmpty(def)
    }, sig)
}

// For renders a keyed reactive list. key must be unique and stable per
// item (it becomes ElementNode.Key for keyed diffing). render should
// return an *ElementNode; other node types are rendered but matched
// positionally.
func For[T any, K comparable](items *core.Signal[[]T], key func(T) K, render func(T) core.Node) core.Node {
    return hooks.UseScope(func() core.Node {
        xs := items.Get()
        children := make([]core.Node, 0, len(xs))
        for _, x := range xs {
            n := render(x)
            if el, ok := n.(*core.ElementNode); ok && el != nil {
                el.Key = key(x)
            }
            children = append(children, n)
        }
        return &core.FragmentNode{Children: children}
    }, items)
}

func orEmpty(fn func() core.Node) core.Node {
    if fn == nil {
        return &core.FragmentNode{}
    }
    if n := fn(); n != nil {
        return n
    }
    return &core.FragmentNode{}
}
```

### 4.2 Keyed reconciliation (`dom/renderer.go`, inside `diffChildren`)

Extend `diffChildren` (§3.6.5):

**Detection:** `hasKeys := true` if any `*ElementNode` in `old` or `new` children has `Key != nil`. If false → positional algorithm from Phase 1, unchanged.

**Keyed algorithm (normative):**

1. **Index old children.** `oldByKey map[any]int` for keyed old children (key → index). Unkeyed old children go into an ordered queue `oldUnkeyed []int`.
2. **Pair (forward pass).** For each new child at index `i`:
   - If it is a keyed `*ElementNode` and `oldByKey` contains its key → pair with that old child; mark the old index used.
   - Else if it is unkeyed → pop the next unused index from `oldUnkeyed` whose node is type-compatible (same concrete type; same tag for elements); pair with it, or with `nil` if none.
   - Else (keyed, no match) → pair with `nil` (will be created).
   Call `diffNode(oldChild, newChild, muts)` for each pair, recording `(id, created)` per index.
3. **Remove.** Every old child never paired → `emitRemoveTree`.
4. **Place (reverse pass).** Walk new children from last to first with `refID := 0`:
   - Emit `MutInsertBefore{NodeID: parentID, ChildID: ids[i], RefID: refID}` for **every** child (created or matched). DOM `insertBefore` moves already-attached nodes and inserts detached ones, so this is uniformly correct; re-inserting a node already in position is a harmless no-op-equivalent.
   - Then `refID = ids[i]`.
   - *Optimization (optional, later):* compute the longest increasing subsequence of matched-old positions and skip the emit for children on it. Do **not** attempt this in the first implementation; correctness first.
5. **Duplicate keys** are a programming error: log once per diff (`log.Printf("goowee: duplicate key %v", k)`) and treat the second occurrence as unkeyed. Do not panic.

**Frame cleanup note (SUPERSEDED — no longer true):** this section predicted that
`ScopeNode` re-render would tear down *all* old component frames, so keyed `For`
rows containing `ComponentNode`s would lose component state until an "ownership
redesign" landed. That redesign effectively landed as component reconciliation
(#6) plus keyed matching for component rows (#7): a matched component keeps its
frame across a scope re-render, and `SetKey` keys `ComponentNode`s as well as
`ElementNode`s. Row state and effects now survive appends, removals, and
reorders — see `TestForRowComponentStateSurvivesListChanges` and friends in
`internal/dom/dom_test.go`. The stale limitation in `For`'s doc comment has been
removed; do not reintroduce it.

### 4.3 `VirtualList` rebuilt

Rewrite `h/virtual_list.go` using `For` for the visible window plus the two pad divs, keyed by the item index offset (`Key(startIdx+i)`), preserving the existing options API (`VirtualListHeight`, `VirtualListOverscan`).

### 4.4 Phase 2 test plan

- `h`: `TestShowTogglesSubtree`, `TestShowElseBranches`, `TestSwitchSelectsCaseAndDefault`, `TestForRendersKeyedChildren` (keys land on `ElementNode.Key`), `TestForNilAndEmptyLists`.
- `dom` keyed diff: `TestKeyedReorderReusesIDs` (reverse a 5-item list → all 5 IDs reused, no creates/removes), `TestKeyedRemoveFirst` (remove head → exactly one RemoveNode, no churn on survivors), `TestKeyedInsertMiddle` (one create, InsertBefore with correct RefID), `TestKeyedAndUnkeyedMix`, `TestDuplicateKeysLoggedNotPanic`.
- Behavior test: a fake DOM applier in Go (map of nodeID → parent/children) that applies mutation batches; property test: for random before/after lists, applying the diff to the fake DOM yields exactly the new list. (~100 lines; this is the single highest-value test in the plan.)

### 4.5 Phase 2 definition of done

- [ ] `UseScope` no longer appears in `examples/` (only `Show`/`ShowElse`/`Switch`/`For`/`Textf`).
- [ ] Todos example uses `For` with stable keys; removing the first todo emits 1 `RemoveNode` and 0 element creations.
- [ ] Fake-DOM property test passes 1,000 random cases.
- [ ] `VirtualList` scrolling works in the browser demo.

---

## 5. Phase 3 — `.gwx` Template Compiler (outline only; gated)

**Decision gate:** do not start until (a) Phases 1–2 have been stable for a meaningful period, and (b) there is concrete external demand for HTML-syntax authoring. The Vugu lesson (see `docs/learnings/competitor-research.md`): a template format without editor tooling is *negative* value.

Shape, when/if built:

- `.gwx` files: HTML with `{go expressions}`, compiled by `gwxgen` into **Phase 1 `h` calls** — the template layer is sugar with zero new runtime semantics.
- Control flow maps to Phase 2 primitives: `<if cond={show}>` → `Show`, `<for each={todos} key={t.ID}>` → `For`.
- The deliverable is the *toolchain*, not just the compiler: `gwxgen` + LSP (hover/completion/diagnostics via gopls proxying into generated code) + formatter + VS Code extension. Budget these as first-class, or don't start.
- Study templ's parser architecture (`github.com/a-h/templ`); its generator backend targets string writers and would be replaced with `h`-call emission.

No further specification here on purpose — it must not distract from Phases 1–2.

---

## 6. Implementation Order (checklist for the implementer)

Work top to bottom; each step compiles and its tests pass before the next.

1. [x] `core`: implement `docs/plans/signal-core-fixes.md` in full (token-based `Signal.Subscribe`, safe notification, equality skip, `Signal.Update`). Independently shippable PR.
2. [ ] `core`: `Attr/Prop/Bind/Handler/HandlerOptions/BindTarget`, `ElementNode` restructure, `Item` + nil-safe `Apply` methods, `VoidElements`, `EventData` accessors, `Mutation.RefID` + `MutRemoveAttribute` (§3.1, §3.2, §3.4). *The repo will not compile until step 5 — that's expected; commit at step 6.*
3. [ ] `core`: `ComponentFrame.Disposers` + `RegisterDisposer` + `Computed`; `hooks.RunFrameCleanup` calls disposers (§3.3).
4. [ ] `core`: `BindingRegistry` rework (§3.6.2).
5. [ ] `h` package: h.go, elements.go, attrs.go, binds.go, events.go, text.go (§3.5, all tables).
6. [ ] `dom`: `NodeRegistry` rework (§3.6.3); `renderNode` rewrite (§3.6.1); diff rewrite (§3.6.4, §3.6.5); root mounting (§3.6.6). Rewrite `dom` tests. **Commit.**
7. [ ] `ssr`: renderer rewrite (§3.7). Rewrite `ssr` tests. **Commit.**
8. [ ] `bridge` + `runtime/goowee.js` (§3.8). **Commit.**
9. [ ] `router`, `examples/counter/app`, `h/virtual_list.go` port; delete `/html`; rewrite remaining tests (§3.9). Browser smoke test. **Commit.**
10. [ ] Phase 1 DoD checklist (§3.11). Update `docs/plan-v1.md` §3/§4/§12 to reference this plan.
11. [ ] Phase 2: `diffChildren` keyed algorithm + tests incl. fake-DOM property test (§4.2, §4.4). **Commit.**
12. [ ] Phase 2: `h/flow.go` + `VirtualList` on `For` + example migration (§4.1, §4.3). Phase 2 DoD (§4.5). **Commit.**

## 7. Explicitly Out of Scope (tracked elsewhere)

- Hydration redesign & SSR/DOM ID-parity walker — review §6.
- Dirty-scope render batching & rAF-on-demand scheduler — review §4.
- SSR request-safety (render context instead of global `renderStack`) and no-effects-on-server — review §3. **Note:** if that work lands first and threads a context through render, the `Apply`/constructor API here is unaffected; only renderer internals move.
- `OnMount`/`Watch` lifecycle redesign — review §1.
- SVG namespace support; template compiler implementation (Phase 3 gate).

## 8. Appendix: Full Before/After (todos excerpt)

Before (current API):

```go
&core.ElementNode{Tag: "div", Props: map[string]any{"class": "todos"}, Children: []core.Node{
    &core.ElementNode{Tag: "input", Props: map[string]any{
        "value": input,
        "oninput": func(ed core.EventData) {
            if v, ok := ed.Data["value"].(string); ok { setInput(v) }
        },
    }},
    hooks.UseScope(func() core.Node {
        var lis []core.Node
        for i, t := range todos.Get() {
            i := i
            lis = append(lis, &core.ElementNode{Tag: "li", Children: []core.Node{
                &core.TextNode{Value: t.Title},
                &core.ElementNode{Tag: "button", Props: map[string]any{
                    "textContent": "x",
                    "onclick":     func(core.EventData) { removeAt(i) },
                }},
            }})
        }
        return &core.ElementNode{Tag: "ul", Children: lis}
    }, todos),
}}
```

After (Phase 1 + 2):

```go
Div(Class("todos"),
    Input(BindValue(input)),
    Ul(
        For(todos, func(t Todo) int { return t.ID }, func(t Todo) core.Node {
            return Li(
                Text(t.Title),
                Button(OnClick(func() { remove(t.ID) }), Text("x")),
            )
        }),
    ),
)
```
