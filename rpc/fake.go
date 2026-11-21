package rpc

import "context"

// TB is the part of testing.TB that Fake uses (so the package doesn't import
// testing into app builds).
type TB interface {
	Helper()
	Cleanup(func())
}

// Fake stubs f with fn for the rest of the test: every Call of f in this
// process runs fn instead of reaching a server, in component tests
// (gooweetest) and in the server's own tests. The stub is removed when the
// test ends.
//
//	rpc.Fake(t, api.AddTodo, func(ctx context.Context, req api.AddTodoReq) (api.Todo, error) {
//	    return api.Todo{ID: 1, Title: req.Title}, nil
//	})
func Fake[Req, Res any](t TB, f *Func[Req, Res], fn func(ctx context.Context, req Req) (Res, error)) {
	t.Helper()
	mu.Lock()
	prev, had := fakes[f.name]
	fakes[f.name] = func(ctx context.Context, req any) (any, error) { return fn(ctx, req.(Req)) }
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if had {
			fakes[f.name] = prev
		} else {
			delete(fakes, f.name)
		}
	})
}
