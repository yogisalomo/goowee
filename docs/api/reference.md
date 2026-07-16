# API Reference

The public surface of goowee spans five packages. Import them as:

```go
import (
    "github.com/yogisalomo/goowee/core"
    . "github.com/yogisalomo/goowee/h"       // dot-imported DSL
    "github.com/yogisalomo/goowee/hooks"
    "github.com/yogisalomo/goowee/router"
    "github.com/yogisalomo/goowee/ssr"        // server-side renderer
    "github.com/yogisalomo/goowee/bridge"    // WASM entry point
)
```

---

## `core` — Signals, scheduler, node types

### Signals

```go
// Create a signal with an initial value.
s := core.NewSignal(0)                    // *core.Signal[int]

// Read and write.
s.Get() int
s.Set(v T)
s.WithEquals(func(a, b T) bool)          // custom equality (default: ==)

// Derived signal that recomputes when deps change.
total := core.Computed([]core.SignalAccessor{items}, func() int {
    return sum(items.Get())
})                                       // *core.Signal[int]
```

### Types

```go
type SignalAccessor interface { SignalID() SignalID }

type Node interface { nodeMarker() }
type ElementNode struct { Tag, Namespace string; Attrs, Props, Events, Children }
type TextNode struct { Text string }
type ComponentNode struct { Name string; Setup func() Node }
type ScopeNode struct { ... }         // created by Show/For/Switch
```

### Scheduler

```go
core.Schedule(func())                  // enqueue an update on the render loop
```

- Call from goroutines, timers, callbacks (off-loop code) to update signals
  safely.

---

## `h` — Element, attribute, and event DSL (dot-import)

### Elements

```go
Div(children...)
Span(children...)
P(children...)
H1(children...) ... H6(children...)
Button(children...)
Input(items...)
Form(items...)
Label(children...)
Ul(children...)
Li(children...)
Nav(children...)
Main(children...)
Footer(children...)
A(children...)
Img(items...)
Table(children...)
Thead(children...)
Tbody(children...)
Tr(children...)
Th(children...)
Td(children...)
Select(children...)
Option(children...)
Textarea(items...)

// Generic element:
El("tag", items...)

// Raw HTML (untrusted input must be sanitized):
Raw(html string)                        // renders pre-rendered HTML verbatim
```

### SVG

```go
Svg(items...)                           // roots an SVG namespaced subtree
G(items...), Path(items...), Circle(items...), Rect(items...)
Line(items...), Polyline(items...), Polygon(items...), Ellipse(items...)
```

### Text

```go
Text("static string")                   // never updates
TextS(sig *core.Signal[string])         // reactive — tracks the signal
Textf("format %d %s", args...)          // reactive when any arg is a signal
```

### Attributes (static)

```go
Class("value"), ID("value"), Href("value"), Style("value")
Type("value"), Name("value"), Placeholder("value")
Target("value"), Rel("value"), ReadOnly(bool), Disabled(bool)
Rows(int), Cols(int), Min(int), Max(int), Step("value")
Checked(bool), Value("value"), Selected(bool), Required(bool)
AutoFocus(bool)
Attr("name", "value")                   // arbitrary attribute

// ARIA
AriaLabel("value"), AriaHidden(bool), AriaCurrent("value")
AriaExpanded(bool), AriaPressed(bool), AriaSelected(bool)
AriaChecked(bool), AriaDisabled(bool), AriaRequired(bool)
AriaInvalid(bool), AriaDescribedBy("value"), AriaLabelledBy("value")
```

### Attributes (reactive — track a signal)

Add `S` suffix: `ClassS(sig)`, `StyleS(sig)`, `HrefS(sig)`, `ValueS(sig)`,
`DisabledS(sig)`, `CheckedS(sig)`, `RequiredS(sig)`, `ReadOnlyS(sig)`,
`AutoFocusS(sig)`, `PlaceholderS(sig)`, `NameS(sig)`, `TypeS(sig)`.

These accept a `*core.Signal[string]` or `*core.Signal[bool]` and update the
attribute when the signal changes.

Arbitrary reactive attribute: `AttrS(name string, sig *core.Signal[string])`.

### Events

**Simple handlers** — receive the relevant value directly:

```go
OnClick(func())                           // click
OnInput(func(string))                     // element value on each keystroke
OnChange(func(string))                    // element value on commit
OnSubmit(func(map[string]string))         // form values keyed by Name
OnFocus(func()), OnBlur(func())
OnKeyDown(func()), OnKeyUp(func())
OnPaste(func(string)), OnCut(func(string)), OnCopy(func(string))
OnFocusIn(func()), OnFocusOut(func())
OnReset(func()), OnInvalid(func())
```

**Full event data** — receive `core.EventData` with position, key, target, etc.:

```go
OnClickE(func(core.EventData), ...options)
```

Options: `PreventDefault()`, `StopPropagation()`, `Once()`, `Passive()`.

Available on all DOM event types (keyboard, mouse, pointer, focus, etc.)
via the `E` suffix pattern.

### Two-way bindings

```go
BindValue(sig *core.Signal[string])          // input ↔ signal (keystroke)
BindValueLazy(sig *core.Signal[string])      // input ↔ signal (on change/blur)
BindChecked(sig *core.Signal[bool])          // checkbox ↔ signal
BindSelect(sig *core.Signal[string])         // <select> ↔ signal
```

### Control flow

```go
// Conditional rendering.
Show(cond interface{}, then func() core.Node)
ShowElse(cond interface{}, then func() core.Node, else_ func() core.Node)

// Multi-way switch.
Switch(sig *core.Signal[T], cases map[T]func() core.Node, default func() core.Node)

// Keyed list rendering.
For[T, K comparable](sig *core.Signal[[]T], keyFn func(T) K, render func(T) core.Node)

// Virtualized (windowed) list for large datasets.
VirtualList[T any](sig *core.Signal[[]T], rowHeight int, render func(int, T) core.Node, opts ...VirtualListOption)
```

### Refs and portals

```go
Ref() *core.Ref                          // create a ref handle
RefTo(ref *core.Ref)                     // attach to an element
// Methods on Ref:
ref.Focus()
ref.Blur()
ref.Click()
ref.ScrollIntoView()

Portal(target string, children ...core.Node)  // render into another container
```

### Error boundary

```go
ErrorBoundary(fallback func(err any) core.Node, child core.Node)
```

### Dynamic content (SSR hydration escape hatch)

```go
Dynamic(children ...core.Node)           // client value wins on hydration
```

### Helpers

```go
Nodes(nodes ...core.Node)                // flatten children
Group(children ...core.Node)             // logical group (no wrapper element)
SelectOnFocus()                          // handler option
```

### Binding options

```go
// Options for Input, Textarea, Select:
Value(s string)                          // static value
ValueS(sig *core.Signal[string])         // reactive value
Placeholder(s string)
PlaceholderS(sig *core.Signal[string])
Disabled(bool)
DisabledS(sig *core.Signal[bool])
Required(bool)
RequiredS(sig *core.Signal[bool])
```

---

## `hooks` — Component hooks

```go
// State
count, setCount := hooks.UseState(0)          // (*core.Signal[T], func(T))

// Computed (re-exported from core for convenience)
total := hooks.UseComputed([]core.SignalAccessor{deps}, func() int)

// Effects
hooks.OnMount(func() func())                  // run once on mount, return cleanup
hooks.Watch([]core.SignalAccessor{deps}, func())  // react to signal changes
hooks.UseEffect([]core.SignalAccessor{deps}, func() func())  // mount + deps + cleanup

// Scope (for manual scope re-renders — internal use for Show/For/Switch)
hooks.UseScope()                              // *core.ScopeNode

// Async data
res := hooks.UseResource[T]([]core.SignalAccessor{deps}, func() (T, error))
// res *Resource[T] with fields:
res.Data      *core.Signal[T]
res.Loading   *core.Signal[bool]
res.Err       *core.Signal[error]
res.Refetch()

// Convenience wrapper for Resource state rendering (in h package):
h.ShowResource[T](res, loadingFn, errFn, dataFn)
```

---

## `router` — Client-side routing

```go
// Create a router from the current URL path.
r := router.New(initialPath string)

// Enable browser history integration (popstate + pushState).
r.BindHistory()

// Define routes. Most-specific match wins.
r.Route(routes map[string]func() core.Node)

// Read params.
r.Param(name string) string                    // snapshot for handlers
r.ParamSignal(name string) *core.Signal[string] // reactive — bind to markup
r.Params() map[string]string

// Read/write query parameters.
r.QueryParam(name string) string                       // snapshot
r.QueryParamSignal(name string) *core.Signal[string]   // reactive
r.SetQueryParam(name, value string)                    // replaceState
r.SetQueryParamPush(name, value string)                // pushState

// Navigation.
r.Navigate(path string)
r.NavigateReplace(path string)
r.Back()
r.Forward()
r.Link(path, text string) core.Node            // renders an <a>

// Nested routing.
r.SubRoute(prefix string, routes map[string]func() core.Node)

// Guards.
router.Guard(check func() bool, fallback func() core.Node, route func() core.Node) func() core.Node

// Lazy loading.
router.Lazy(load func() core.Node) func() core.Node

// Current path (for initialisation).
router.CurrentPath() string
```

---

## `ssr` — Server-side rendering

```go
import "github.com/yogisalomo/goowee/ssr"

renderer := ssr.New()
html := renderer.Render(node core.Node)        // string
```

The SSR renderer produces HTML with `data-node-id` attributes and text
marker comments. The client-side `dom` renderer detects these during boot
and hydrates instead of rebuilding.

---

## `bridge` — WASM entry point

```go
import "github.com/yogisalomo/goowee/bridge"

bridge.Run(app core.Node)
```

`Run` mounts `app` into the page, hydrates it if the HTML was server-rendered,
and drives the render loop. It never returns (keeps the WASM module alive).
Called once from `main()` — that's all you need for the client side.
