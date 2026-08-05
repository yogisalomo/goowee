# Building apps with goowee — a guide for coding agents

This file tells a coding agent how to build a UI with **goowee** (a Go→WebAssembly
reactive framework) correctly on the first try. If you are working in a project
that depends on `github.com/yogisalomo/goowee`, read this before writing UI code.
See [`docs/canonical/adr.md`](docs/canonical/adr.md) for the *why* behind these rules.

> Copy this file into your project (as `AGENTS.md` or `CLAUDE.md`), or add a line
> to your existing agent instructions: *"This project uses goowee — follow its
> AGENTS.md."* The **Golden rules** below are the ones that prevent wasted iterations.

---

## Mental model (internalize this first)

goowee is **not React**. The single most important fact:

> **A component's function body runs ONCE, when it mounts.** It is setup, not render.
> It does not re-run when state changes.

State lives in **signals**. When a signal changes, goowee updates *only* the exact
DOM text/attribute/list bound to it — there is no re-render pass and no virtual DOM.
So you never "re-render a component"; you bind parts of the DOM to signals and let
those parts update.

```go
func Counter() core.Node {
    return core.Component("Counter", func() core.Node {
        count, setCount := hooks.UseState(0) // signal + setter

        // This runs ONCE. `Textf` binds the signal, so only the text updates.
        return Div(
            P(Textf("Count: %d", count)),
            Button(OnClick(func() { setCount(count.Get() + 1) }), Text("increment")),
        )
    })
}
```

If you find yourself expecting the function body to run again on a click — stop.
Bind a signal instead.

---

## Golden rules (each prevents a common mistake)

1. **Static vs reactive display.** `Text("x")`, `Class("x")`, `Value("x")` are
   **static** — they never update. To make display track a signal, use the
   reactive forms: `TextS(sig)`, `Textf("...", sig)` (any signal arg is reactive),
   and the `-S`-suffixed attribute/prop helpers (`ClassS`, `ValueS`, `DisabledS`, …).
2. **Read/write signals** with `sig.Get()` / `setter(v)` (from `UseState`) or
   `sig.Set(v)`. In markup, pass the **signal itself** to reactive helpers
   (`Textf("%d", count)`), not `count.Get()` (which captures a one-time value).
3. **Effects declare their deps explicitly** — there is no auto-tracking:
   - `hooks.UseEffect([]core.SignalAccessor{a, b}, func() func() { ...; return cleanup })`
   - `hooks.Watch([]core.SignalAccessor{a}, func() { ... })` (no cleanup)
   - `hooks.OnMount(func() func() { ...; return cleanup })` (runs once on mount,
     **client only** — never during SSR; use it for timers, subscriptions, fetches).
4. **Off the render loop → `core.Schedule`.** Any code running outside the render
   loop (a `time.Ticker`, `go func`, a fetch/network callback, a channel receive)
   **must not call a setter or `sig.Set` directly** — that races the renderer.
   Wrap the update: `core.Schedule(func() { setCount(n) })`. Inside `Schedule` you
   read/write signals normally.
5. **Lists → `For` with a stable key.** `For(sig, func(x T) K { return x.ID }, func(x T) core.Node { ... })`.
   The key must be unique and stable per item so reordering preserves identity.
6. **Conditionals → `Show` / `ShowElse` / `Switch`** (not an `if` in the body,
   which only runs once): `ShowElse(flag, func() core.Node {...}, func() core.Node {...})`.
7. **Derived values → `core.Computed`**, not recomputation in the body:
   `total := core.Computed([]core.SignalAccessor{items}, func() int { ... })`.
8. **SSR must be deterministic.** If you server-render, the client must produce the
   same markup. For content that legitimately differs per request (dates, locale,
   auth), either wrap it in `h.Dynamic()` (client value wins on hydration) or render
   a placeholder and fill it in from `OnMount` (client-only).
9. **Imperative DOM → refs; overlays → portals.** `ref := h.Ref()`, attach with
   `h.RefTo(ref)`, then `ref.Focus()`/`Blur()`/`Click()`/`ScrollIntoView()` from a
   handler/effect. `h.Portal("#modal-root", …)` renders outside the current subtree.
10. **Prefer the typed helpers.** Use `h`'s element/attr/event constructors rather
    than building `core.ElementNode` literals by hand.
11. **Head metadata → `Metadata` + `Page`.** Wrap `<title>`, `<meta>`, `<link>`,
    `<script>` etc. in `Metadata(...)` anywhere in the tree; they're collected and
    injected into `<head>` by both SSR and DOM renderers. For the full set of
    SEO/social/structured-data tags, use `Page(PageMeta{...})` which expands a
    single config into title, OG, Twitter Cards, JSON-LD, and LLM elements.
    A single `Page(PageMeta{Title: "…"})` is enough for most pages.

---

## API cheat sheet

Import the DSL with a dot import; keep `core`, `hooks`, `router` qualified:

```go
import (
    "github.com/yogisalomo/goowee/core"
    . "github.com/yogisalomo/goowee/h"
    "github.com/yogisalomo/goowee/hooks"
    "github.com/yogisalomo/goowee/router"
)
```

**State & reactivity** (`core`, `hooks`)
- `count, setCount := hooks.UseState(0)` → `*core.Signal[int]`, `func(int)`
- `sig.Get()`, `sig.Set(v)`, `core.NewSignal(v)`, `sig.WithEquals(eq)`
- `core.Computed(deps, compute) *Signal[T]`
- `hooks.UseEffect(deps, func() func())`, `hooks.Watch(deps, func())`, `hooks.OnMount(func() func())`
- `hooks.UseResource(deps, fetch) *Resource[T]` — async load (`Data`/`Loading`/`Err` + `Refetch`)
- `hooks.UseComputed(deps, compute) *Signal[T]` — re-export of `core.Computed`
- `core.Schedule(func())` — run an update on the render loop from off-loop code

**Elements & content** (`h`, dot-imported)
- Elements: `Div`, `Span`, `P`, `H1`–`H6`, `Button`, `Input`, `Form`, `Label`,
  `Ul`/`Li`, `Nav`, `Main`, `Footer`, `A`, `Img`, … and `El("tag", …)` for anything else.
- Raw HTML: `Raw(html)` — inserts pre-rendered HTML verbatim (sanitize untrusted input!).
- Text: `Text("static")`, `TextS(sig)`, `Textf("Count: %d", count)` (reactive args).
- Attrs: `Class`, `ID`, `Href`, `Type`, `Name`, `Placeholder`, `Style`, `Attr(name, val)`,
  ARIA (`AriaLabel`, `AriaHidden(true)`, `AriaCurrent("page")`, …). Reactive: add `S`
  (`ClassS(sig)`, `StyleS(sig)`).
- Props: `Value`, `Checked`, `Disabled`, `Required`, … and reactive `ValueS`, `DisabledS`, …
- Control flow: `Show(cond, then)`, `ShowElse(cond, then, else)`,
  `Switch(sig, map[T]func()core.Node, default)`, `For(sig, keyFn, render)`.
- Async rendering: `ShowResource(res, loadingFn, errFn, dataFn)` — handles loading/error/data states.
- SVG: `Svg(...)` roots a namespaced subtree; shapes `Path`, `Circle`, `Rect`, `G`,
  `Line`, `Polyline`, `Polygon`, `Ellipse`; arbitrary attrs via `Attr("viewBox", …)`.
- Refs/portals: `Ref()`, `RefTo(ref)`, `Portal(target, …)`.
- Error boundary: `ErrorBoundary(func(err any) core.Node { … }, child)` — renders
  the fallback if rendering `child` panics (a mount-time failure), so it doesn't
  blank the page.
- Head metadata: `Metadata(Title("…"), Meta(…), Link(…), JSONLD(…), …)` —
  wraps head elements; collected and injected into `<head>` by both renderers.
  For a single-config expansion: `Page(PageMeta{Title: "…", Description: "…"})`
  generates `<title>`, OG/Twitter tags, JSON-LD, and LLM meta automatically.

**Events** (`h`)
- Simple: `OnClick(func())`, `OnInput(func(string))`, `OnChange(func(string))`,
  `OnSubmit(func(map[string]string))`, `OnFocus`, `OnBlur`, `OnKeyDown`, …
- Full event data / options: `OnClickE(func(core.EventData), PreventDefault(), StopPropagation())`.
- Two-way binding: `BindValue(strSig)`, `BindChecked(boolSig)`, `BindSelect(strSig)`,
  `BindValueLazy(strSig)` (commits on change).

**Router** (`router`)
- `r := router.New(router.CurrentPath())`; `r.BindHistory()` (browser back/forward).
- `r.Route(map[string]func() core.Node{ "/": home, "/todos/:id": todo, "/404": notFound })`
  — exact wins; `:param` and `/*` supported; most-specific match is deterministic.
- Params: `r.Param("id")` (snapshot), `r.ParamSignal("id")` (reactive — bind this),
  `r.Params()`.
- Query params: `r.QueryParam("tag")` (snapshot), `r.QueryParamSignal("tag")` (reactive),
  `r.SetQueryParam("tag", "value")` (replaceState), `r.SetQueryParamPush("tag", "value")` (pushState).
- Navigation: `r.Navigate(path)`, `r.NavigateReplace(path)`, `r.Back()`, `r.Forward()`,
  `r.Link(to, text)`.
- Nesting/util: `r.SubRoute(prefix, routes)`, `router.Guard(check, fallback, route)`,
  `router.Lazy(load)`.

---

## Project setup (WASM entry + build)

A goowee app compiles to WebAssembly. Minimal `main.go`:

```go
//go:build js && wasm

package main

import (
    "github.com/yogisalomo/goowee/bridge"
    "github.com/yogisalomo/goowee/router"
    "yourmodule/app" // your App(r) component
)

func main() {
    r := router.New(router.CurrentPath())
    r.BindHistory()
    // bridge.Run mounts the app, hydrates the server-rendered DOM when present
    // (claiming it instead of rebuilding), and drives the frame loop. It never
    // returns, keeping the WASM module alive to service events.
    bridge.Run(app.App(r))
}
```

Build and serve:

```sh
GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app     # your wasm entry
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/            # Go's JS loader
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js web/  # the bridge
# Serve web/ with an index.html that loads: wasm_exec.js, goowee.js, main.wasm,
# and has a <div id="root"></div>. (See goowee's examples/counter for a template.)
```

For **SSR + hydration**: on the server, `body := ssr.New().Render(App(r))`, wrap it in
`<div id="root">{body}</div>`, and serve the same wasm loader. The client detects the
server DOM (via `data-node-id`) and hydrates it. Keep server and client markup identical
(rule 8).

**Deployment:** goowee apps are SPAs — the server must serve `index.html` for
all paths that don't match a static file, or deep links return 404 before the
WASM loads. See [`docs/deployment.md`](docs/deployment.md) for host-specific
configs (Cloudflare Pages, Netlify, Vercel, GitHub Pages, nginx) and
[`docs/docker/`](docs/docker/) for a reference Docker image.

---

## Copy-paste patterns

**Async data — prefer `hooks.UseResource`** (it runs the fetch in a goroutine
and applies the result safely; don't hand-roll goroutine+Schedule for loads):
```go
func Greeting() core.Node {
    return core.Component("Greeting", func() core.Node {
        msg := hooks.UseResource(nil, func() (string, error) { return api.Greeting() })
        return ShowResource(msg,
            func() core.Node { return P(Text("Loading…")) },
            func(err error) core.Node { return P(Textf("Error: %s", err)) },
            func(data *core.Signal[string]) core.Node { return P(TextS(data)) },
        )
    })
}
```
`Resource` exposes `Data`/`Loading`/`Err` signals + `Refetch()`; pass deps
(`UseResource([]core.SignalAccessor{id}, …)`) to refetch when they change.
Fetching is client-side, so SSR renders the loading state. (For a one-off
side-effect that isn't a data load, use `OnMount` + a goroutine + `core.Schedule`
directly.)

**Controlled form:**
```go
email, setEmail := hooks.UseState("")
return Form(
    OnSubmit(func(vals map[string]string) { submit(vals["email"]) }),
    Input(Type("email"), Name("email"), BindValue(email)),
    Button(Text("Save")),
)
```

**Keyed list:**
```go
For(todos, func(t Todo) int { return t.ID }, func(t Todo) core.Node {
    return Li(Text(t.Title))
})
```

**Reactive param route:** `P(Textf("Todo %s", r.ParamSignal("id")))` — updates in place
as `/todos/1` → `/todos/2` (bind `ParamSignal`, don't read `Param` once in setup).

---

## Anti-patterns (do NOT do these)

- ❌ Expecting the component body to re-run on state change. It runs once.
- ❌ `P(Text(count.Get()))` for changing values → renders once, never updates.
  Use `Textf("%d", count)` / `TextS`.
- ❌ Calling a setter from `time.AfterFunc`, a goroutine, or a JS callback directly.
  Wrap in `core.Schedule`.
- ❌ `if cond { return A } else { return B }` in the body to switch UI reactively.
  Use `ShowElse`/`Switch`.
- ❌ A plain Go `for`/`append` to build a list from a signal. Use `For`.
- ❌ Non-deterministic SSR output (`time.Now()`, random) without `h.Dynamic()` or the
  placeholder-plus-`OnMount` pattern.
- ❌ Reaching into the DOM with `syscall/js` from app code. Use a `Ref` (`Focus`, etc.).

---

## Worktree pattern

When asked to draft work in an isolated worktree, create it under `.opencode/worktrees/`:

```sh
git worktree add -b <branch-name> .opencode/worktrees/<name> <base-branch>
```

This keeps worktrees inside the repo (and gitignored via `.opencode/`), avoiding
`/tmp` and keeping them discoverable. When done, `git worktree remove .opencode/worktrees/<name>` and delete the branch.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
