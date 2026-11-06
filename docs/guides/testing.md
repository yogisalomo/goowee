# Testing components

`gooweetest` renders components on your machine, with no browser: it applies
the renderer's real DOM updates to an in-memory document, dispatches events
through the real handlers, and runs effects and `core.Schedule` callbacks.

```go
func TestCounter(t *testing.T) {
    s := gooweetest.Render(t, Counter())
    s.Click(s.FindByText("Increment"))
    if got := s.Find("p.count").Text(); got != "Count: 1" {
        t.Fatalf("got %q", got)
    }
}
```

Interactions flush, so assertions see the updated DOM. For results that arrive
from goroutines (`hooks.UseResource`, a fetch), use `s.WaitFor` or
`s.WaitForText`.

## Keep components buildable on the host

A test runs natively, so the package it renders must build without
`GOOS=js`. The one thing that breaks that is importing `syscall/js`, so reach
the browser through goowee instead. Each of these works in the browser and has
a host version that tests can drive:

| Browser API | Use | In tests |
|---|---|---|
| `fetch` | `net.Fetch` (`goowee/net`) | `gooweetest.NewFakeNet(t)`, `n.Handle(pattern, fn)`, `n.Requests()` |
| `FormData` upload | `net.Request{Form: …}` | the handler reads `r.Form.Value`, `r.Form.File(…).Bytes()` |
| `WebSocket` | `net.Dial` | `s.WaitForSocket(n, path)` → `Send`, `Received`, `Close` |
| `EventSource` | `net.Subscribe` | `s.WaitForStream(n, path)` → `Send`, `SendEvent`, `Fail` |
| `localStorage`, `sessionStorage` | `storage.Local`, `storage.Session` | in memory: seed before `Render`; cleared when the test ends |
| `location`, `history` | the router: `router.CurrentPath()`, `r.ParamSignal`, `r.QueryParamSignal`, `r.Navigate` | `router.New("/the/path")` |
| a pasted or dropped file | `e.Files()`, `f.Bytes()` | `s.Paste`, `s.Drop`, `s.SelectFiles` with `gooweetest.File` |
| `setInterval`, `setTimeout` | a goroutine (`time.Ticker`) + `core.Schedule` | runs as is; `s.WaitFor` |
| DOM methods (`focus`, `setSelectionRange`) | a `Ref`: `ref.Focus()`, `ref.Call(…)` | `s.Focused()`, `el.Calls()` |

```go
func TestProfile(t *testing.T) {
    n := gooweetest.NewFakeNet(t)
    n.Handle("GET /api/user", func(r net.Request) (net.Response, error) {
        return gooweetest.JSONResponse(200, User{Name: "Ada"})
    })
    n.Handle("/", gooweetest.Status(204)) // everything else the app posts
    s := gooweetest.Render(t, Profile())
    s.WaitForText("Ada")
}
```

## Browser APIs goowee doesn't cover

For anything else (`Notification`, the Clipboard API, a JavaScript library),
keep the `syscall/js` code behind a small seam, so the component's package
still builds on the host:

```
ui/platform.go        // the interface (or plain functions) components call
ui/platform_js.go     // //go:build js && wasm: the syscall/js implementation
ui/platform_other.go  // //go:build !(js && wasm): a stand-in tests control
```

```go
// platform.go
package ui

// Notify shows a system notification. A variable, so tests can record calls.
var Notify = notify

// platform_js.go
//go:build js && wasm

func notify(title, body string) {
    js.Global().Get("Notification").New(title, map[string]any{"body": body})
}

// platform_other.go
//go:build !(js && wasm)

func notify(title, body string) {}
```

```go
// a test
var shown []string
ui.Notify = func(title, body string) { shown = append(shown, title) }
```

Two more rules keep a component the same in both builds:

- JavaScript callbacks (an event listener, a timer) run off the render loop:
  set signals inside `core.Schedule`, as any off-loop code does.
- A JavaScript library that owns a DOM element gets it in `OnMount`, through
  `bridge.Element(ref)`. Put that call in a `_js.go` file too.
