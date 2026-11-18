# Devtools & observability

goowee ships structured logging at every recover boundary and an optional
dev-mode inspector for the component/scope tree and signal graph (roadmap P4.2).

## Structured logging

Recover boundaries and reactive warnings emit entries via `core.Log` with a
stable `kind` field:

| Kind | When |
|------|------|
| `recover.event_handler` | Panic in a DOM event handler |
| `recover.error_boundary` | Panic caught by `h.ErrorBoundary` (render or update) |
| `recover.re_render` | Panic during a scope re-render (subtree kept) |
| `signal.cycle` | Signal notification cycle detected |
| `warn` | Non-fatal issues (duplicate list keys, invalid SSR attrs) |

On the host these print as `goowee[kind] message key=value …`. In the browser
the WASM bridge forwards the same payload to `goowee.log()`, which routes
`recover.*` to `console.error`, `signal.cycle`/`warn` to `console.warn`, and
everything else to `console.log`.

## Dev-mode inspector

Enable devtools with either:

- URL query: `?goowee-dev` or `?goowee-dev=1`
- `localStorage.setItem("goowee-dev", "1")` then reload

Then open the browser console and run:

```js
goowee.inspect()
```

This prints and returns a snapshot with:

- **`tree`** — mounted component/scope/element tree (paths, hook signal ids, scope deps)
- **`signals`** — registered signals with id, kind, owner path, current value, subscriber count

Signals are registered automatically from `UseState`, `UseResource`, and
`Computed` when devtools are enabled. A signal made with `core.NewSignal`,
such as one in a store, appears once it is named or first written.

### Naming signals

Give the signals you'll look for a name, so the inspector and the change log
show `cart` rather than a number:

```go
type Store struct {
    User *core.Signal[User]
    Cart *core.Signal[[]Item]
}

func NewStore() *Store {
    return &Store{
        User: core.NewSignal(User{}).Named("user"),
        Cart: core.NewSignal([]Item(nil)).Named("cart"),
    }
}
```

`Named` returns the signal, so it chains like `WithEquals`, and works on a
`UseState` or `Computed` signal too. Without dev mode it does nothing. A store
in a package-level variable is named before the app knows whether dev mode is
on; the browser build holds those names until it does. The router names its
own signals `router.path`, `router.query` and `router.params`.

## Change log

In dev mode every `Set` that changes a value is logged: the signal, its value
before and after, and the code that wrote it. Run

```js
goowee.changes()                  // prints a table and returns the rows
goowee.changes({ clear: true })   // empties the log
```

| Field | |
|-------|-|
| `seq`, `ms` | order, and milliseconds since dev mode started |
| `signal`, `name` | the inspector id, and the `Named` name if any |
| `from`, `to` | the values, formatted with `%+v` (cut at 200 characters) |
| `at`, `func` | where the write happened: `app/store.go:31`, `app.(*Store).AddToCart` |

`at` and `func` are the first caller outside goowee, so a write through a
`UseState` setter or a store method points at your handler or method. A write
goowee makes itself, such as a two-way binding or a router link, points at the
goowee code that made it (`h/binds.go`, `router/router.go`). A `Computed`
isn't written, so it doesn't appear; the signals it derives from do. The log
keeps the last 500 changes.

Dev mode tracks reads and writes in globals, so it is for a browser tab, not
for a server rendering requests concurrently.

## Programmatic use

```go
import "github.com/yogisalomo/goowee/devtools"

devtools.Enable()          // host tests / custom tooling
snap := devtools.Snapshot() // map[string]any, JSON-serializable
log := devtools.Changes()   // []devtools.Change, oldest first
devtools.ClearChanges()
```

`bridge.Run` enables devtools automatically when `goowee.devEnabled()` is true.

## Production builds without dev tools

Dev mode is switched on at run time (`?goowee-dev`), so by default its code is
in every build: the inspector, the change log, and the dependency checks with
their `runtime.Callers` and `FuncForPC` lookups. Build with
`-tags goowee_nodev` to leave all of that out:

```sh
GOOS=js GOARCH=wasm go build -tags goowee_nodev -o dist/main.wasm ./cmd/app
```

The build links no `goowee/devtools` code (a test checks the wasm's name
section), and `core.SetDevChecks` does nothing in it. `?goowee-dev` then turns
nothing on, and `goowee.inspect()` and `goowee.changes()` print "built without
dev tools". Everything else works the same, and the E2E smoke test runs on
both builds. On the minimal app the tag saves about 105 KB of wasm, or 15 KB
after brotli.

Builds without the tag keep today's behavior, so `?goowee-dev` works on any of
them.

