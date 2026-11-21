# Adding goowee to an existing Go site

You don't have to hand a whole site to goowee. A page your Go server already
renders, with `html/template`, templ or plain strings, can carry one or more
goowee components, called **islands**. The server renders each island's HTML
into the page; in the browser, goowee hydrates each one, and the rest of the
page stays yours.

## 1. A component that renders from its props

An island is a component built from props alone, so the server and the
browser build the same thing:

```go
package widgets

type CartProps struct {
    Items int `json:"items"`
}

func Cart(p CartProps) core.Node {
    return core.Component("Cart", func() core.Node {
        n, setN := hooks.UseState(p.Items)
        return Div(Class("cart"),
            Textf("%d items", n),
            Button(OnClick(func() { setN(n.Get() + 1) }), Text("Add")),
        )
    })
}
```

## 2. Render it on the server

`ssr.Island(name, node, props)` returns the island's HTML: a
`<div data-goowee-island="name">` with the component's markup and its props
as JSON. Put it in your template like any other `template.HTML`:

```go
cart, err := ssr.Island("cart", widgets.Cart(props), props)
if err != nil { /* props didn't marshal */ }
tmpl.Execute(w, map[string]any{"Cart": cart})
```

```html
<header>…your server markup…</header>
{{ .Cart }}
<script src="/wasm_exec.js"></script>
<script src="/goowee.js"></script>
<script>goowee.boot({wasm: "/islands.wasm"});</script>
```

Load `goowee.js` in `<head>` (a plain `<script>`, not `defer`) if you want
clicks and typing made before the WASM arrives to be kept (see
[concepts](concepts.md#before-the-app-is-interactive)).

## 3. Hydrate it in the browser

A small `main` package, built with `GOOS=js GOARCH=wasm`, maps each island
name to a factory that rebuilds the component from its props:

```go
//go:build js && wasm

package main

func main() {
    bridge.Islands(map[string]func(json.RawMessage) core.Node{
        "cart": func(raw json.RawMessage) core.Node {
            var p widgets.CartProps
            _ = json.Unmarshal(raw, &p)
            return widgets.Cart(p)
        },
    })
    bridge.Wait()
}
```

Each island hydrates on its own, keeping the server's DOM, and works on its
own. Islands share one scheduler, so they can share signals through a
package-level store, and `core.Schedule` works as in an app.

## Mounting without the server

`bridge.Mount(selector, node)` mounts a component into any element and
returns at once, so a page can hold several goowee roots: an app in `#root`
and a widget elsewhere, say. It hydrates when the element holds server
markup for that node. `bridge.Run(app)` is `Mount` into `#root` plus
`bridge.Wait()`.

`Mount` and `Islands` return `*bridge.Root` values; `root.Unmount()` removes
one root's content and runs its cleanups, leaving the others running.

## Limits

- One WASM binary per page. It can mount any number of roots, but two goowee
  binaries in one page would share the runtime's globals.
- Island props travel as JSON: use fields `encoding/json` can marshal.
- An island renders its own markup only. Page metadata (`h.Metadata`) inside
  an island isn't written to the page's `<head>`.
