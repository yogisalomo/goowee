package hooks

import "github.com/yogisalomo/goowee/core"

// Resource is the state of an async load. Data holds the last successful value
// (the zero value until one arrives), Loading is true while a fetch is in
// flight, and Err holds the last error (nil on success). Bind these signals in
// your view — e.g. Show(res.Loading, …).
type Resource[T any] struct {
	Data    *core.Signal[T]
	Loading *core.Signal[bool]
	Err     *core.Signal[error]

	fetch func() (T, error)
	gen   int // guards against a stale fetch overwriting a newer one
}

// UseResource loads data asynchronously. fetch runs in a goroutine on mount,
// again whenever any dep changes, and on Refetch; its result is applied back on
// the render loop (via core.Schedule), so it never races the renderer. Fetching
// is client-side only: during SSR the resource stays in its loading state and
// the client loads after hydration.
//
//	user := hooks.UseResource(nil, func() (User, error) { return api.GetUser(id) })
//	return Div(
//	    Show(user.Loading, func() core.Node { return P(Text("Loading…")) }),
//	    ShowElse(hasErr(user), errView, func() core.Node { return userView(user.Data) }),
//	)
//
// Pass deps to refetch when they change (e.g. a route param signal):
//
//	page := hooks.UseResource([]core.SignalAccessor{id}, func() (Page, error) {
//	    return api.GetPage(id.Get())
//	})
func UseResource[T any](deps []core.SignalAccessor, fetch func() (T, error)) *Resource[T] {
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
		return nil
	})
	if len(deps) > 0 {
		Watch(deps, r.load)
	}
	return r
}

// Refetch re-runs the fetcher (e.g. from a "retry" button).
func (r *Resource[T]) Refetch() { r.load() }

// load runs on the render loop (mount, dep change, Refetch). It bumps the
// generation, flips Loading on, and fetches off-loop; the result is applied via
// core.Schedule and ignored if a newer load has started since.
func (r *Resource[T]) load() {
	r.gen++
	gen := r.gen
	r.Loading.Set(true)
	r.Err.Set(nil)
	go func() {
		data, err := r.fetch()
		core.Schedule(func() {
			if gen != r.gen {
				return // a newer load superseded this one
			}
			if err != nil {
				r.Err.Set(err)
			} else {
				r.Data.Set(data)
			}
			r.Loading.Set(false)
		})
	}()
}
