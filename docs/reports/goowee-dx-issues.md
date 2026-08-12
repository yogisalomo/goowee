# Goowee DX Issues & Suggestions

> Collected while building a real-world app (yogi.sh personal site) on top of goowee.
> Each issue includes the current behavior, why it matters, and a concrete suggestion.

---

## 1. `h.Raw()` — Missing Raw HTML Rendering

**Severity:** High
**Files:** `h/h.go`, `core/types.go`, `internal/dom/renderer.go`, `ssr/renderer.go`

### Current behavior

There is no way to render a pre-rendered HTML string into the DOM. Every piece of content must be constructed through the `h` DSL (`Div(...)`, `P(...)`, etc.). If you have HTML from an external source (markdown renderer, CMS, third-party widget), you must either:

1. Parse the HTML yourself and map every element to `h.El("tag", ...)` calls, or
2. Escape it and display it as plain text.

Neither is practical for real-world content like blog posts, which contain deeply nested HTML with code blocks, tables, blockquotes, and arbitrary elements.

### Why it matters

Any app that renders user-generated or pre-rendered HTML needs this. Specifically:

- **Markdown blogs:** A markdown renderer (goldmark, blackfriday) produces an HTML string. Without `h.Raw()`, there's no way to inject it into the goowee node tree.
- **CMS integrations:** Headless CMS systems return rich text as HTML.
- **Third-party embeds:** Widgets, iframes, or script-injected content.
- **Documentation sites:** Content from MDX or similar pipelines.

### Suggested API

```go
// h package

// Raw renders a pre-rendered HTML string into the DOM without parsing it
// into individual element nodes. Use for markdown output, CMS content,
// or any HTML that doesn't need reactive updates.
//
// WARNING: The HTML is inserted as-is. Do not use with untrusted input
// without sanitization (e.g. bluemonday). The node does not participate
// in reconciliation — the entire content is replaced on re-render.
func Raw(html string) *core.RawNode
```

### Implementation sketch

**`core/types.go`** — Add a new node type:

```go
// RawNode holds a pre-rendered HTML string that is inserted verbatim
// into the DOM. It does not participate in child reconciliation.
type RawNode struct {
    HTML string
    ID   int // assigned by walker
}
```

**`h/h.go`** — Add the constructor:

```go
func Raw(html string) *core.RawNode {
    return &core.RawNode{HTML: html}
}
```

**`internal/dom/renderer.go`** — Handle in the walker:

```go
func (r *DOMRenderer) VisitRaw(id int, rn *core.RawNode) {
    if rn == nil {
        return
    }
    if r.hydrating {
        // Claim the server-rendered node
        *r.muts = append(*r.muts, core.Mutation{
            Type: core.MutHydrate, NodeID: id, Key: "tag", Value: "raw",
        })
        return
    }
    // Create a container element and set its innerHTML
    *r.muts = append(*r.muts, core.Mutation{
        Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: "div",
    })
    *r.muts = append(*r.muts, core.Mutation{
        Type: core.MutSetProperty, NodeID: id, Key: "innerHTML", Value: rn.HTML,
    })
}
```

**`ssr/renderer.go`** — Emit HTML verbatim:

```go
func (v *ssrVisitor) VisitRaw(id int, rn *core.RawNode) {
    if rn == nil {
        return
    }
    v.r.Meta.NodeMap[id] = v.path
    if v.silent {
        return
    }
    fmt.Fprintf(v.buf, "<div data-node-id=\"%d\">%s</div>", id, rn.HTML)
}
```

**`internal/walker/walker.go`** — Add `VisitRaw` to the `Visitor` interface and wire it in `Walk`.

### Hydration notes

During hydration, the client claims the `<div data-node-id="N">` that already contains the server-rendered HTML. The `MutHydrate` mutation tells the JS runtime to find and claim that node. Since the HTML is static (no signals), no further mutations are needed.

If the `Raw` content changes at runtime (e.g. navigating to a different blog post), the diff algorithm sees a `RawNode` with different content and replaces the entire subtree — this is the correct behavior since raw HTML can't be incrementally patched.

---

## 2. Missing ARIA Attribute Helpers

**Severity:** Low
**File:** `h/attrs.go`

### Current behavior

The API reference documents `AriaChecked(bool)` and `AriaDisabled(bool)`, but the implementation in `attrs.go` does not include them. The actual helpers present are:

```
AriaLabel, AriaHidden, AriaExpanded, AriaCurrent, AriaDescribedBy,
AriaLabelledBy, AriaControls, AriaPressed, AriaSelected, AriaInvalid,
AriaRequired, AriaHaspopup, AriaModal, AriaLive, AriaAtomic, AriaBusy
```

Missing from the implementation:
- `AriaChecked(bool)` — needed for custom checkbox/radio components
- `AriaDisabled(bool)` — needed for custom interactive elements

### Suggested fix

Add to `h/attrs.go`:

```go
func AriaChecked(v bool) core.Item   { return attrItem{"aria-checked", fmtBool(v)} }
func AriaDisabled(v bool) core.Item  { return attrItem{"aria-disabled", fmtBool(v)} }
```

These follow the exact same pattern as the existing `AriaPressed`, `AriaSelected`, etc.

### Why it matters

While `Aria(name, value)` works as a fallback, typed helpers provide:
- **Discoverability:** Developers find them via autocomplete instead of reading docs.
- **Consistency:** Matches the pattern of all other ARIA helpers.
- **Correctness:** Prevents typos in attribute names (e.g. `aria-ckecked`).

---

## 3. `hooks.UseComputed` — Documentation/Implementation Drift

**Severity:** Low
**Files:** `docs/api/reference.md`, `AGENTS.md`, `hooks/`

### Current behavior

The API reference and AGENTS.md both document:

```go
total := hooks.UseComputed([]core.SignalAccessor{deps}, func() int {
    return sum(items.Get())
})
```

This does not exist in the `hooks` package. The actual function is `core.Computed`:

```go
total := core.Computed([]core.SignalAccessor{deps}, func() int {
    return sum(items.Get())
})
```

### Suggested fix (two options)

**Option A: Add the re-export to hooks** (matches docs, better DX):

```go
// hooks/use_computed.go
package hooks

func UseComputed[T any](deps []core.SignalAccessor, compute func() T) *core.Signal[T] {
    return core.Computed(deps, compute)
}
```

This is a one-liner re-export that makes the hooks package a single import for all hook-like functionality. Developers shouldn't need to know whether a function lives in `core` or `hooks`.

**Option B: Update the docs** (minimal change):

Remove `hooks.UseComputed` from `docs/api/reference.md` and `AGENTS.md`, replacing it with `core.Computed`. This is simpler but means developers need to import both `hooks` and `core` for common patterns.

**Recommendation:** Option A. The re-export costs nothing and keeps the mental model clean: "hooks for state/effects/computed, h for elements, router for navigation."

---

## 4. No Query Parameter Support in Router

**Severity:** Medium
**File:** `router/router.go`

### Current behavior

The router only matches on path segments. There is no API to read or react to URL query parameters (`?key=value`). To read `?tag=golang` from `/blog?tag=golang`, you would need to drop down to JS interop:

```go
query := js.Global().Get("location").Get("search").String()
// manually parse query string...
```

This is not reactive — the signal system can't track changes to query parameters.

### Why it matters

Query parameters are a common pattern for:

- **Filtering:** `/blog?tag=golang` (filter posts by tag)
- **Pagination:** `/blog?page=2`
- **Search:** `/blog?q=supabase`
- **State preservation:** `/games/about-me?code=ABCD`

Currently, yogi.sh uses path-based routing for tags (`/tags/:tag`) as a workaround, but query parameters are more idiomatic for optional filtering.

### Suggested API

```go
// Router additions

// QueryParam returns the value of a URL query parameter ("" if absent).
// This is a snapshot — good for event handlers.
func (r *Router) QueryParam(name string) string

// QueryParamSignal returns a derived signal of one query parameter's
// value that updates when the URL changes. Bind it reactively.
func (r *Router) QueryParamSignal(name string) *core.Signal[string]

// SetQueryParam updates a query parameter without navigating.
// Uses replaceState by default (no history entry).
func (r *Router) SetQueryParam(name, value string)

// SetQueryParamPush is like SetQueryParam but pushes a history entry.
func (r *Router) SetQueryParamPush(name, value string)
```

### Implementation sketch

Store query params as a signal alongside the path signal:

```go
type Router struct {
    Path      *core.Signal[string]
    Query     *core.Signal[map[string]string] // NEW
    // ...
}
```

In `BindHistory`, parse `window.location.search` on popstate:

```go
func (r *Router) BindHistory() {
    // ... existing popstate listener ...
    // On path change, also parse query:
    search := js.Global().Get("location").Get("search").String()
    r.Query.Set(parseQuery(search))
}
```

For `SetQueryParam`:

```go
func (r *Router) SetQueryParam(name, value string) {
    q := r.Query.Get()
    q[name] = value
    r.Query.Set(q)
    // Update URL with replaceState
    js.Global().Get("history").Call("replaceState", nil, "", r.base+r.Path.Get()+"?"+encodeQuery(q))
}
```

`QueryParamSignal` would be a `core.Computed` on `r.Query`:

```go
func (r *Router) ParamSignal(name string) *core.Signal[string] {
    return core.Computed([]core.SignalAccessor{r.Query}, func() string {
        return r.Query.Get()[name]
    })
}
```

---

## 5. No `Suspense`-Like Pattern for Async Content

**Severity:** Medium
**File:** `hooks/use_resource.go`

### Current behavior

`UseResource` returns a `Resource[T]` with `.Data`, `.Loading`, and `.Err` signals. The developer must manually compose the loading/error/data states:

```go
res := hooks.UseResource(nil, fetchUser)
return Div(
    Show(res.Loading, func() core.Node {
        return P(Text("Loading..."))
    }),
    ShowElse(
        func() bool { return res.Err.Get() != nil },
        func() core.Node { return P(Text("Error: " + res.Err.Get().Error())) },
        func() core.Node { return renderUser(res.Data) },
    ),
)
```

This works, but every use site duplicates the same Show/ShowElse/Loading/Error boilerplate.

### Why it matters

For apps with multiple async data sources (blog post content, user profiles, API data), the repetition adds up. A `Suspense`-like boundary would let the developer declare the fallback once and let children handle their own data:

```go
// Hypothetical API
return Suspense(
    func() core.Node { return P(Text("Loading...")) },  // fallback
    func() core.Node {
        user := hooks.UseResource(nil, fetchUser)
        posts := hooks.UseResource(nil, fetchPosts)
        return Div(
            renderUser(user.Data),
            renderPosts(posts.Data),
        )
    },
)
```

### Suggested approach

This is a larger feature. A minimal version could be a helper function (not a new node type) that wraps the common pattern:

```go
// hooks/suspense.go

// ShowResource renders data when loaded, loading while fetching, and error on failure.
func ShowResource[T any](
    res *Resource[T],
    loading func() core.Node,
    err func(error) core.Node,
    data func(*core.Signal[T]) core.Node,
) core.Node {
    return h.Group(
        h.Show(res.Loading, loading),
        h.ShowElse(
            func() bool { return res.Err.Get() != nil },
            func() core.Node { return err(res.Err.Get()) },
            func() core.Node { return data(res.Data) },
        ),
    )
}
```

This doesn't solve the "multiple resources share one fallback" case, but it eliminates the most common boilerplate.

---

## 6. `For` Loses Component State Across Re-renders

**Severity:** Medium (documented, but worth reiterating)
**File:** `h/flow.go`

> **RESOLVED — the report was accurate about the docs, not the code.** This
> issue was filed from `For`'s doc comment, which was stale: it described
> pre-#6 behavior. The behavior it describes has not been true since component
> reconciliation (#6) and keyed component rows (#7) landed. Verified by
> `TestForRowComponentStateSurvivesListChanges`,
> `TestForRowInnerScopeStateSurvivesListChanges`, and
> `TestNestedComponentInKeyedRowPreserved` (`internal/dom/dom_test.go`): row
> state, inner scopes, and effects all survive appends, removals, and reorders
> as long as the key survives. The misleading doc comment has been rewritten.
> **Downstreams that hoisted row state out of `For` to work around this can
> move it back into the row component.**

### Current behavior

The `For` helper uses `ScopeNode` internally. When the scope re-renders (e.g. an item is added to the list), all old component frames inside the scope are torn down and new ones are created. This means any component inside a `For` row loses its state (signals, effects) on every re-render.

```go
// This component's internal state resets every time the list changes
h.For(items, keyFn, func(item Item) core.Node {
    return core.Component("Row", func() core.Node {
        count, setCount := hooks.UseState(0) // resets on list change
        return Div(
            Textf("Count: %d", count),
            Button(h.OnClick(func() { setCount(count.Get() + 1) }), Text("+")),
        )
    })
})
```

### Why it matters

For list UIs where individual rows have interactive state (expanded/collapsed, selected, form inputs), the state loss is disruptive. Developers must hoist row state into signals outside the `For` scope, which adds complexity.

### Suggested approach (future)

This is noted in the codebase as pending an "ownership redesign." The eventual solution would be to give each keyed row its own component identity, so state persists as long as the key persists. This is similar to how React's `key` prop preserves component identity.

In the meantime, the workaround is documented but could benefit from an example in the docs showing the "state outside For" pattern.

---

## 7. No Global State / Context Mechanism

**Severity:** Low-Medium
**Files:** `hooks/`, `core/`

### Current behavior

There is no built-in mechanism for sharing state across distant components without prop-threading. Signals can be closed over (captured in the component closure), but there's no formal "context" or "store" pattern.

For small apps this is fine. For larger apps, you end up with either:
- Signals defined at the top level and closed over by many components (works but implicit)
- Manual prop-threading through intermediate components (verbose)

### Suggested approach

A lightweight `UseContext` / `ProvideContext` pattern would help:

```go
// Hypothetical
var UserCtx = core.CreateContext[User]() // returns a ContextProvider + hook

func App() core.Node {
    return UserCtx.Provider(currentUser, func() core.Node {
        return Div(
            Header(),  // can read UserCtx via UseContext
            Content(), // can also read UserCtx
        )
    })
}

func Header() core.Node {
    return core.Component("Header", func() core.Node {
        user := UserCtx.UseContext() // reads from nearest Provider
        return Nav(Textf("Hello, %s", user.Name))
    })
}
```

This is not blocking for yogi.sh (the site is small enough to close over signals), but it would help the framework scale to larger applications.

---

## 8. WASM Binary Size — No Tree-Shaking or Splitting

**Severity:** Info (not blocking, but worth noting)

### Current behavior

Everything compiles into a single WASM binary. The entire framework, all hooks, all page components, and all blog content are in one file. For yogi.sh, this includes goldmark (markdown parser), yaml.v3 (front matter parser), and the full blog post content as embedded strings.

### Why it matters

Go WASM binaries are large because Go doesn't have tree-shaking. A minimal "hello world" is ~2MB. Adding goldmark and yaml.v3 likely pushes it to ~4-5MB uncompressed (~1.2MB gzipped).

The goowee docs recommend serving with gzip, which helps. But for sites with strict performance budgets, this could be a concern.

### Suggested approaches

1. **Document the expected binary size** for common app types (minimal, with blog, with routing).
2. **Consider a "lite" build** that strips devtools, logging, and error recovery for production. A build tag (`//go:build production`) could conditionally compile these out.
3. **Lazy route loading** already exists (`router.Lazy`) but only defers initialization, not code loading. A future improvement could use Go's plugin system or multiple WASM binaries for true code splitting (this is a large effort).

---

## 9. SSR `Render` Return Signature Mismatch

**Severity:** Low (docs only)
**Files:** `docs/api/reference.md`, `ssr/renderer.go`

### Current behavior

The API reference documents:

```go
html := renderer.Render(node core.Node) // returns a single string
```

The actual implementation returns two strings:

```go
body, head := renderer.Render(n core.Node) // returns (body, head)
```

### Suggested fix

Update the API reference to match the implementation:

```go
renderer := ssr.New()
body, head := renderer.Render(node) // body for <div id="root">, head for <head>
```

This is important because the `head` return value is needed for injecting metadata (`<title>`, `<meta>`, OG tags) into the HTML shell. Without it, SSR-rendered pages would have no `<title>` tag.

---

## 10. No `h.Raw` — Workaround Available but Fragile

**Severity:** Info (supplements Issue #1)

### Current workaround

Without `h.Raw()`, the only option is to use `El("div")` with manually parsed children. For a markdown blog, you'd need to write an HTML-to-goowee-node converter:

```go
func htmlToNodes(html string) []core.Node {
    // Parse HTML with html/tokenizer
    // For each element, create h.El(tag, attrs..., children...)
    // For each text node, create h.Text(content)
    // Return the tree
}
```

This is:
- **Tedious:** You need to handle every HTML element type, attribute, and nesting pattern.
- **Fragile:** Any HTML structure your parser doesn't handle gets silently dropped or broken.
- **Incomplete:** You'd need to handle `<pre><code class="language-rust">` blocks, `<table>` with `<thead>`/`<tbody>`, nested `<blockquote>`, etc.

The `h.Raw()` helper (Issue #1) eliminates this entirely. It's the highest-priority DX improvement.

---

## Summary

| # | Issue | Severity | Effort |
|---|-------|----------|--------|
| 1 | `h.Raw()` missing | High | Medium |
| 2 | Missing `AriaChecked`, `AriaDisabled` | Low | Trivial |
| 3 | `hooks.UseComputed` docs/source drift | Low | Trivial |
| 4 | No query parameter support in router | Medium | Medium |
| 5 | No `Suspense`-like pattern | Medium | Medium-Large |
| 6 | `For` loses component state | ~~Medium~~ **not a bug** | Resolved — stale doc comment, behavior already correct |
| 7 | No global state/context mechanism | Low-Medium | Medium |
| 8 | WASM binary size (no tree-shaking) | Info | Large (architectural) |
| 9 | SSR `Render` return signature mismatch | Low | Trivial (docs) |
| 10 | No `h.Raw` workaround is fragile | Info | — (see #1) |
