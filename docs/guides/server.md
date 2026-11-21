# Talking to your server

`goowee/rpc` lets the client call your Go server with typed functions. One
definition, in a package both sides import, names a function and its request
and response types. The server registers a handler for it, the client calls
it, and if the two disagree, the code doesn't compile.

The complete example is [`examples/notes`](../../examples/notes): a list, a
detail page and a form, server-rendered.

## 1. Define the functions

```go
package api // imported by the server and by the WASM client

type Note struct {
    ID    int    `json:"id"`
    Title string `json:"title"`
}
type AddReq struct {
    Title string `json:"title"`
}

var (
    ListNotes = rpc.Define[ListReq, []Note]("notes.list")
    AddNote   = rpc.Define[AddReq, Note]("notes.add")
)
```

Requests and responses travel as JSON, so use types `encoding/json` handles.

## 2. Handle them on the server

```go
mux := http.NewServeMux()
rpc.Handle(mux, api.AddNote, func(ctx context.Context, req api.AddReq) (api.Note, error) {
    if req.Title == "" {
        return api.Note{}, rpc.Invalid("the note needs a title",
            map[string]string{"title": "Give the note a title."})
    }
    return store.Add(req), nil
})
```

`Handle` serves each function at `rpc.BasePath + "/" + name` (`/_rpc/notes.add`),
POST only. By default it refuses cross-site calls: a call's `Origin`, when
sent, must be the server's own, and its `Content-Type` must be
`application/json`, which a cross-site HTML form can't send. For
cookie-authenticated apps that blocks cross-site request forgery;
`rpc.AllowCrossSite()` turns it off.

`rpc.Before(check)` runs before the handler, for authentication: return an
error to refuse the call (`rpc.Errorf(rpc.CodeUnauthorized, …)` answers 401),
or a context carrying the user for the handler to read.

The server package imports `net/http`; the shared `api` package and the
client don't, so your WASM binary stays small.

## 3. Call them from the client

`Call` blocks, so call it from a goroutine and apply the result with
`core.Schedule`:

```go
OnSubmit(func(v map[string]string) {
    go func() {
        note, err := api.AddNote.Call(ctx, api.AddReq{Title: v["title"]})
        core.Schedule(func() {
            var e *rpc.Error
            if errors.As(err, &e) {
                titleErr.Set(e.Fields["title"]) // shown next to the field
                return
            }
            r.Navigate(fmt.Sprintf("/notes/%d", note.ID))
        })
    }()
})
```

For data a component shows, `rpc.UseQuery` is a `hooks.Resource` that calls
on mount and again whenever its request signal changes, cancelling a call it
supersedes:

```go
query := core.NewSignal(api.ListReq{})
notes := rpc.UseQuery(api.ListNotes, query)
h.ShowResource(notes, loading, failed, func(list *core.Signal[[]api.Note]) core.Node { … })
```

## Errors

Every failure is an `*rpc.Error` with a `Code`, a `Message` and, for form
validation, `Fields`. The codes map to HTTP statuses: `CodeInvalid` 400,
`CodeUnauthorized` 401, `CodeForbidden` 403, `CodeNotFound` 404,
`CodeConflict` 409, `CodeInternal` 500. A handler error that isn't an
`*rpc.Error` is logged on the server and reaches the client as `CodeInternal`
with the message "internal error", so internals don't leak. `CodeNetwork`
means no response arrived.

## Server rendering and tests

In the server's own process, `Call` runs the registered handler directly,
without HTTP, so server-side code can use the same functions.

In component tests, `rpc.Fake(t, api.AddNote, fn)` stubs one function for the
rest of the test. `gooweetest.NewFakeNet` also works, since calls go through
`goowee/net`.
