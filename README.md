# Goowee

A Go framework for building reactive web UIs, compiled to WebAssembly, with
server-side rendering and hydration.

**[Live demo + tutorial →](https://yogisalomo.github.io/goowee/)** (the site is itself a goowee app)

```go
import . "github.com/yogisalomo/goowee/h"

func App() core.Node {
    return core.Component("App", func() core.Node {
        count, setCount := hooks.UseState(0)
        return Div(
            P(Textf("Count: %d", count)),
            Button(OnClick(func() { setCount(count.Get() + 1) }), Text("Click me")),
        )
    })
}
```

> **Status: experimental (v0).** The core is hardened and tested, but the public
> API is not yet stable — expect breaking changes.
> See [`docs/plans/roadmap-1.0.md`](docs/plans/roadmap-1.0.md) for the path to 1.0.

> **Building with an AI coding agent?**
> [`AGENTS.md`](AGENTS.md) is a focused agent guide (mental model, golden rules,
> API cheat sheet, copy-paste patterns, anti-patterns). Copy it into your project.

## Quick start

Requires Go 1.25+.

```bash
go mod init myapp
go get github.com/yogisalomo/goowee
```

Create `main.go`:

```go
//go:build js && wasm

package main

import (
    . "github.com/yogisalomo/goowee/h"
    "github.com/yogisalomo/goowee/bridge"
    "github.com/yogisalomo/goowee/core"
    "github.com/yogisalomo/goowee/dom"
    "github.com/yogisalomo/goowee/hooks"
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
        return Div(Class("counter"),
            P(Textf("Count: %d", count)),
            Button(OnClick(func() { setCount(count.Get() + 1) }), Text("Click me")),
        )
    })
}
```

Build, copy the runtime, and serve:

```bash
GOOS=js GOARCH=wasm go build -o web/main.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js web/
cd web && python3 -m http.server 8080
```

```html
<!-- web/index.html (minimal) -->
<script src="wasm_exec.js"></script>
<script src="goowee.js"></script>
<div id="root"></div>
<script>
    const go = new Go();
    WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject)
        .then(r => go.run(r.instance));
</script>
```

The runnable reference app lives in `examples/counter` — `make serve` (client)
or `make serve-ssr` (SSR + hydration).

## Guides

| Guide | What you'll learn |
|-------|-------------------|
| [Getting Started](docs/guides/getting-started.md) | Project scaffold, first component, WASM build, adding routing and SSR |
| [Concepts](docs/guides/concepts.md) | Signals, run-once components, Show/For, effects, SSR, routing, refs, bindings |
| [API Reference](docs/api/reference.md) | Complete public surface for core, h, hooks, router, ssr, dom, bridge |

## Project layout

```
core/         Signals, scheduler, node types, bindings, render context
dom/          DOM renderer, diff/reconciliation, hydration, event registry
hooks/        UseState, UseEffect, OnMount, Watch, UseScope
h/            Typed element/attr/event DSL, control flow, VirtualList
router/       Client-side router (matching, params, history)
ssr/          Server-side HTML renderer
bridge/       WASM bridge (Go ↔ JS)
runtime/      JS runtime (goowee.js)
examples/     Demo app (counter, form, todos, dashboard, routing, params)
cmd/          SSR server binary
test/e2e/     Headless-browser smoke test
docs/         Design docs, ADRs, roadmap, guides, API reference
```

## Building and running

```bash
make test        # go test ./...
make bench       # benchmarks
make size        # WASM binary size vs budget
make e2e         # headless-Chrome end-to-end smoke test (needs Node + Chrome)
make serve       # build WASM and serve on :8083
make serve-ssr   # SSR-rendered server on :8081
```

## License

MIT — see [LICENSE](LICENSE).
