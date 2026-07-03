# Goowee

A Go framework for building reactive web UIs compiled to WebAssembly.

```go
func App() core.Node {
    count, setCount := hooks.UseState(0)
    return html.Button(html.Props{
        "textContent": count,
        "onclick": func(core.EventData) { setCount(count.Get() + 1) },
    })
}
```

## Quick start

Requires Go 1.25+.

```bash
go mod init myapp
go get goowee
```

Create `main.go`:

```go
//go:build js && wasm

package main

import (
    "goowee/bridge"
    "goowee/core"
    "goowee/dom"
    "goowee/html"
    "goowee/hooks"
)

func main() {
    renderer := dom.New()
    muts, _ := renderer.Render(App())
    renderer.Scheduler.Enqueue(muts...)
    bridge.Init(renderer.Scheduler, renderer.Registry)
    select {}
}

func App() core.Node {
    return core.Component("App", func() core.Node {
        count, setCount := hooks.UseState(0)
        return html.Div(nil,
            html.P(html.Props{"textContent": count}),
            html.Button(html.Props{
                "textContent": "Click me",
                "onclick":     func(core.EventData) { setCount(count.Get() + 1) },
            }),
        )
    })
}
```

Build:

```bash
GOOS=js GOARCH=wasm go build -o main.wasm .
```

Copy the runtime JS and Go's wasm_exec.js next to your binary:

```bash
cp $GOROOT/misc/wasm/wasm_exec.js .
cp $(go env GOMODCACHE)/goowee@latest/runtime/goowee.js .
```

Serve it all with any static file server:

```html
<!DOCTYPE html>
<script src="wasm_exec.js"></script>
<script src="goowee.js"></script>
<script>
    const go = new Go();
    WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject)
        .then(r => go.run(r.instance));
</script>
```

## Concepts

### Nodes

Every component returns a `core.Node`. Build them with the `html` package helpers or directly:

```go
// helper
html.Div(html.Props{"class": "container"}, html.Text("hello"))

// or raw structs
&core.ElementNode{
    Tag: "div",
    Props: map[string]any{"class": "container"},
    Children: []core.Node{&core.TextNode{Value: "hello"}},
}
```

Available helpers: `Div`, `Span`, `P`, `H1`–`H6`, `A`, `Ul`, `Ol`, `Li`, `Form`, `Input`, `Button`, `Label`, `Select`, `Option`, `Textarea`, `Nav`, `Main`, `Section`, `Header`, `Footer`, `Article`, `Aside`, `Img`, `Br`, `Hr`, `Table`, `Thead`, `Tbody`, `Tr`, `Th`, `Td`, `Strong`, `Em`, `Code`, `Pre`, `Fragment`, `Text`.

### State

`hooks.UseState` returns a signal and a setter:

```go
count, setCount := hooks.UseState(0)
```

Pass the signal directly to a prop for automatic reactivity:

```go
html.Span(html.Props{"textContent": count})
```

The signal binding is set up once during render. When you call `setCount(n)`, the DOM node updates automatically — no component re-render needed for property changes.

### Structural updates: UseScope

For conditional rendering or lists, wrap the dynamic part in `hooks.UseScope`:

```go
show, setShow := hooks.UseState(true)

return html.Div(nil,
    hooks.UseScope(func() core.Node {
        if show.Get() {
            return html.P(html.Props{"textContent": "Now you see me"})
        }
        return nil
    }, show),
    html.Button(html.Props{
        "textContent": "Toggle",
        "onclick":     func(core.EventData) { setShow(!show.Get()) },
    }),
)
```

The scope re-executes when `show` changes, diffs the old and new trees, and emits only the mutations needed to update the DOM.

### Effects

For side effects that respond to signal changes:

```go
hooks.UseEffect([]core.SignalAccessor{count}, func() func() {
    fmt.Println("count is now", count.Get())
    return nil // optional cleanup
})
```

Effects run once immediately, then re-run when any dependency changes. Return a cleanup function to tear down subscriptions or timers.

### Routing

```go
import "goowee/router"

r := router.New("/")
r.SetNavFn(func(path string) {
    // update browser URL via history.pushState
})

// define routes
r.Route(map[string]func() core.Node{
    "/":      homePage,
    "/about": aboutPage,
})

// link without page reload
r.Link("/about", "About us")
```

### VirtualList

Render large lists efficiently — only visible items are in the DOM:

```go
html.VirtualList(itemsSignal, itemHeight, func(i int, item Item) core.Node {
    return html.Li(nil, html.Text(item.Name))
}, html.VirtualListHeight(400))
```

### Server-side rendering

Serve pre-rendered HTML from your Go server:

```go
import "goowee/ssr"

renderer := ssr.New()
html := renderer.Render(app.App(router.New(path)))
```

The SSR output includes `data-node-id` attributes for hydration metadata.

## Project layout

```
core/       Node types, signals, scheduler, bindings
dom/        DOM renderer, diff algorithm, event registry
hooks/      UseState, UseEffect, UseScope
bridge/     WASM bridge (Go ↔ JS interop)
html/       Element helpers and VirtualList
router/     Client-side router
ssr/        Server-side HTML renderer
runtime/    JS runtime (goowee.js)
examples/   Demo app with 7 pages
cmd/        SSR server binary
```

## Building and running

```bash
make test       # run all Go tests
make serve      # build WASM and serve on :8083
make serve-ssr  # build + SSR-rendered server on :8081
```
