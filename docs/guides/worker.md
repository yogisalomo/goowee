# Running the app in a Web Worker

By default the Go program runs on the page's main thread, so its garbage
collection, a large re-render or a long computation in a handler stop the
page from painting, scrolling and taking input until they finish. Worker mode
runs the same program in a dedicated Web Worker. The page only applies the
DOM updates Go sends and forwards events, so it stays responsive while Go
works (ADR-034).

Measured with `test/e2e/busy.mjs`, where a click handler computes for two
seconds: on the main thread the page goes 1,983 ms without a frame, in worker
mode 16.8 ms.

## Turning it on

One option on the page, nothing in Go:

```html
<script src="wasm_exec.js"></script>
<script src="goowee.js"></script>
<script>goowee.boot({worker: true})</script>
```

- The worker is started from `goowee.js` itself, so it must be served from the
  page's own origin, like any worker script. With a Content Security Policy,
  allow it: `worker-src 'self'`.
- The worker loads `wasm_exec.js` too: by default the one the page loads (its
  `<script src>`), else `wasm_exec.js` next to the page. Pass
  `{wasmExec: "/assets/wasm_exec.js"}` to name it. The page itself doesn't
  need the `<script>` for it in worker mode.
- `localStorage.setItem("goowee-worker", "1")` turns worker mode on without
  changing the page (when `opts.worker` isn't given). The E2E suite uses it.
- `bridge.InWorker()` tells Go code which mode it runs in.

Everything else works as on the main thread: rendering and hydration,
events, the router (history, scroll restoration, fragments, focus), forms and
bindings, `ref.Get`/`ref.Call`, files from inputs, paste and drop, `goowee/net`,
storage, islands, `goowee/canvas`, transitions, and the dev tools.

## Cancelling events: `PreventDefaultWhen`

An event is dispatched on the page, and by the time the worker runs its Go
handler the browser has already acted on it. So a handler's own
`e.PreventDefault()` can't take effect in worker mode; it logs a warning
once per event type. Say when to cancel declaratively instead, which works in
both modes and is decided by the page before Go runs:

```go
// Before: decided inside the handler.
Textarea(OnKeyDownE(func(e core.EventData) {
    if e.Key() == "Enter" && !e.ShiftKey() {
        e.PreventDefault()
        send()
    }
}))

// After: the condition is part of the handler's options.
enter := KeyIs("Enter").Without(core.ModShift)
Textarea(OnKeyDownE(func(e core.EventData) {
    if enter.Match(e) {
        send()
    }
}, PreventDefaultWhen(enter)))
```

| Cancel when | Option |
|---|---|
| always (a form submit, a drop) | `PreventDefault()` |
| a plain left click, not Ctrl/Cmd-click (an in-app link) | `PreventDefaultWhen(PlainClick())` |
| a key, with or without modifiers (never while an IME composes) | `PreventDefaultWhen(KeyIs("s").With(core.ModCtrl))` |
| a drag, drop or paste that carries files | `PreventDefaultWhen(CarriesFiles())` |

`StopPropagation()` as an option also applies on the page. A handler's
`e.StopPropagation()` still keeps the event from the remaining Go handlers.

## Limits

| Not available in worker mode | Why, and what to do |
|---|---|
| `bridge.Element`, and DOM calls through `syscall/js` | A worker has no DOM objects. `bridge.Element` returns `undefined` and warns once. Use refs (`ref.Call`, `ref.Get`), or keep such code to main-thread mode (`bridge.InWorker()`). |
| A dynamic `e.PreventDefault()` | See above: use `PreventDefault()` or `PreventDefaultWhen(...)`. |
| `BeforeLeave` on a page unload | `beforeunload` must be answered synchronously. In-app navigation is still guarded. |
| `ScrollOptions.Container` scroll positions | Only the window's position is tracked for restoration. |
| A fragment link whose target is missing | It doesn't scroll to the top: the worker can't ask the page whether the target exists. |
| A quota error from `storage.Store.Set` | Writes are applied by the page after `Set` has returned; the page logs a failure. |
| A synchronous `goowee.inspect()` / `goowee.changes()` | They return promises in worker mode. |

Handlers still run one at a time. A long handler no longer stops the page from
painting, but the next click's handler waits for it. Move long work to a
goroutine and apply the result with `core.Schedule`, as on the main thread.

## How it works

The page's `goowee.js` starts the worker with the same file. The worker
imports `wasm_exec.js`, runs the module, and installs its own `goowee.host`:
everything Go asks of the browser goes through that object (Go never reads
`document` or `location` itself; a test enforces it). Batches go to the page
as transferred buffers. Events come back with the ids of the nodes whose
handlers should run. Every batch says which event the worker had handled,
so the page can hold a write to a field the user has typed in since. Without
that, a late frame would put old text back. The message protocol is
documented in #158.
