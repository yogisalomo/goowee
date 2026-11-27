# API Reference

The public surface of goowee spans these packages. Import them as:

```go
import (
    "github.com/yogisalomo/goowee/core"
    . "github.com/yogisalomo/goowee/h"       // dot-imported DSL
    "github.com/yogisalomo/goowee/hooks"
    "github.com/yogisalomo/goowee/router"
    "github.com/yogisalomo/goowee/net"       // fetch + WebSocket
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
s.Peek() T                               // read without counting as a dependency (dev checks)
s.Named("cart")                          // a label in ?goowee-dev's inspector and change log; no-op otherwise

// Group writes: each written signal notifies once, when fn returns. Event
// handlers, Schedule callbacks, ref reads and effects are batched already.
core.Batch(func())

// Derived signal that recomputes when its input changes. Prefer these: the
// dependency and its read are one expression, so they cannot drift apart.
total := core.Derive(items, func(xs []Item) int { return sum(xs) })
full  := core.Derive2(first, last, func(f, l string) string { return f + " " + l })

// For three or more deps, keep Computed and build the list with Deps.
total := core.Computed(core.Deps(items, tax, discount), func() int {
    return sum(items.Get(), tax.Get(), discount.Get())
})                                       // *core.Signal[int]
```

### Types

```go
type SignalAccessor interface { Value() any; Subscribe(func()) func() } // *Signal[T]

type Node interface { /* implemented by the node types below */ }
type ElementNode struct { ID int; Tag, Namespace string; Key any; Attrs; Props; Binds; Handlers; Children []Node; … }
type TextNode struct { ID int; Value any }  // a string, or a signal (reactive)
type ComponentNode struct { Name string; Key any; Render func() Node; … }

// Components: setup runs once, at mount. Identity is name + key — a
// different key remounts.
core.Component(name string, setup func() core.Node) *ComponentNode
// A preserved instance receives new props through its props signal.
core.ComponentWithProps(name string, props P, setup func(*core.Signal[P]) core.Node) *ComponentNode
core.SetKey(node core.Node, key any) core.Node // returns node: `return core.SetKey(Card(m), m.ID)`
type ScopeNode struct { ... }         // created by Show/For/Switch
```

### Context (tree-scoped values)

```go
var Session = core.NewContext(Session{})          // the default, for consumers with no provider
Session.Provide(sig *core.Signal[Session], children ...core.Node) core.Node  // in scope below; nearest wins
Session.Use(func(s *core.Signal[Session]) core.Node) core.Node              // the nearest provider's signal
```

- Bind the signal (`TextS`, `core.Computed`, a `Show` dep) to follow changes;
  `Use`'s function re-runs like a `Show`'s content, so keep state in a
  component inside it.
- SSR-safe: each render resolves its own providers, so concurrent requests
  never see each other's values. Provide request state (the session) near the
  root of the per-request tree.

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

// Head (see docs/guides/metadata.md): Metadata(Title("…"), Meta(…), Link(…), Page(PageMeta{…}))
TitleS(sig core.SignalAccessor)         // a document title that follows sig; the first title in the tree wins

// Raw HTML, inside a <div> (untrusted input must be sanitized):
Raw(html string)                        // renders pre-rendered HTML verbatim
RawS(sig *core.Signal[string])          // replaces the HTML when sig changes
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
OnFocus(func()), OnBlur(func())         // target only (don't bubble)
OnKeyDown(func(key string)), OnKeyUp(func(key string))
OnPaste(func(text string))              // the pasted plain text
OnCut(func(text string)), OnCopy(func(text string))   // the selected text
OnPasteE(func(core.EventData))          // pasted files (screenshots), text, types
OnDragEnter/OnDragOver/OnDragLeave(func(core.EventData))   // drop zone: PreventDefault on dragover to accept
OnDrop(func(core.EventData))            // dropped files, text, position
OnFocusIn(func()), OnFocusOut(func())   // bubble
OnReset(func()), OnInvalid(func())
OnMouseEnter(func()), OnMouseLeave(func())   // target only
OnLoad(func()), OnError(func())              // <img>, <script>, media; target only
OnPointerDown/OnPointerMove/OnPointerUp(func(core.EventData))
OnWheel(func(core.EventData)), OnContextMenu(func(core.EventData))
OnScrollEnd(func(scrollTop float64))         // target only
OnBeforeInput(func(core.EventData))          // e.InputData(); PreventDefault rejects the edit
OnToggle(func(open bool))                    // <details>, popover; target only
OnClose(func(returnValue string)), OnCancel(func(core.EventData))   // <dialog>; target only
OnTransitionEnd/OnAnimationEnd(func(core.EventData))               // bubble
OnPlay/OnPause/OnEnded/OnTimeUpdate/OnSeeked/OnVolumeChange/OnLoadedMetadata(func(core.EventData))
                                             // <video>, <audio>; target only
On("type", func(core.EventData))             // any event type
```

Handlers bubble through ancestors, innermost first; non-bubbling events
(focus, blur, mouseenter/leave, invalid, load, error, toggle, dialog close
and cancel, scrollend, media) reach the target only.

Media events carry the playback state at the moment they fired, so a custom
player needs no `Ref` reads to show it. `Duration()` is 0 until the metadata
loads and `+Inf` for a live stream. Control playback through a Ref
(`ref.Call("play")`) or `BindProp("currentTime", sig)`.

**Full event data** — receive `core.EventData` with position, key, target, etc.:

```go
OnClickE(func(core.EventData), ...options)

// Decide while handling (dispatch is synchronous):
e.PreventDefault(), e.StopPropagation()

// Accessors on core.EventData:
e.Value() string, e.Checked() bool, e.InputType() string
e.Key(), e.Code() string, e.Repeat(), e.IsComposing() bool
e.CtrlKey(), e.ShiftKey(), e.AltKey(), e.MetaKey() bool
e.ClientX(), e.ClientY(), e.OffsetX(), e.OffsetY() float64
e.Button(), e.Buttons() int             // 0 primary, 1 middle, 2 secondary
e.PointerID() int, e.PointerType() string, e.Pressure() float64
e.DeltaX(), e.DeltaY(), e.DeltaZ() float64, e.DeltaMode() int   // wheel
e.Touches(), e.ChangedTouches() []core.Touch
e.ScrollTop(), e.ScrollLeft() float64
e.FormValues() map[string]string        // submit
e.Files() []core.File                   // file input change/input, paste, drop (not dragover)
e.DataText() string                     // paste/drop: the text/plain data; cut/copy: the selection
e.DataTypes() []string                  // paste/drag: "text/plain", "text/html", "Files", …
e.InputData() string                    // input/beforeinput: inserted text; composition*: composed text
e.CurrentTime(), e.Duration(), e.Volume(), e.PlaybackRate() float64   // media events
e.Paused(), e.Ended(), e.Seeking(), e.Muted() bool, e.ReadyState() int
e.Buffered() []core.TimeRange, e.MediaError() (code int, message string)
e.NewState(), e.OldState() string       // toggle/beforetoggle: "open" or "closed"
e.ReturnValue() string                  // dialog close/cancel
e.PropertyName(), e.AnimationName(), e.PseudoElement() string, e.ElapsedTime() float64
e.NaturalWidth(), e.NaturalHeight() float64   // <img> load

// core.File: Name, Size, Type, LastModified; contents on demand.
f.Bytes() ([]byte, error)               // blocks — call from a goroutine, apply via core.Schedule
```

Options (static, apply to every event): `PreventDefault()`,
`StopPropagation()`, `SelectOnFocus()`.

### Two-way bindings

```go
BindValue(sig *core.Signal[string])          // input ↔ signal (keystroke)
BindValueLazy(sig *core.Signal[string])      // input ↔ signal (on change/blur)
BindNumber(sig *core.Signal[T])              // numeric input ↔ number signal
BindChecked(sig *core.Signal[bool])          // checkbox ↔ signal
BindRadio(sig *core.Signal[string], value string) // radio group ↔ signal
BindSelect(sig *core.Signal[string])         // <select> ↔ signal
BindSelectMulti(sig *core.Signal[[]string])  // <select multiple> ↔ signal
BindFiles(sig *core.Signal[[]core.File])     // file input ↔ picked files
```

### Reactive attributes and styles

```go
AttrS(name string, sig core.SignalAccessor)  // any attribute; false removes bool attrs
BindAttr(name string, sig core.SignalAccessor) // old spelling; false writes "false"
ClassS(sig core.SignalAccessor)              // whole class attribute
ClassIf(name string, sig *core.Signal[bool]) // one class, composes with Class
Classes(map[string]*core.Signal[bool])       // several independent classes
StyleProp(name string, sig core.SignalAccessor) // one CSS property
StylePx(name string, sig core.SignalAccessor)   // one CSS property, numeric + "px"
HiddenS(sig *core.Signal[bool])              // hidden attribute
AriaExpandedS / AriaPressedS / AriaSelectedS / AriaCheckedS … // "true"/"false"
```

### Control flow

```go
// Conditional rendering.
Show(cond interface{}, then func() core.Node)
ShowElse(cond interface{}, then func() core.Node, else_ func() core.Node)
ShowWhen(sig *core.Signal[T], cond func(T) bool, then func() core.Node)  // derives the bool condition

// Multi-way switch.
Switch(sig *core.Signal[T], cases map[T]func() core.Node, default func() core.Node)

// Keyed list rendering.
For[T, K comparable](sig *core.Signal[[]T], keyFn func(T) K, render func(T) core.Node)

// Virtualized (windowed) list for large datasets. keyOrHeight is an int row
// height (legacy index keys) or func(T) K (stable item keys).
VirtualList[T any](sig *core.Signal[[]T], keyOrHeight any, render func(int, T) core.Node, opts ...VirtualListOption)

VirtualListHeight(px int) VirtualListOption      // viewport height (default 400)
VirtualListWindow(px int) VirtualListOption      // alias
VirtualListItemHeight(px float64) VirtualListOption // estimate when using key fn
VirtualListOverscan(n int) VirtualListOption
VirtualListMeasure() VirtualListOption              // real row sizes via ResizeObserver
VirtualListHorizontal() VirtualListOption
VirtualListWindowScroll() VirtualListOption
VirtualListStickToBottom() VirtualListOption
VirtualListOnEndReached(fn func()) VirtualListOption
VirtualListKeyFunc[T, K](func(T) K) VirtualListOption
VirtualListOnHandle[T](func(*VirtualListHandle[T])) VirtualListOption

handle.ScrollToIndex(i int)
handle.ScrollToKey(key string)
handle.Refresh()
```

### Refs and portals

```go
Ref() *core.Ref                          // create a ref handle
RefTo(ref *core.Ref)                     // attach to an element
// Commands (one-way, next frame):
ref.Focus()
ref.Blur()
ref.Click()
ref.ScrollIntoView()
// A method with arguments, after the frame's DOM updates (so after a value
// the same handler set). Offsets are UTF-16 units, as in JS:
ref.Call(method string, args ...any)    // ref.Call("setSelectionRange", at, at)
// Read a value back (answered after the next frame's DOM updates; fn runs
// on the render loop). v: float64 | string | bool | map[string]any | nil.
// A prop naming a method (getBoundingClientRect) is called with no args.
ref.Get(prop string, fn func(v any))
// Draw a goowee/canvas buffer on a <canvas> (after the frame's DOM updates):
ref.Draw(cmds []uint32)                 // ref.Draw(ctx.Commands())

Portal(target string, children ...core.Node)  // render into another container
// (client-side; reconciled on re-render, so state inside survives)
```

### Error boundary

```go
ErrorBoundary(fallback func(err any) core.Node, child core.Node)
```

### Dynamic content (SSR hydration escape hatch)

```go
Dynamic() core.Item                      // on an element: client value wins on hydration
```

### Helpers

```go
Fragment(children ...core.Node)          // groups nodes, no wrapper element (a Node)
Empty()                                  // renders nothing: `return Empty()` from a func returning core.Node
Group(items ...core.Item) core.Item      // bundles attrs/events/children into one Item (not a Node)
Nodes(nodes []core.Node) []core.Item     // spread built nodes: El("div", Nodes(children)...)
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
hooks.OnMount(func() func())                  // run once after mount (DOM applied), return cleanup
hooks.Watch1(sig, func(T))                    // react to one signal, passed by value
hooks.Watch([]core.SignalAccessor{deps}, func())  // react to signal changes
hooks.UseEffect([]core.SignalAccessor{deps}, func() func())  // mount + deps + cleanup

// Persistence: load sig from localStorage once mounted, save it (JSON) on each change
hooks.UsePersist(sig, "cart")                 // also hooks.PersistOptions{Store: storage.Session}

// Scope (for manual scope re-renders — internal use for Show/For/Switch)
hooks.UseScope()                              // *core.ScopeNode

// Async data
res := hooks.UseResource[T]([]core.SignalAccessor{deps}, func(ctx context.Context) (T, error)) // ctx cancelled on refetch/unmount
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
r.QueryParam(name string) string                       // snapshot; a repeated key gives its last value
r.QueryValues(name string) []string                    // every value: ?tag=a&tag=b gives [a b]
r.QueryParamSignal(name string) *core.Signal[string]   // reactive
r.SetQueryParam(name, value string)                    // replaceState; "" removes it
r.SetQueryParamPush(name, value string)                // pushState
// Both write parameters sorted by name, keep the fragment, and drop "?" when none are left.
r.Fragment                                             // *core.Signal[string]: the URL's #fragment, without "#"

// Navigation.
r.Navigate(url string)          // "/r/2?t=20#end": Path "/r/2", Query {t: 20}; no query clears it
r.NavigateReplace(url string)   // same, replaceState; "?t=3" keeps the path, "#x" the path and query
r.Back()
r.Forward()
r.Link(path, text string, items ...core.Item) *core.ElementNode // <a>; plain clicks navigate in-app,
                                              // modifier/middle clicks go to the browser
router.InAppClick(e core.EventData) bool     // the same test, for custom link handlers

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

body, head := ssr.New().Render(node core.Node)  // body HTML + <head> HTML from Metadata

// A net/http handler for whole documents: concurrent renders, gzip,
// status from the page (404 via the router), fallback on panic/ClientOnly.
ssr.Handler(ssr.HandlerOptions{
    Page       func(*http.Request) ssr.Page      // ssr.Page{Node, Status func() int}
    Document   func(w io.Writer, head, body string)
    Fallback   http.Handler                     // e.g. the client-only index.html
    ClientOnly func(*http.Request) bool
}) http.Handler
ssr.RoutedPage(app func(*router.Router) core.Node) func(*http.Request) ssr.Page

// An island in any server page (html/template, templ); bridge.Islands hydrates it.
ssr.Island(name string, node core.Node, props any) (template.HTML, error)

// router, for servers:
router.NewURL(u *url.URL) *router.Router   // path + query from a request URL
r.NotFound() bool                          // last Route render hit the 404 fallback
```

The SSR renderer produces plain HTML that opens with `<!--goowee-->`
(`ssr.HydrationMarker`), with comments only for scope ends, adjacent and
empty text. The client-side `dom` renderer detects the marker during boot
and hydrates, claiming the server's nodes in order, instead of rebuilding.

---

## `gooweetest` — headless component tests

```go
import "github.com/yogisalomo/goowee/gooweetest"

s := gooweetest.Render(t, node)       // mount; flushed; unmounted when the test ends
s.Find(sel) / s.Query(sel) / s.FindAll(sel) / s.FindByText(text) *Element
s.Click(el), s.Input(el, v), s.Change(el, v), s.Check(el, bool), s.Focus(el)
s.Submit(form) bool, s.KeyDown(el, key) bool, s.Dispatch(el, event, data) bool  // true = prevented
s.Flush(), s.WaitFor(func() bool), s.WaitForText(text)
s.Text(), s.HTML(), s.Head(), s.Title(), s.Portal(selector), s.Focused(), s.Unmount()
el.Text(), el.HTML(), el.Attr(name), el.HasAttr(name), el.Value(), el.Checked(), el.Prop(name), el.Children(), el.Find(sel)
```

Selectors: tag, `#id`, `.class`, `[attr]`, `[attr=value]`, descendant (space).

Files, paste and drop (contents come back from `File.Bytes`):

```go
f := gooweetest.File{Name: "shot.png", Type: "image/png", Data: png}
s.Paste(el, text, files...) bool          // paste; true = prevented
s.Drop(el, files...) bool                 // drop; dispatch "dragover" with DragOverData() for the accept step
s.SelectFiles(input, files...)            // file input: input + change
el.Calls() []gooweetest.Call              // ref.Call history: {Method, Args}
el.Canvas() []canvas.Command              // the last goowee/canvas buffer drawn on a <canvas>, decoded
el.DrawCount() int                        // how many frames drew it
el.Prop("selectionStart")                 // caret, kept by Input, setSelectionRange, value writes
```

A fake `goowee/net` for components that fetch or open sockets (install it
before `Render`; not for parallel tests):

```go
n := gooweetest.NewFakeNet(t)
n.Handle("GET /api/user", func(r net.Request) (net.Response, error) {  // net/http.ServeMux patterns
    return gooweetest.JSONResponse(200, User{Name: "Ada"})            // unmatched → 404
})
n.Handle("POST /api/", gooweetest.Status(204))   // everything under /api/; "/rooms/{id}" wildcards
n.Handle("/", gooweetest.Status(204))            // every request nothing more specific matches
ws := s.WaitForSocket(n, "/ws")                  // flushes until the app has an open socket there
es := s.WaitForStream(n, "/api/events")          // an open net.Subscribe stream: Send, SendEvent, Fail(reconnecting), Reopen
n.SetReconnectDelay(0)                           // sockets reconnect without their real backoff
n.Requests() []net.Request                 // what the app sent (JSON already in Body)
ws := n.Socket("/ws")                      // latest socket dialed to that path (query ignored)
ws.Send(text), ws.SendBinary(b), ws.SendJSON(v)  // to the app; Flush to run its OnMessage
ws.Received() []net.Message, ws.Close(code, reason), ws.Closed(), ws.URL
```

---

## `net` — fetch and WebSocket

The browser's `fetch` and `WebSocket` without `net/http` (which adds ~7 MB to
the WASM binary). Outside the browser (SSR, native tests) `Fetch` returns
`net.ErrUnavailable` and sockets never open; `gooweetest.NewFakeNet` fakes both.

```go
import "github.com/yogisalomo/goowee/net"

// Blocks until the body has arrived: call from a goroutine (UseResource's
// fetch runs in one). err = no response (network, CORS, ctx done → ctx.Err()).
net.Fetch(ctx, net.Request{
    Method: "POST", URL: "/api/x",            // Method "" = GET; URL relative to the page
    Header: map[string]string{"X-Token": t},
    JSON:   v,                                // or Body []byte; JSON sets Content-Type
    Credentials: "include",                   // "" = same-origin
}) (net.Response, error)
resp.Status, resp.StatusText, resp.URL, resp.Header /* lowercase names */, resp.Body
resp.OK() bool, resp.Text() string
resp.JSON(&v) error                           // a non-2xx status → *net.StatusError

// Upload: a multipart body; files go as the browser's own File (no copy through Go).
form := new(net.Form).Add("caption", c).AddFile("photo", f)   // f: a core.File from e.Files()
net.Fetch(ctx, net.Request{Method: "POST", URL: "/api/photos", Form: form})

// Server-sent events (EventSource): reconnects by itself, sending Last-Event-ID.
sub := net.Subscribe("/api/events", net.SubscribeOptions{
    Events:  []string{"presence"},              // named types, besides "message"
    OnOpen:  func(),
    OnEvent: func(e net.Event),                 // e.Type, e.Data, e.ID
    OnError: func(reconnecting bool),           // false: the browser gave up
})
sub.Close()

// A WebSocket; returns at once. Callbacks run on the render loop (no
// core.Schedule) and stop after Close.
sock := net.Dial("/ws", net.SocketOptions{      // ws(s):// or relative (page's scheme)
    Protocols: []string{"v1"},
    OnOpen:    func(),
    OnMessage: func(m net.Message),             // m.Text(), m.Data, m.Binary
    OnClose:   func(e net.CloseEvent),          // e.Code, e.Reason, e.Reconnecting
    Reconnect: true,                            // backoff MinBackoff (1s) doubling to MaxBackoff (30s), jittered
    ReconnectURL: func() string,                // e.g. "/ws?since=" + cursor
})
sock.Send(text) error, sock.SendBinary(b) error // net.ErrNotOpen unless connected; never blocks
sock.Close()                                    // code 1000, stops reconnecting

net.SetTransport(t net.Transport) (previous net.Transport) // swap the network (tests, other hosts)
```

---

## `storage` — localStorage and sessionStorage

```go
import "github.com/yogisalomo/goowee/storage"

storage.Local.Get(key) (string, bool)     // also storage.Session
storage.Local.Set(key, value) error       // ErrUnavailable where storage is disabled; quota errors
storage.Local.Remove(key), .Clear(), .Keys() []string
```

Read it in `OnMount` (client-only): during a server render it is empty and
writes do nothing. To keep a signal in storage, `hooks.UsePersist(sig, key)`
does the loading and saving. Outside the browser it's an in-memory store, which
gooweetest clears when a test ends. See [the testing guide](../guides/testing.md).

---

## `canvas` — drawing on a `<canvas>` from Go

```go
import "github.com/yogisalomo/goowee/canvas"

canvas.View(opts ...canvas.Option) core.Node        // a <canvas> drawn by a draw function
canvas.Size(w, h float64)                           // required; CSS pixels
canvas.Draw(deps []core.SignalAccessor, fn func(c *canvas.Ctx)) // required; runs on mount and when a dep changes
canvas.Label(text)                                  // role="img" + aria-label
canvas.Attrs(items ...core.Item)                    // class, id, handlers (style is the View's)
canvas.Fallback(children ...core.Node)              // content inside the <canvas>
canvas.NoClear()                                    // draw on top of the last picture

c := canvas.NewCtx(w, h)                            // record by hand; ref.Draw(c.Commands())
c.Reset(w, h); c.Width(); c.Height(); c.Commands() []uint32
// State and transforms
c.Save(); c.Restore(); c.Translate(x, y); c.Scale(x, y); c.Rotate(rad); c.Transform(a, b, c, d, e, f)
// Styles
c.FillStyle(css); c.StrokeStyle(css); c.LineWidth(w); c.GlobalAlpha(a); c.LineDash(seg...)
c.LineCap(canvas.CapButt|CapRound|CapSquare); c.LineJoin(canvas.JoinMiter|JoinRound|JoinBevel)
c.Font(css); c.TextAlign(canvas.AlignStart|…|AlignCenter); c.TextBaseline(canvas.BaselineAlphabetic|…)
// Paths
c.BeginPath(); c.ClosePath(); c.MoveTo(x, y); c.LineTo(x, y); c.Arc(x, y, r, a0, a1)
c.Rect(x, y, w, h); c.QuadTo(cx, cy, x, y); c.CubicTo(c1x, c1y, c2x, c2y, x, y)
c.Fill(); c.Stroke(); c.Clip()
// Rectangles and text
c.ClearRect(…); c.FillRect(…); c.StrokeRect(…); c.FillText(x, y, s); c.StrokeText(x, y, s)
// Bulk: one command for many marks ([]float32; panic on a ragged length)
c.Segments(v)  // x1 y1 x2 y2 …, added to the path
c.Circles(v)   // x y r …, added to the path
c.Rects(v)     // x y w h …, added to the path
c.Polyline(v)  // x y …, one connected line
c.FillRects(v) // x y w h …, filled, no path
// Conveniences
c.Line(x1, y1, x2, y2); c.Circle(x, y, r)

canvas.Decode(buf []uint32) (w, h float64, flags uint32, cmds []canvas.Command, err error)
cmd.String()                                        // "FillRect(0, 0, 10, 10)"
```

Every draw starts from the default context state, on a cleared canvas unless
`NoClear`. The buffer format is versioned (`canvas.Version`, which equals
`core.DrawVersion`); serve the `goowee.js` of the same goowee version as the
app. See [Canvas](../guides/concepts.md#canvas--drawing-from-go).

---

## `rpc` — typed server functions

```go
import "github.com/yogisalomo/goowee/rpc"

var AddNote = rpc.Define[AddReq, Note]("notes.add")      // shared by server and client

rpc.Handle(mux, AddNote, func(ctx context.Context, req AddReq) (Note, error) { … },
    rpc.Before(auth), rpc.AllowCrossSite(), rpc.MaxBody(n)) // server only (not in wasm builds)
note, err := AddNote.Call(ctx, req)                      // blocks: goroutine or UseQuery
res := rpc.UseQuery(f, reqSignal)                        // *hooks.Resource[Res], refetches on change
rpc.Fake(t, f, fn)                                       // stub one function in a test
rpc.Invalid(msg, fields) / rpc.Errorf(code, format, ...) // *rpc.Error {Code, Message, Fields, Status}
rpc.BasePath = "/_rpc"
```

See [Talking to your server](../guides/server.md).

## `bridge` — WASM entry point

```go
import "github.com/yogisalomo/goowee/bridge"

bridge.Run(app core.Node)                       // Mount("#root", app) + Wait()
bridge.Mount(selector string, n core.Node) *bridge.Root // non-blocking; several roots per binary
bridge.Islands(map[string]func(props json.RawMessage) core.Node) []*bridge.Root // hydrate ssr.Island markup
bridge.Wait()                                   // block forever; call last in main
(*bridge.Root).Unmount()

// js/wasm only: the live DOM element a ref points at, for JS libraries.
// js.Undefined() before render / after removal. Call from OnMount or later.
bridge.Element(ref *core.Ref) js.Value
// js/wasm only: the browser's File behind a core.File (picked, pasted,
// dropped), for a JS library or browser API, without copying its bytes.
bridge.FileValue(f core.File) js.Value
```

`Run` mounts `app` into the page, hydrates it if the HTML was server-rendered,
and drives the render loop. It never returns (keeps the WASM module alive).
Called once from `main()` — that's all you need for the client side.
