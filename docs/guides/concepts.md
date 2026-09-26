# Concepts

This guide explains how goowee works. Read it once to build a mental model;
then the [API reference](../api/reference.md) is all you need day-to-day.

---

## Signals — the unit of reactivity

A **signal** holds a value and notifies subscribers when it changes. Signals
are goowee's answer to "how does the UI know to update."

```go
count, setCount := hooks.UseState(0)  // returns (*core.Signal[int], func(int))
```

- `count.Get()` reads the current value.
- `setCount(v)` or `count.Set(v)` writes a new value and notifies subscribers.

A signal bound to a DOM node updates **only that node** — there's no
re-render, no virtual DOM, no component function re-run. This is fine-grained
reactivity.

Derive a signal from others with `core.Computed`:

```go
total := core.Computed([]core.SignalAccessor{items}, func() string {
    sum := 0
    for _, item := range items.Get() { sum += item.Value }
    return fmt.Sprintf("Total: %d", sum)
})
```

`Computed` recomputes only when a listed dependency changes. It is read-only:
`total.Get()`. It is also lazy: it subscribes to its deps only while something
observes it (a binding, a scope, a `Watch`), and an unobserved `Computed` holds
no subscriptions at all — `Get` recomputes on demand — so creating one inside a
render (as `Textf` does) can't leak.

### Batching

Writes inside an **event handler**, a `core.Schedule` callback, a `ref.Get`
reply, or an effect are batched: each written signal notifies **once, after the
handler returns**, so a `Computed`/`Watch` over several signals never sees a
half-applied update. Outside those paths, wrap related writes yourself:

```go
core.Batch(func() {
    first.Set("Ada")
    last.Set("Lovelace") // fullName (Computed over both) recomputes once
})
```

Values written inside a batch are readable immediately; only notification is
deferred.

### Static vs reactive binding

- `Text("hello")`, `Class("active")`, `Value("x")` — **static**, never update.
- `TextS(sig)`, `ClassS(sig)`, `ValueS(sig)` — **reactive**, track the signal.
- `Textf("Count: %d", count)` — **reactive** when any `*core.Signal` is passed
  as a printf argument; static args are fine alongside signal args.

Every attribute and property has a reactive `-S` variant: `StyleS`, `DisabledS`,
`CheckedS`, `HrefS`, etc.

---

## Components — run once, not re-render

A component is a function wrapped in `core.Component`:

```go
func Greeting() core.Node {
    return core.Component("Greeting", func() core.Node {
        name, setName := hooks.UseState("World")
        return Div(
            P(Textf("Hello, %s!", name)),
            Input(BindValue(name)),
        )
    })
}
```

**The function body runs exactly once, when the component mounts.** It is
setup, not render. State lives in signals captured by closures; the DOM is
bound to those signals declaratively via the reactive helpers.

This is the Single Most Important Fact about goowee. If you expect the body
to re-run when state changes, you will fight the framework. Instead, think:
"what signals does this DOM depend on?"

Consequences:
- No hook-ordering rules — call `UseState` conditionally, in loops, anywhere.
- A matched component — same name **and** key — is preserved across parent
  re-renders (state + effects survive). A plain `core.Component` therefore
  keeps showing the plain values it was first called with.

### Inputs that change

When a parent can hand a component new data, pick one:

- **Pass signals** instead of values — the component binds to them.
- **`core.ComponentWithProps`** — the component receives its props as a signal;
  a preserved instance gets new props through it and keeps its state:

  ```go
  func UserCard(u User) core.Node {
      return core.ComponentWithProps("UserCard", u, func(p *core.Signal[User]) core.Node {
          open, setOpen := hooks.UseState(false) // survives new props
          return Div(Textf("%v", p), …)
      })
  }
  ```

- **Key it** to remount when a value changes (React's `key` idiom):
  `core.SetKey(Card(name), name)`.

In dev mode (`?goowee-dev`) goowee warns, once per component, when an unkeyed
plain component is kept across a parent re-render.

---

## Control flow — Show, ShowElse, Switch, For

Because the component body runs once, you need declarative control flow to
react to signal changes.

### Show / ShowElse

```go
Show(flag, func() core.Node { return P(Text("Visible")) })

ShowElse(loading,
    func() core.Node { return P(Text("Loading\u2026")) },
    func() core.Node { return P(Text("Content")) },
)
```

`flag` can be a `bool`, a `*core.Signal[bool]`, or any expression — if it's a
signal, the shown/hidden scope re-renders on change.

### Switch

```go
Switch(tab, map[string]func() core.Node{
    "home":    homeView,
    "profile": profileView,
}, notFoundView)
```

The first argument is the signal; the matching case renders, and changes
reactively.

### For (keyed lists)

```go
For(entries, func(e Entry) int { return e.ID }, func(e Entry) core.Node {
    return Li(Text(e.Title))
})
```

- First arg: `*core.Signal[[]T]` — the list you want to render.
- Second arg: key function — must return a unique, stable identifier so
  reordering preserves row identity (and element state).
- Third arg: render function — returns a node for each item.

Keyed reconciliation means toggling a checkbox in one row doesn't lose the
list scroll position or refocus an input.

`For` renders only what changed: an unchanged item (compared with `==`) reuses
its mounted row as-is, so appending to a long list renders one row. When the
item behind a key changes, an element row is re-rendered and diffed in place,
a `core.ComponentWithProps` row receives the new item and keeps its state, and
a plain `core.Component` row is remounted (it can only show the values it was
created with).

For very long lists, use `VirtualList` which only renders the visible window:

```go
VirtualList(items, 48 /* row height in px */, func(i int, item Item) core.Node {
    return Li(Text(item.Name))
}, VirtualListHeight(300))
```

---

## Effects — lifecycle and side effects

All effects declare their dependencies explicitly — there is no auto-tracking.

### OnMount

```go
hooks.OnMount(func() func() {
    // Runs once, after mount — the component's DOM is in the document, so
    // refs are set (client only — never during SSR).
    ticker := time.NewTicker(time.Second)
    stop := make(chan struct{})
    go func() {
        for {
            select {
            case <-stop:
                return
            case <-ticker.C:
                core.Schedule(func() { tick() })
            }
        }
    }()
    return func() { close(stop); ticker.Stop() } // cleanup on unmount
})
```

Use `OnMount` for timers, subscriptions, and one-shot fetches. The cleanup
function runs on unmount, so no goroutine leaks.

Hooks are best called in a component's setup. Called directly inside a
reactive region's render function (a `Show` branch, a `UseScope`), they belong
to that render: when the region re-renders, the previous render's `Watch`,
`UseEffect` and `OnMount` are disposed (the new render creates its own), and
all of them are disposed when the region is removed.

### Watch

```go
hooks.Watch([]core.SignalAccessor{count}, func() {
    // Runs when count changes.
    fmt.Println("count is now", count.Get())
})
```

No cleanup. Runs on each dep change — not on mount (use `OnMount` or
`UseEffect` for that).

### UseEffect

```go
hooks.UseEffect([]core.SignalAccessor{userID}, func() func() {
    // Runs on mount and whenever userID changes.
    sub := subscribe(userID.Get())
    return func() { sub.Unsubscribe() } // cleanup on change or unmount
})
```

Combines mount + dep change + cleanup. The cleanup runs before re-running on
a dep change, and on unmount.

---

## Off-loop updates — core.Schedule

Timers, goroutines, network callbacks, and channel receive handlers run
**outside** the render loop. They must not call a setter or `sig.Set()`
directly — that races the renderer. Instead, wrap the update:

```go
core.Schedule(func() {
    setCount(n)
    // Inside Schedule you can read/write signals normally.
})
```

`core.Schedule` enqueues the function on the render loop's frame queue; it's
drained once per frame. The stopwatch tutorial and `UseResource` both use
this internally.

---

## Async data — UseResource

```go
res := hooks.UseResource(nil, func() (string, error) {
    return fetchGreeting() // blocking call, runs in a goroutine
})

return ShowElse(res.Loading,
    func() core.Node { return P(Text("Loading\u2026")) },
    func() core.Node { return P(TextS(res.Data)) },
)
```

`UseResource` returns a `*Resource[T]` with three signal fields:
- `Data` — the loaded value (empty string before first success)
- `Loading` — true while fetching
- `Err` — error string

Plus `Refetch()` to reload. Pass a deps slice to reload on signal change:

```go
res := hooks.UseResource([]core.SignalAccessor{userID}, func() (Profile, error) {
    return loadProfile(userID.Get())
})
```

Fetching is client-side; SSR renders the loading state, and the client fills
it in after hydration.

---

## Error boundaries

Wrap a subtree that might panic during rendering:

```go
ErrorBoundary(
    func(err any) core.Node {
        return P(Style("color:red"), Textf("Something went wrong: %v", err))
    },
    riskyComponent(),
)
```

If `riskyComponent()` panics while rendering — at mount or when a re-render
reaches it — the boundary catches it and renders the fallback; the rest of the
page stays intact. Everything the failed render had set up (effects, bindings,
handlers) is released. A later successful render swaps the child back in. A
panic in a scope re-render outside any boundary is contained too: that scope
keeps its previous DOM, and the panic is logged.

---

## SSR + Hydration

The same component tree renders to HTML on the server:

```go
import "github.com/yogisalomo/goowee/ssr"

body := ssr.New().Render(App(r))
// serve <div id="root">{body}</div> + the WASM entry
```

SSR emits `data-node-id` attributes, `<!--g{id}-->` text markers, and a
`<!--/{id}-->` end anchor after each reactive region (`Show`, `For`, `Switch`,
`Route`, `UseScope`). On the client, the `dom` renderer detects the
server-rendered DOM and **hydrates** — claiming existing nodes (anchors
included) and wiring up event handlers and bindings without rebuilding. The
anchors stay in the DOM: they mark where each region ends, so re-rendered
content is always inserted in the right place.

### Deterministic markup

Hydration trusts that server and client produce the **same markup**. For
content that legitimately differs — dates, locale, per-request data — use one
of:

1. `h.Dynamic()` — wraps a non-deterministic subtree; the client's value wins
   over the server's.
2. Placeholder + `OnMount` — render a placeholder on the server, fill in the
   real content from `OnMount` (client-only).

A structural mismatch (wrong tag type, missing node) logs a clear
`console.error` and recovers best-effort.

---

## Routing

```go
r := router.New(router.CurrentPath())
r.BindHistory()

r.Route(map[string]func() core.Node{
    "/":          homePage,
    "/users/:id": func() core.Node { return userPage(r) },
})
```

Matching is deterministic: exact match wins, then most-specific param/prefix.
Read a URL param reactively:

```go
// Bind to markup — updates in place when :id changes (no remount).
P(Textf("User %s", r.ParamSignal("id")))
```

Use `r.Param("id")` for a one-shot read inside a handler. Navigation:

```go
r.Navigate("/about")
r.NavigateReplace("/redirect")
r.Back()
r.Link("/about", "About", Class("nav")) // an <a> that navigates without a page reload;
                                        // cmd/ctrl/middle-click still opens a new tab
```

### Sub-routes, guards, lazy loading

```go
// Nested routes under a prefix:
r.SubRoute("/admin", adminRoutes)

// Guard checks before routing:
router.Guard(isLoggedIn, loginPage, route)

// Defer initialising a route handler:
router.Lazy(func() core.Node { return heavyPage() })
```

---

## Refs and Portals

Imperative DOM access via refs:

```go
nameRef := Ref()
Input(RefTo(nameRef), Type("text"))
Button(OnClick(func() { nameRef.Focus() }), Text("Focus input"))
```

Commands: `Focus()`, `Blur()`, `Click()`, `ScrollIntoView()`. They run on the
next frame, one-way.

Reading a value back — measuring, scroll position, selection, validity — is
`Get`. The read is answered on the next frame *after* that frame's DOM updates
apply, and the callback runs on the render loop, so it may set signals:

```go
nameRef.Get("offsetWidth", func(v any) {
    w, _ := v.(float64)           // numbers decode as float64
    setWidth(int(w))
})
nameRef.Get("getBoundingClientRect", func(v any) {
    rect, _ := v.(map[string]any) // objects decode as maps
    ...
})
```

A `prop` naming a method (`getBoundingClientRect`, `checkValidity`) is called
with no arguments. `v` is `nil` if the property is undefined or the node is
gone.

`OnMount` runs once the component's DOM is in the document, so measuring
right after mount is direct:

```go
hooks.OnMount(func() func() {
    listRef.Get("clientHeight", func(v any) { ... })
    return nil
})
```

### Handing an element to a JavaScript library

A map, a chart, a rich-text editor, a `<canvas>` context: get the live element
with `bridge.Element(ref)` (js/wasm builds only) from `OnMount`, and tear the
library down in the cleanup:

```go
//go:build js && wasm

mapRef := Ref()
hooks.OnMount(func() func() {
    m := js.Global().Get("L").Call("map", bridge.Element(mapRef))
    return func() { m.Call("remove") }
})
return Div(RefTo(mapRef), Style("height:320px")) // no goowee children
```

Give the library an element whose children goowee doesn't manage (render it
empty), so the library and the renderer never edit the same nodes. The
dashboard example draws its bar chart on a `<canvas>` this way.

Render outside the current subtree — modals, tooltips, dropdowns — with
portals:

```go
Portal("#modal-root",
    Div(Class("modal-overlay"), Div(Class("modal-body"), Text("Hello"))),
)
```

## Two-way bindings

```go
Input(BindValue(name))          // text input ↔ signal (keystroke)
Input(BindValueLazy(name))      // text input ↔ signal (commit on change/blur)
Input(Type("checkbox"), BindChecked(agreed))  // checkbox ↔ bool signal
Select(BindSelect(selected),   // <select> ↔ signal
    Option(Value("a"), Text("A")),
    Option(Value("b"), Text("B")),
)
```

---

## Events

```go
OnClick(func() { handleClick() })
OnInput(func(val string) { handleInput(val) })
OnChange(func(val string) { handleChange(val) })
OnSubmit(func(vals map[string]string) { submit(vals) })

// Full event data:
OnClickE(func(e core.EventData) {
    fmt.Println("click at", e.ClientX(), e.ClientY())
})
```

Helpers cover the common events — `OnFocus`, `OnBlur`, `OnKeyDown`, `OnKeyUp`,
`OnPaste`, `OnCut`, `OnCopy`, `OnFocusIn`, `OnFocusOut`, `OnReset`,
`OnInvalid`, `OnMouseEnter`, `OnMouseLeave`, `OnPointerDown/Move/Up`,
`OnWheel`, `OnContextMenu`, `OnLoad`, `OnError` — and `On("type", fn)` takes any
other.

**Propagation works like the DOM.** A handler runs, then the handlers of the
element's ancestors, innermost first. Events that don't bubble — `focus`,
`blur`, `mouseenter`, `mouseleave`, `invalid`, `load`, `error`, media events —
reach only the element's own handler.

**Handlers decide while they run:**

```go
Textarea(OnKeyDownE(func(e core.EventData) {
    if e.Key() == "Enter" && !e.ShiftKey() {
        e.PreventDefault()  // no newline — this keystroke sends
        e.StopPropagation() // ancestors don't see it
        send()
    }
}))
```

The static options `PreventDefault()` / `StopPropagation()` do the same for
every event. `scroll` and `pointermove` are coalesced to the latest event per
element per frame, and arrive after the fact (they can't be prevented).

`core.EventData` has typed accessors for the payload: `Value`, `Checked`,
`Key`, `Code`, `Repeat`, `CtrlKey`/`ShiftKey`/`AltKey`/`MetaKey`,
`ClientX/Y`, `OffsetX/Y`, `Button`, `Buttons`, `PointerID`, `PointerType`,
`DeltaX/Y/Z`, `DeltaMode`, `Touches`, `ScrollTop/Left`, `FormValues`, `Files`.

The tutorial's **Events** lesson shows all of this live.

### File inputs

On a `change` (or `input`) event from `<input type="file">`, `e.Files()` holds
each picked file's `Name`, `Size`, `Type` and `LastModified`. The contents stay
in the browser until you ask: `Bytes()` blocks on the read, so call it from a
goroutine and apply the result with `core.Schedule` — the same rule as any
fetch (see [Off-loop updates](#off-loop-updates--coreschedule)):

```go
Input(Type("file"), Accept("audio/*"), OnChangeE(func(e core.EventData) {
    for _, f := range e.Files() {
        setStatus("reading " + f.Name)
        go func() {
            data, err := f.Bytes()
            core.Schedule(func() {
                if err != nil { setErr(err); return }
                setAudio(data)
            })
        }()
    }
}))
```

A file stays readable until the input's selection changes or the element is
removed. Calling `Bytes()` inline in the handler would block the WASM event
loop, which is what the read needs to complete — hence the goroutine.
