package hooks

import (
	"context"

	"github.com/yogisalomo/goowee/core"
)

// Resource is the state of an async load. Data holds the last successful value
// (the zero value until one arrives), Loading is true while a fetch is in
// flight, and Err holds the last error (nil on success). Bind these signals in
// your view — e.g. Show(res.Loading, …), or h.ShowResource.
type Resource[T any] struct {
	Data    *core.Signal[T]
	Loading *core.Signal[bool]
	Err     *core.Signal[error]

	fetch  func(ctx context.Context) (T, error)
	gen    int                // guards against a stale fetch overwriting a newer one
	cancel context.CancelFunc // cancels the in-flight fetch
}

// UseResource loads data asynchronously. fetch runs in a goroutine after the
// component mounts, again whenever any dep changes, and on Refetch; its result
// is applied back on the render loop (via core.Schedule), so it never races the
// renderer. Fetching is client-side only: during SSR the resource stays in its
// loading state and the client loads after hydration.
//
// fetch receives a context that is cancelled when its result is no longer
// wanted — a newer load started (dep change, Refetch) or the component
// unmounted — so pass it to your request and stop early:
//
//	user := hooks.UseResource(nil, func(ctx context.Context) (User, error) {
//	    return api.GetUser(ctx, id)
//	})
//
// Pass deps to refetch when they change (e.g. a route param signal):
//
//	page := hooks.UseResource([]core.SignalAccessor{id}, func(ctx context.Context) (Page, error) {
//	    return api.GetPage(ctx, id.Get())
//	})
//
// A superseded or cancelled fetch's result is ignored, whatever it returns.
//
// For HTTP in the browser, prefer the browser's fetch over net/http, which
// adds ~7 MB to the WASM binary — see docs/guides/serving.md for a
// context-aware helper.
func UseResource[T any](deps []core.SignalAccessor, fetch func(ctx context.Context) (T, error)) *Resource[T] {
	var zero T
	r := &Resource[T]{
		Data:    core.NewSignal(zero),
		Loading: core.NewSignal(true),
		Err:     core.NewSignal[error](nil),
		fetch:   fetch,
	}
	core.RegisterSignal(r.Data, "resource.data")
	core.RegisterSignal(r.Loading, "resource.loading")
	core.RegisterSignal(r.Err, "resource.err")
	OnMount(func() func() {
		r.load()
		return r.stop // unmount: cancel and ignore whatever is in flight
	})
	if len(deps) > 0 {
		Watch(deps, r.load)
	}
	return r
}

// Refetch re-runs the fetcher (e.g. from a "retry" button), cancelling a
// fetch still in flight.
func (r *Resource[T]) Refetch() { r.load() }

// load runs on the render loop (mount, dep change, Refetch). It cancels the
// previous fetch, bumps the generation, flips Loading on, and fetches
// off-loop; the result is applied via core.Schedule and ignored if a newer
// load has started (or the component unmounted) since.
func (r *Resource[T]) load() {
	r.stop()
	gen := r.gen
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.Loading.Set(true)
	r.Err.Set(nil)
	go func() {
		data, err := r.fetch(ctx)
		core.Schedule(func() {
			if gen != r.gen {
				return // superseded or unmounted
			}
			cancel()
			r.cancel = nil
			if err != nil {
				r.Err.Set(err)
			} else {
				r.Data.Set(data)
			}
			r.Loading.Set(false)
		})
	}()
}

// stop cancels the in-flight fetch (if any) and invalidates its result.
func (r *Resource[T]) stop() {
	r.gen++
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}
