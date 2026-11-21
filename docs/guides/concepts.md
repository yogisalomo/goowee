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
  `return core.SetKey(Card(name), name)` (SetKey returns the node it keys).

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

### ForList (lists that change one row at a time)

`For` takes a `*Signal[[]T]`: every change replaces the slice, and `For`
compares the whole list with the rows it has. For long lists that change one
row at a time, use a `core.List` and `ForList`. The list records each
change, and `ForList` applies just that change: an append mounts one row, an
update sets one row's item signal, and a removal or move touches one row.
That costs the same at 10,000 rows as at 10.

```go
todos := core.NewList(func(t Todo) int { return t.ID }, initial...)

Ul(ForList(todos, func(t *core.Signal[Todo]) core.Node {
    title := core.Computed([]core.SignalAccessor{t}, func() string { return t.Get().Title })
    return Li(TextS(title))
}))

todos.Append(Todo{ID: 4, Title: "milk"})
todos.Update(4, func(t *Todo) { t.Title = "oat milk" }) // only this row's text changes
todos.Move(4, 0)
todos.Remove(4)
todos.Set(all) // a full replace: rows are matched by key, as For does
```

Each row gets its item as a signal, so bind what depends on it. A `List` is
a `SignalAccessor`: put it in a `Computed`'s dependencies and read
`list.Items()` to derive a count or a filter. Keys must be unique.

### Checking dependencies in dev mode

Dependencies are declared by hand, so a missing one is a silent bug — the
region just doesn't update. In dev mode (`?goowee-dev`), goowee records the
signals a `Show`/`For`/`Switch`/`Route`/`UseScope` render or a `Computed`
reads and warns — once per read site, with its `file:line` — about any that
isn't declared:

```go
For(todos, key, func(t Todo) core.Node {
    return Li(Class(ifElse(selected.Get() == t.ID, "on", ""))) // ⚠ selected isn't a dep of For
})
```

Fix it by binding the value (`ClassS(…)` over a `Computed`), adding the dep,
or — for a deliberate snapshot — reading with `sig.Peek()`, which is never
counted.

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
res := hooks.UseResource(nil, func(ctx context.Context) (string, error) {
    return fetchGreeting(ctx) // blocking call, runs in a goroutine
})

return ShowResource(res,
    func() core.Node { return P(Text("Loading\u2026")) },
    func(err error) core.Node { return P(Textf("Error: %v", err)) },
    func(data *core.Signal[string]) core.Node { return P(TextS(data)) },
)
```

`UseResource` returns a `*Resource[T]` with three signal fields:
- `Data` — the last loaded value (the zero value before the first success)
- `Loading` — true while fetching
- `Err` — the last error (`nil` on success)

Plus `Refetch()` to reload. Pass a deps slice to reload on signal change:

```go
res := hooks.UseResource([]core.SignalAccessor{userID}, func(ctx context.Context) (Profile, error) {
    return loadProfile(ctx, userID.Get())
})
```

`fetch`'s context is cancelled when its result is no longer wanted — a newer
load started (dep change, `Refetch`) or the component unmounted — so pass it
to your HTTP request or query and the work stops early. A superseded result is
ignored either way.

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

http.Handle("/", ssr.Handler(ssr.HandlerOptions{
    Page:     ssr.RoutedPage(App), // App(r *router.Router) core.Node
    Document: func(w io.Writer, head, body string) {
        fmt.Fprintf(w, `<!DOCTYPE html><html><head>%s
<script src="wasm_exec.js"></script><script src="goowee.js"></script></head>
<body><div id="root">%s</div><script>goowee.boot()</script></body></html>`, head, body)
    },
    Fallback: http.FileServer(http.Dir("web")), // client-rendered shell, used if a render panics
}))
```

`ssr.Handler` renders each request concurrently, answers **404** when the
router falls through to its not-found route (`router.NotFound`), serves
`Fallback` (typically your client-only `index.html`) when a render panics or
for paths you mark `ClientOnly`, and gzips the HTML. Lower level:
`body, head := ssr.New().Render(node)`.

SSR emits `data-node-id` attributes, `<!--g{id}-->` text markers, and a
`<!--/{id}-->` end anchor after each reactive region (`Show`, `For`, `Switch`,
`Route`, `UseScope`). On the client, the `dom` renderer detects the
server-rendered DOM and **hydrates** — claiming existing nodes (anchors
included) and wiring up event handlers and bindings without rebuilding. The
anchors stay in the DOM: they mark where each region ends, so re-rendered
content is always inserted in the right place.

### Before the app is interactive

A server-rendered page looks ready before the WASM has downloaded and
booted. What the user does in that gap isn't lost:

- **Clicks, keydowns and submits** on the server's markup are queued from
  the moment `goowee.js` loads until the first frame. They are then
  dispatched to their handlers in order, once each. A form submit is held
  back meanwhile, then resubmitted, so `OnSubmit` handles it. A form without
  a handler then submits natively. If the WASM fails to load, held submits go
  through natively.
- **Form state** the user changed (typed text, a ticked box, a selection) is
  announced to Go as `input` and `change` events after hydration, so
  `BindValue`, `BindChecked` and `BindSelect` signals take what the page shows.
- **Styling.** `<html data-goowee-pending>` is set until then, to style "not
  ready yet": `[data-goowee-pending] button { cursor: progress; }`.

This needs `goowee.js` in `<head>` as a plain `<script>`, not `defer` or
`async`, so it is listening before the body is parsed. That is the setup in
`cmd/ssr-server` and the Pages shells.

### Data on the server

`hooks.UseResource` fetches in the browser, so a server-rendered page shows
its loading state. `hooks.Load` is the same resource with a key, and the
server loads it too:

```go
hooks.Load("user:"+id, nil, func(ctx context.Context) (User, error) {
    return api.GetUser.Call(ctx, api.UserReq{ID: id})
}, func(res *hooks.Resource[User]) core.Node {
    return h.ShowResource(res, loading, failed, view)
})
```

Under `ssr.Handler` (or `ssr.Export`), the server runs each `Load`'s fetch
before sending the page, renders it with the data, and embeds the data as
JSON. In the browser the resource starts with it and doesn't fetch again;
after that it refetches on a dep change or `Refetch`, as `UseResource` does.
Keys name the data in the page, so include what it depends on. The data must
be JSON-encodable. A fetch that fails or outlasts `DataTimeout` (5 s by
default) is left to the client. See ADR-030.

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

Use `r.Param("id")` for a one-shot read inside a handler.

A route's content is keyed by the pattern that matched it. Moving to another
route mounts the new route's content fresh, even when both routes render a
component of the same name. The same pattern with new params (`/users/1` to
`/users/2`) keeps the mounted content, which reads the new values through
`ParamSignal`.

Navigation:

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

To draw on a `<canvas>` yourself, use `goowee/canvas` (next section) rather
than one `syscall/js` call per drawing operation.

Render outside the current subtree — modals, tooltips, dropdowns — with
portals:

```go
Portal("#modal-root",
    Div(Class("modal-overlay"), Div(Class("modal-body"), Text("Hello"))),
)
```

Portals render on the client only (the server leaves the target empty). When
the component holding a portal re-renders, the portal's content is diffed like
any other children, so a form or component inside a modal keeps its state.

## Canvas — drawing from Go

`goowee/canvas` draws charts, graphs and visualizations on a `<canvas>`. A draw
function records commands into a `canvas.Ctx` (a buffer of 32-bit words), and
the runtime replays the whole buffer onto the canvas's 2D context in one call
per frame. With thousands of marks that is several times faster than calling
the canvas API through `syscall/js`, and since recording is plain Go it works in
server rendering and in `gooweetest`.

```go
import "github.com/yogisalomo/goowee/canvas"

func SalesChart(sales *core.Signal[[]float64]) core.Node {
    return canvas.View(
        canvas.Size(280, 120),                   // CSS pixels
        canvas.Label("Sales per day, as bars"),  // role="img" + aria-label
        canvas.Draw([]core.SignalAccessor{sales}, func(c *canvas.Ctx) {
            c.FillStyle("#00728f")
            for i, v := range sales.Get() {
                c.FillRect(float64(i)*40+4, c.Height()-v, 32, v)
            }
        }),
    )
}
```

The draw function runs once the canvas is in the document and again whenever a
dep changes; each run records the whole picture, on a cleared canvas
(`canvas.NoClear()` keeps the previous one). Like `h.VirtualList`, `View` reads
its options once: the draw function is captured at mount, so it must read
signals, listed in its deps, to see new data.

- **Coordinates are CSS pixels.** The runtime sizes the backing store for the
  screen's device pixel ratio, so drawings are sharp on high-density screens.
  Pointer events on the canvas report `OffsetX`/`OffsetY` in the same units.
- **Group marks by style.** Once the crossing into JavaScript is cheap, the
  canvas API is the cost. Draw each color as one path and one `Fill`, and use
  the bulk methods for many marks: `Segments`, `Circles`, `Rects`, `Polyline`,
  `FillRects`, which take a `[]float32` and record one command for all of them.
- **Reuse slices between frames.** A `Ctx` keeps its buffer, so recording into
  one that has grown to fit the scene doesn't allocate; keep your `[]float32`
  slices in the component and truncate them (`s = s[:0]`) each run.
- **Accessibility.** Give a canvas that carries information a `Label`, or a
  `Fallback` (a table of the data, say) rendered inside the `<canvas>`.
- **Server and tests.** The draw function never runs during SSR, which renders
  the empty, labelled `<canvas>`. In `gooweetest`, `Element.Canvas()` returns the
  recorded commands (`cmds[0].String() == "FillStyle(\"#00728f\")"`) and
  `Element.DrawCount()` how many frames drew.

To draw from somewhere other than a `View` (a handler, a goroutine via
`core.Schedule`), record into your own `canvas.NewCtx(w, h)` and send it with
`ref.Draw(c.Commands())`; give that `<canvas>` the same CSS size.

### Following the element's size

`canvas.Fill()` replaces `Size`: the canvas fills its parent (which needs a
height of its own), the runtime watches its size, and the picture is redrawn
when it changes, with `c.Width()` and `c.Height()` giving the new size.
Nothing is drawn until the canvas has been laid out. A change of the device
pixel ratio (browser zoom, another screen) replays every canvas at the new
ratio by itself.

### Animation

`core.NextFrame(fn)` runs `fn` once at the start of the next frame, on the
render loop; calling it again from `fn` keeps animating at the display's
refresh rate. Frames stop while the page is hidden. `canvas.Animate` does this
for a View, drawing one picture per frame from mount to unmount:

```go
canvas.View(
    canvas.Fill(),
    canvas.Animate(func(c *canvas.Ctx, dt time.Duration) {
        angle += dt.Seconds()            // dt: time since the last picture
        c.FillStyle("#00add8")
        c.BeginPath()
        c.Arc(c.Width()/2+80*math.Cos(angle), c.Height()/2, 12, 0, 2*math.Pi)
        c.Fill()
    }),
)
```

The first picture after a hidden page is shown again gets the whole hidden
time as `dt`, so clamp it when the motion integrates it. In `gooweetest`,
animations advance only with `screen.Frame()`, one picture per call;
`screen.ResizeCanvas(el, w, h)` stands in for the layout of a `Fill` canvas.

### Layers and hit testing

`canvas.Stack(box, layers...)` puts one canvas per layer in a box, so a layer
that changes rarely isn't redrawn with one that changes often. Each layer is a
`Draw` or an `Animate`; `box` is a `Size` or `Fill`, combined with `Label` or
`Attrs` through `canvas.Options`:

```go
canvas.Stack(
    canvas.Options(canvas.Size(600, 400), canvas.Attrs(h.OnPointerMove(hover))),
    canvas.Draw([]core.SignalAccessor{graph}, drawEdges),     // slow, rare
    canvas.Draw([]core.SignalAccessor{hovered}, drawHighlight), // cheap, often
)
```

`canvas.Grid` finds the mark under the pointer: `Insert(id, x, y, r)` each
mark as you draw (after a `Reset`), then `At(e.OffsetX(), e.OffsetY())` in a
pointer handler. Once grown, it doesn't allocate.

The tutorial's [Canvas lesson](https://yogisalomo.github.io/goowee/canvas)
animates up to 50,000 dots this way, and its
[graph](https://yogisalomo.github.io/goowee/graph) redraws 10,000 nodes and
10,000 edges every frame (`examples/graph`; `make canvasbench` measures it).

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

---

## Testing — gooweetest

`gooweetest` renders components headlessly: an in-memory DOM applies the same
mutation stream the browser runtime does, events go through the real handlers
(with DOM bubbling), effects run after each frame, and `core.Schedule` works —
no browser needed.

```go
func TestCounter(t *testing.T) {
    s := gooweetest.Render(t, Counter())
    s.Click(s.FindByText("Increment"))
    if got := s.Find("p.count").Text(); got != "Count: 1" {
        t.Fatalf("got %q", got)
    }
}
```

- Find elements with `Find`/`Query`/`FindAll` (CSS subset: tag, `#id`,
  `.class`, `[attr]`, `[attr=value]`, descendants) or `FindByText`.
- Interact with `Click`, `Input`, `Change`, `Check`, `Submit`, `KeyDown`,
  `Focus`, or any event via `Dispatch`; each flushes, so assertions see the
  updated DOM. `Submit`/`KeyDown`/`Dispatch` report whether the default action
  was prevented.
- Read with `Text`, `HTML`, `Head`, `Portal(selector)`, `Focused`, and per
  element `Text`, `Attr`, `Value`, `Checked`, `Prop`.
- Async work (`UseResource`, goroutines + `Schedule`): `WaitFor(cond)` /
  `WaitForText(s)`.
- `Unmount` runs every cleanup (Render does it at the end of the test).

There's no layout: `ref.Get` of sizes and positions returns `nil`. For real
browser behavior, see the headless-Chrome E2E harness in `test/e2e`.

