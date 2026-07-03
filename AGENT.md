# AGENT.md

## Commands

```bash
go test ./...              # run all tests
go build ./...             # verify compilation (non-WASM)
GOOS=js GOARCH=wasm go build -o main.wasm ./examples/counter  # WASM build
make serve                 # WASM + dev server on :8083
make serve-ssr             # WASM + SSR server on :8081
```

## Code conventions

- No comments in production code. Let the code speak.
- No emojis in code or docs unless explicitly asked.
- Prefer editing existing files over creating new ones.
- Match existing patterns (signal, hook, node, etc.).
- Avoid adding new external dependencies.
- WASM-only code goes in `bridge/` with `//go:build js && wasm` guard.
- Pure-Go code tested with `go test ./...` (no WASM dependency).

## Architecture

```
Application code (Go)
  → hooks (UseState, UseEffect, UseScope)
    → core (signals, scheduler, bindings, nodes)
      → dom (renderer, diff, event registry)
        → bridge (WASM interop) → browser DOM
```

- Reactivity: signals → BindingRegistry → scheduler → JS bridge
- Structural updates: UseScope → diffNode → scheduler
- Components are VNode tree entries (`ComponentNode`) with hook state tracked in `ComponentFrame`
- No virtual DOM diffing as primary mechanism; only targeted re-renders

## Key packages

| Package | Role |
|---------|------|
| `core` | `Signal[T]`, `Node` types, `Scheduler`, `BindingRegistry`, `ComponentFrame` |
| `hooks` | `UseState`, `UseEffect`, `UseScope` |
| `dom` | `DOMRenderer` (initial render + diff), `NodeRegistry` (events) |
| `html` | Element helpers (`Div`, `Span`, etc.), `VirtualList` |
| `router` | `Router`, `Route`, `Link` |
| `ssr` | Server-side HTML renderer + hydration metadata |
| `bridge` | `Bridge` interface, `NoopBridge`, WASM `Init` |

## Testing patterns

- Pure unit tests: `core/`, `hooks/`, `ssr/` — run with `go test`
- DOM tests: `dom/` — use `DOMRenderer` directly, inspect mutation output
- No browser or WASM required for any test
- Test `ScopeNode` re-renders by changing signals and flushing the scheduler
