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

> **Status: experimental (v0).** The core is hardened and tested (see
> `docs/26-07-02-fable-review.md` and the PRs that resolved it), but the public
> API is not yet stable — expect breaking changes. See `docs/plans/roadmap-1.0.md`
> for the path to production readiness, and `docs/canonical/adr.md` for the
> design decisions behind the framework.

> **Building with an AI coding agent?** [`AGENTS.md`](AGENTS.md) is a focused
> guide (mental model, API cheat sheet, golden rules, anti-patterns) that lets an
> agent write correct goowee code without trial and error. Copy it into your
> project as `AGENTS.md`/`CLAUDE.md`, or point your agent instructions at it.

## How it works

- **Signals** are the unit of reactivity. Components are **setup functions that
  run once** (Solid-style, not React): state lives in signals captured by
  closures, and updates happen at fine grain — a signal bound to a node updates
  just that node; a *scope* re-renders a small region and diffs it.
- **The bridge is thin.** All logic is in Go; `runtime/goowee.js` only applies
  DOM mutations, forwards events, and claims server-rendered nodes on hydration.
- **SSR-first.** Render to HTML on the server, then hydrate: the client claims
  the existing DOM instead of rebuilding it.

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

Build and serve:

```bash
GOOS=js GOARCH=wasm go build -o main.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js .
```

```html
<!DOCTYPE html>
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

## Concepts

### Elements — the `h` package

Dot-import `h` and build elements with typed constructors. Attributes,
properties, events, and children are all just items passed to a constructor:

```go
Div(Class("card"), ID("main"),
    H2(Text("Title")),
    Button(OnClick(handleClick), Text("Go")),
)
```

Attributes are typed helpers (`Class`, `Href`, `Type`, `Placeholder`, …);
properties too (`Value`, `Checked`, `Disabled`, …). Text is a child node:
`Text("static")`, `TextS(sig)` (a signal), or `Textf("Count: %d", count)`
(printf where any signal argument is reactive).

Reactive variants of attributes/properties use an `-S` suffix — `Class` is
static, `ClassS(sig)` binds a signal (see ADR-004).

Inline SVG works too: `h.Svg(...)` roots a namespaced subtree (its descendants
inherit the namespace) with shape helpers like `Path`, `Circle`, `Rect`, `G`,
and `Attr("viewBox", …)` for arbitrary attributes.

For imperative DOM access, attach a ref: `ref := h.Ref()`, `h.RefTo(ref)` on the
element, then `ref.Focus()` / `Blur()` / `Click()` / `ScrollIntoView()` from a
handler or effect. Render outside the current subtree — modals, tooltips — with
`h.Portal("#modal-root", …)` (client-side; see ADR-017 for both).

### State

`hooks.UseState` returns a signal and a setter:

```go
count, setCount := hooks.UseState(0)
```

Bind a signal to text or a property and it updates that node when the signal
changes — no component re-render:

```go
Span(TextS(count))          // reactive text
Input(ValueS(name))         // reactive property
Input(BindValue(name))      // two-way (value + oninput)
```

`core.Computed(deps, fn)` derives a signal from others.

### Control flow

Conditional and list rendering re-render a small scope when their deps change:

```go
Show(loggedIn, func() core.Node { return P(Text("Welcome")) })

ShowElse(loading,
    func() core.Node { return Spinner() },
    func() core.Node { return Content() },
)

For(todos, func(t Todo) int { return t.ID }, func(t Todo) core.Node {
    return Li(Text(t.Title))
})

Switch(tab, map[string]func() core.Node{
    "home":    homeView,
    "profile": profileView,
}, notFoundView)
```

`For` is keyed — reordering reuses each row (element *or* component), preserving
its state and DOM.

### Effects

`OnMount` runs setup once and returns a cleanup for unmount — the place for
timers, goroutines, and subscriptions:

```go
hooks.OnMount(func() func() {
    stop := startTicker()
    return func() { stop() } // runs on unmount
})
```

`Watch(deps, fn)` reacts to signal changes; `UseEffect(deps, fn)` runs on mount
and on each change with cleanup. (Effects don't run during server rendering.)

### Routing

```go
import "github.com/yogisalomo/goowee/router"

r := router.New(router.CurrentPath())
r.BindHistory() // pushState on navigation, popstate on back/forward

r.Route(map[string]func() core.Node{
    "/":            homePage,
    "/about":       aboutPage,
    "/users/:id":   func() core.Node { return userPage(r) },
})

r.Link("/about", "About") // navigates without a page reload
```

Matching is deterministic (exact > most-specific prefix/param). Read a URL
param reactively with `r.ParamSignal("id")` so a preserved view updates when the
param changes; `r.Param("id")` is a snapshot for handlers.

### Server-side rendering & hydration

```go
import "github.com/yogisalomo/goowee/ssr"

body := ssr.New().Render(App())
// serve <div id="root">{body}</div> + the wasm loader
```

SSR emits `data-node-id` attributes and text markers. On boot the client
detects the server-rendered DOM and **hydrates** — claiming the existing nodes
and wiring up handlers/bindings instead of rebuilding. `cmd/ssr-server` is a
working example.

Hydration trusts that the server and client render the **same markup**. For
content that legitimately differs — dates, locale, per-request data — wrap the
subtree in `h.Dynamic()` so the client's value is re-applied over the server's,
or render a placeholder and fill it in from `OnMount` (which runs only on the
client). A structural mismatch logs a clear console error. See ADR-016.

### Serving & compression

The WASM binary is the app's one large download and dominates time-to-interactive,
so serve it **compressed**: gzip cuts the example binary ~3.7× (4.2 MB → 1.1 MB).
GitHub Pages and most CDNs do this automatically; the `cmd/ssr-server` reference
server gzips the wasm, JS/CSS, and SSR HTML out of the box. See
[`docs/serving.md`](docs/serving.md).

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
docs/         Design docs, ADRs, roadmap
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
