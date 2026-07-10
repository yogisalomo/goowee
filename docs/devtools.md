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
`Computed` when devtools are enabled.

## Programmatic use

```go
import "github.com/yogisalomo/goowee/devtools"

devtools.Enable()          // host tests / custom tooling
snap := devtools.Snapshot() // map[string]any, JSON-serializable
```

`bridge.Run` enables devtools automatically when `goowee.devEnabled()` is true.
