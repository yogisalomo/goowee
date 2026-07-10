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
| `devtools`| Optional dev-mode inspector: `devtools.Enable()`, `devtools.Snapshot()`. |
| `core`    | **Only the subset listed below.** |

### The public subset of `core`

`core` is a leaf package that mixes the public reactive API with framework
internals (the renderer contract, scheduler, mutation types, component-frame
stack, node structs). Only these identifiers are public:

- **Reactivity:** `Signal[T]`, `NewSignal`, `Computed`, `SignalAccessor`,
  `Schedule` (off-loop updates).
- **Composition:** `Node`, `Component`, `ComponentNode`.
- **Events / misc:** `EventData`, `Ref`, `SVGNamespace`.

Everything else exported from `core` — `Scheduler`, `RenderContext`,
`BindingRegistry`, `Mutation`/`MutationType`, `ComponentFrame`, `PushComponent`/
`PopComponent`, `SaveFrameStack`/`RestoreFrameStack`, `CurrentComponent`,
`CurrentEnv`, `UseContext`, `SetActiveScheduler`, `FlatTree`, `CollectIDs`, the
concrete node structs (`ElementNode`, `TextNode`, …), `Bind`/`Attr`/`Prop`/
`Handler` — exists **only because the renderer packages share it across package
boundaries**. It is not part of the v0 contract; do not depend on it. (It stays
exported rather than hidden because Go's `internal/` visibility works per package,
not per identifier, and splitting `core` into public/internal halves is a larger
refactor tracked separately.)

## Internal — do not import

`internal/dom` (the DOM renderer, diff/reconciliation, hydration, and event
registry) is under `internal/`, so the Go toolchain **prevents** any module
outside goowee from importing it. There is no supported way to drive the renderer
directly; go through `bridge.Run` on the client and `ssr` on the server.

`runtime/goowee.js` is the JS bridge that applies mutations and forwards events.
It is an implementation detail of the client and not a stable interface.

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

`bridge.Run` replaces the old `dom.New()` / `renderer.Render` / `bridge.Init`
sequence, which required reaching into renderer internals (`renderer.Scheduler`,
`renderer.Registry`) that are now hidden.
