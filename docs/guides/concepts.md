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
`total.Get()`.

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
- Props that change are passed as **signals**, not plain values.
- A matched component is preserved across parent re-renders (state + effects
  survive).

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
    // Runs once on mount (client only — never during SSR).
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

### Watch

```go
hooks.Watch([]core.SignalAccessor{count}, func() {
    // Runs when count changes.
    fmt.Println("count is now", count.Get())
})
```

No cleanup. Runs once on mount, then on each dep change.

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

If `riskyComponent()` panics while rendering, the boundary catches it and
renders the fallback — the rest of the page stays intact. Update-time panics
are contained (the subtree keeps its previous state) and logged to the
console.

---

## SSR + Hydration

The same component tree renders to HTML on the server:

```go
import "github.com/yogisalomo/goowee/ssr"

body := ssr.New().Render(App(r))
// serve <div id="root">{body}</div> + the WASM entry
```

SSR emits `data-node-id` attributes and text markers. On the client, the
`dom` renderer detects the server-rendered DOM and **hydrates** — claiming
existing nodes and wiring up event handlers and bindings without rebuilding.

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
r.Link("/about", "About") // renders an <a> that navigates without page reload
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

To measure right after mount, defer past setup — the element has no id until
the component's tree is walked:

```go
hooks.OnMount(func() func() {
    core.Schedule(func() { listRef.Get("clientHeight", func(v any) { ... }) })
    return nil
})
```

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

// Full event data and options:
OnClickE(func(e core.EventData) {
    fmt.Println("click at", e.X, e.Y)
}, PreventDefault(), StopPropagation())
```

All standard events are available: `OnFocus`, `OnBlur`, `OnKeyDown`,
`OnKeyUp`, `OnPaste`, `OnCut`, `OnCopy`, `OnFocusIn`, `OnFocusOut`,
`OnReset`, `OnInvalid`, and more.

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
