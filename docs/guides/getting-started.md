# Getting Started

Build a reactive web UI in Go, compiled to WebAssembly, in five minutes.

## Prerequisites

- Go 1.25+
- A text editor

## 1. Scaffold a project

```bash
go mod init myapp
go get github.com/yogisalomo/goowee@latest
```

Create the minimal layout:

```
myapp/
  cmd/
    app/
      main.go    WASM entry point
  app/
    app.go       component tree
  web/
    index.html   page shell
```

## 2. Write a component

`app/app.go`:

```go
package app

import (
    "github.com/yogisalomo/goowee/core"
    . "github.com/yogisalomo/goowee/h"
    "github.com/yogisalomo/goowee/hooks"
)

func App() core.Node {
    return core.Component("App", func() core.Node {
        count, setCount := hooks.UseState(0)
        return Div(Class("counter"),
            P(Textf("Count: %d", count)),
            Button(OnClick(func() { setCount(count.Get() + 1) }), Text("Click me")),
        )
    })
}
```

A component is a **setup function that runs once on mount** (Solid-style, not
React). State lives in a **signal** (`count`). `Textf` binds the signal to the
text node, so clicking the button updates just that number — no re-render, no
virtual DOM.

## 3. Create the WASM entry point

`cmd/app/main.go`:

```go
//go:build js && wasm

package main

import (
    "github.com/yogisalomo/goowee/bridge"
    "github.com/yogisalomo/goowee/router"
    "myapp/app"
)

func main() {
    r := router.New(router.CurrentPath())
    r.BindHistory()
    bridge.Run(app.App(r))
}
```

## 4. Create the HTML shell

`web/index.html`:

```html
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>myapp</title>
    <script src="wasm_exec.js"></script>
    <script src="goowee.js"></script>
</head>
<body>
    <div id="root"></div>
    <script>
        const go = new Go();
        WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject)
            .then(r => go.run(r.instance));
    </script>
</body>
</html>
```

For a production page shell, see `examples/counter/index.html` which includes
a `counter.js` bootstrap that handles loading states.

## 5. Build and serve

```bash
GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js web/
cd web && python3 -m http.server 8080
```

Open `http://localhost:8080`. You should see a counter that increments when
clicked.

## Adding routing

```go
import "github.com/yogisalomo/goowee/router"

func App(r *router.Router) core.Node {
    return core.Component("App", func() core.Node {
        return r.Route(map[string]func() core.Node{
            "/":     homePage,
            "/about": aboutPage,
        })
    })
}

func homePage() core.Node {
    return H1(Text("Home"))
}
```

The entry point stays the same — just pass the router to your app component:

```go
r := router.New(router.CurrentPath())
r.BindHistory()
bridge.Run(app.App(r))
```

## Adding SSR

The same component tree can render to HTML on the server:

```go
import "github.com/yogisalomo/goowee/ssr"

body := ssr.New().Render(App(r))
// wrap in <div id="root">{body}</div> and serve with the WASM loader
```

The client detects the server-rendered DOM (via `data-node-id` attributes) and
**hydrates** — claiming existing nodes and wiring up handlers instead of
rebuilding. See `../guides/concepts.md` for details.

## Next steps

- [Concepts guide](concepts.md) — signals, run-once components, control flow, effects
- [API reference](../api/reference.md) — complete public surface
- [Tutorial](https://yogisalomo.github.io/goowee/) — live interactive lessons in the browser
- `examples/counter/app/app.go` — the full reference app with all features
