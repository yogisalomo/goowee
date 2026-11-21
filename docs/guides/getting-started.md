# Getting Started

Build a reactive web UI in Go, compiled to WebAssembly, in a minute.

## Prerequisites

- Go 1.25+
- A text editor

## Quick start

```bash
go install github.com/yogisalomo/goowee/cmd/goowee@latest
goowee new myapp
cd myapp
goowee dev
```

Open http://localhost:8080. `goowee dev` builds the app to WebAssembly,
serves it, and on every save rebuilds and reloads the page; a build error
shows over the page instead of a stale app. Dev mode (the inspector and the
dependency checks, [devtools.md](../devtools.md)) is on.

What `goowee new` wrote:

```
myapp/
  app/app.go        the UI: App, a router with two pages, a Counter component
  app/app_test.go   gooweetest tests: go test ./... runs them without a browser
  main.go           the browser entry point (//go:build js && wasm)
  web/index.html    the page shell: a loading message, the scripts, goowee.boot
  AGENTS.md         layout, commands and rules, for you and for coding agents
```

`goowee new myapp --ssr` adds `server/`, which renders each page on the
server ([Adding SSR](#adding-ssr)); `goowee dev` then runs it and restarts it
on every change. `--tinygo` builds with TinyGo, about a third of the size
([serving.md](serving.md)).

## A component

`app/app.go`'s `Counter`:

```go
func Counter() core.Node {
	return core.Component("Counter", func() core.Node {
		count, setCount := hooks.UseState(0)
		label := core.Computed([]core.SignalAccessor{count}, func() string {
			return "Clicked " + strconv.Itoa(count.Get()) + " times"
		})
		return Section(
			H1(Text("myapp")),
			Button(ID("count"), OnClick(func() { setCount(count.Get() + 1) }), TextS(label)),
		)
	})
}
```

The component function runs once. `count` is a signal; `TextS(label)` binds
the button's text to it, so a click updates that text node and nothing else.

## Building for production

```bash
goowee build
```

`dist/` gets the WASM (symbols stripped, dev tools left out), `goowee.js` and
`wasm_exec.js` under content-hashed names that `index.html` refers to,
`.gz` siblings (and `.br` when the `brotli` command is installed), and a
report of the WASM's size by package. Serve the hashed files with
`Cache-Control: immutable` ([serving.md](serving.md)).

## Without the CLI

`goowee` only automates the build:

```bash
GOOS=js GOARCH=wasm go build -o web/main.wasm .
go run github.com/yogisalomo/goowee/cmd/goowee assets ./web   # goowee.js and wasm_exec.js
cd web && python3 -m http.server 8080
```

`assets` copies the `goowee.js` and `wasm_exec.js` that match this build:
goowee's version comes from your `go.mod`, the loader from the Go toolchain.
A stale `goowee.js` is rejected at boot with a message naming both versions
and the command to fix it.

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

The client detects the server-rendered DOM (via `data-node-id` attributes) and
**hydrates** — claiming existing nodes and wiring up handlers instead of
rebuilding. See `../guides/concepts.md` for details.

## Next steps

- [Concepts guide](concepts.md) — signals, run-once components, control flow, effects
- [API reference](../api/reference.md) — complete public surface
- [Tutorial](https://yogisalomo.github.io/goowee/) — live interactive lessons in the browser
- `examples/counter/app/app.go` — the full reference app with all features
