# Metadata & head management

goowee provides two ways to manage `<head>` content: **low-level helpers**
for individual elements and the **`Page()` constructor** that expands a single
config into the full set of social/SEO/structured-data tags.

---

## The Metadata container

Wrap head elements in `Metadata(...)`. Place it anywhere in the component tree;
the renderers collect its children and inject them into `<head>` — the
`Metadata` node itself is invisible in the body.

```go
func App(...) core.Node {
    return core.Component("App", func() core.Node {
        return Div(
            Metadata(
                Title("My Page"),
                Meta(Name("description"), Content("A description")),
                Link(Rel("canonical"), Href("https://example.com")),
            ),
            // ... rest of the tree
        )
    })
}
```

Multiple `Metadata` blocks accumulate — you can sprinkle them across layouts,
pages, and subcomponents.

---

## Low-level helpers

Each maps to a single HTML element:

| Helper       | Output                                        |
|--------------|-----------------------------------------------|
| `Title(s)`   | `<title>s</title>`                            |
| `Meta(...)`  | `<meta ...>`                                  |
| `Link(...)`  | `<link ...>`                                  |
| `Script(...)`| `<script ...>...</script>`                    |
| `StyleEl(...)`| `<style>...</style>`                        |
| `NoScript(...)`| `<noscript>...</noscript>`                 |
| `Base(...)`  | `<base ...>`                                  |

Attach attributes with `Name()`, `Content()`, `Rel()`, `Href()`, `Type()`,
`Attr("property", "og:title")`, etc.:

```go
Metadata(
    Meta(Name("robots"), Content("index, follow")),
    Meta(Attr("property", "og:image"), Content("https://example.com/hero.png")),
)
```

### JSONLD

Inject structured data (schema.org, etc.):

```go
Metadata(
    JSONLD(map[string]any{
        "@context":    "https://schema.org",
        "@type":       "WebPage",
        "name":        "My Page",
        "description": "A description",
    }),
)
```

Panics at mount if JSON marshalling fails — wrap in `ErrorBoundary` if the
data is dynamic.

### LLM

Machine-readable metadata for LLM consumption:

```go
Metadata(
    LLM("home page", "Welcome to the goowee framework", "go", "wasm", "ui"),
)
```

Emits `<meta name="llm" content="purpose=...;context=...;keywords=...">`.

---

## High-level constructor: `Page()`

`Page(PageMeta{...})` generates the full set of HTML/Social/JSON-LD/LLM
elements from a single config, propagating each field to every relevant format.

```go
Metadata(
    Page(PageMeta{
        Title:       "My Page",
        Description: "A description of my page",
        Canonical:   "https://example.com/my-page",
        Author:      "Jane Doe",
        Keywords:    []string{"go", "wasm", "ui"},
        Image:       "https://example.com/og.png",
        SiteName:    "Example",
    }),
)
```

### Field → output mapping

| Field        | Emitted elements                                             |
|-------------|--------------------------------------------------------------|
| `Title`     | `<title>`, `og:title`, `twitter:title`, JSON-LD `name`, LLM `purpose` |
| `Description`| `<meta name="description">`, `og:description`, `twitter:description`, JSON-LD `description`, LLM `context` |
| `Canonical` | `<link rel="canonical">`                                     |
| `Author`    | `<meta name="author">`                                       |
| `Keywords`  | `<meta name="keywords">`, LLM keywords                       |
| `Robots`    | `<meta name="robots">`                                       |
| `Locale`    | `og:locale` (default `en_US`)                                |
| `SiteName`  | `og:site_name`                                               |
| `Image`     | `og:image`, `twitter:image`, `twitter:card` (default `summary_large_image`) |
| `TwitterCard`| overrides the `twitter:card` value                          |
| `Charset`   | `<meta charset>` (default `utf-8`)                           |
| `Viewport`  | `<meta name="viewport">` (default `width=device-width, initial-scale=1`) |

Always emitted: charset, viewport, `og:locale`.

### Minimal example

```go
Metadata(Page(PageMeta{Title: "Minimal"}))
```

Generates charset, viewport, title, og:title, twitter:title, og:locale,
JSON-LD WebPage, and LLM meta.

### Mixing with low-level helpers

`Page()` returns a `Fragment`, so it composes naturally inside `Metadata`:

```go
Metadata(
    Page(PageMeta{Title: "Post", Description: "..."}),
    Link(Rel("stylesheet"), Href("/styles.css")),
    StyleEl(Text("body{background:#fff}")),
)
```

> **Head children must be static elements.** `Metadata` children are rendered to
> the head as plain HTML — `Title`, `Meta`, `Link`, `Script`, `StyleEl`, and
> `Fragment`/`Page` groupings of those. Reactive head content (`Show`, `Switch`,
> a component, or a signal-bound value) is **not** collected into the SSR head
> and is not supported; keep head metadata static and per-route.

---

## SSR head injection

The SSR renderer collects all `Metadata` children into a separate head buffer.
`Render()` returns `(body, head)`; the caller drops `head` into the document
`<head>` (see `cmd/ssr-server`). On the client, hydration treats the
server-rendered `<head>` as authoritative: the DOM renderer walks the `Metadata`
children to keep node-id parity with SSR but **does not re-emit** them, so the
head tags are not duplicated. A pure-client app (no SSR) creates and appends the
head tags on first render as usual.
