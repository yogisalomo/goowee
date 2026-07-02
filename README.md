# Goowee

A Go framework for building web UIs with WebAssembly.

## How it works

The application runs in the browser as a WASM binary. A VDOM tree is built from Go structs, then diffed against the previous tree to produce DOM mutations. Signals track state changes and trigger targeted re-renders of only the affected parts of the tree.

## Project structure

```
core/       - Node types, signals, scheduler, bindings
dom/        - VDOM renderer, diff algorithm, event handler registry
hooks/      - UseState, UseScope, UseEffect
bridge/     - WASM bridge (Go <-> JS interop)
html/       - Element helper functions and VirtualList component
router/     - Client-side router
ssr/        - Server-side rendering
runtime/    - JS runtime (goowee.js)
examples/   - Counter app (also serves as integration test)
cmd/        - CLI entry points (ssr-server)
```

## Getting started

Requires Go 1.25.1 or later.

```
make test       # run all Go tests
make serve      # build WASM and serve examples/counter on :8083
make serve-ssr  # build WASM + SSR server
```

Open http://localhost:8083 after running `make serve`.

## Building for production

```
GOOS=js GOARCH=wasm go build -o main.wasm ./examples/counter
```

Copy the runtime JS and Go's wasm_exec.js next to the binary, then serve the directory.

## Components

A component is a function that returns a Node:

```go
func Greeting(name string) core.Node {
    return html.P(html.Props{"textContent": "Hello " + name})
}
```

## State

UseState returns a signal and a setter:

```go
count, setCount := hooks.UseState(0)
```

Signals are reactive. Reading them inside a UseScope subscribes the scope to the signal. Changes trigger re-render of only that scope.

## Router

```go
r.Route(map[string]func() core.Node{
    "/": homePage,
    "/about": aboutPage,
})
```

Links use `r.Link` to navigate without page reloads.
