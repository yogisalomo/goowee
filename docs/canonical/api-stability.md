# API stability & the public/internal boundary

goowee is **v0 (experimental)**. This document defines what is part of the public
API, what is off-limits, and what "v0" promises about breaking changes.

## Stability policy (v0)

- **v0 means breaking changes are allowed** between minor releases (`v0.x`), but
  they will be called out in the [CHANGELOG](../CHANGELOG.md) and, where a rename
  is involved, the old name will be kept for one minor release with a
  `// Deprecated:` comment pointing at the replacement.
- Anything **not** listed as public below may change or disappear at any time
  without notice, even in a patch release.
- When goowee reaches **v1**, the public surface below is frozen under semver:
  no breaking change without a major-version bump.

## Public packages

These are the supported API. Import them freely.

| Package | What you use it for |
|---------|--------------------|
| `h`       | The typed DSL: elements, attributes, events, control flow (`Show`/`For`/`Switch`), `Svg`, `Ref`/`Portal`, `ErrorBoundary`. |
| `hooks`   | `UseState`, `UseEffect`, `Watch`, `OnMount`, `UseResource`, `UseScope`. |
| `router`  | `Route`, params (`ParamSignal`), navigation, guards, sub-routes, base path. |
| `ssr`     | Server-side rendering: `ssr.New().Render(node)`. |
| `bridge`  | The client entry point: `bridge.Run(node)`. |
| `core`    | **Only the subset listed below.** |

### The public subset of `core`

`core` is a leaf package that mixes the public reactive API with framework
internals (the scheduler contract, mutation types, component-frame definition,
node structs). Only these identifiers are public:

- **Reactivity:** `Signal[T]`, `NewSignal`, `Computed`, `SignalAccessor`,
  `Schedule` (off-loop updates), `RegisterDisposer`.
- **Composition:** `Node`, `Component`, `ComponentNode`.
- **Events / misc:** `EventData`, `Ref`, `SVGNamespace`.

Everything else exported from `core` — `Scheduler`, `Mutation`/`MutationType`,
`ComponentFrame`, `SetActiveScheduler`, `FlatTree`, the concrete node structs
(`ElementNode`, `TextNode`, …), `Handler`, `HandlerOptions` — exists **only
because the renderer packages share it across package boundaries**. It is not
part of the v0 contract; do not depend on it.

Some identifiers **were** in `core` but have been moved to `internal/runtime/`
as part of the P0.3 boundary enforcement: `RenderContext`, `BindingRegistry`,
`PushComponent`, `PopComponent`, `SaveFrameStack`, `RestoreFrameStack`,
`CurrentComponent`, `CurrentEnv`, `UseContext`, `CollectIDs`. These are now
inaccessible to external modules — the Go compiler enforces the boundary.

## Internal — do not import

- `internal/dom` — the DOM renderer, diff/reconciliation, hydration, and event
  registry. Go's `internal/` mechanism **prevents** any module outside goowee
  from importing it. Go through `bridge.Run` on the client and `ssr` on the
  server.
- `internal/runtime` — the component-frame stack, render context, binding
  registry, and ID collection (moved here from `core` in P0.3). Same
  `internal/` protection.
- `runtime/goowee.js` — the JS bridge that applies mutations and forwards
  events. It is an implementation detail of the client and not a stable
  interface.

## The one entry point you need

```go
//go:build js && wasm
package main

import (
    "github.com/yogisalomo/goowee/bridge"
    "github.com/yogisalomo/goowee/router"
    "yourmodule/app"
)

func main() {
    r := router.New(router.CurrentPath())
    r.BindHistory()
    bridge.Run(app.App(r)) // mounts, hydrates if server-rendered, runs the loop
}
```

That's it — no direct import of `dom`, no manual scheduler wiring, no hydration
check. `bridge.Run` handles all of that internally.
