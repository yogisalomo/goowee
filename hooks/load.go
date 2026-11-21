package hooks

import (
	"context"
	"encoding/json"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/data"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// Load is UseResource whose data the server loads too (#152). key names the
// data in the page, so include what it depends on ("user:" + id). Under
// ssr.Handler or ssr.Export the server runs fetch before sending the page,
// renders the page with the data, and embeds it as JSON; in the browser the
// resource starts with that data, Loading false, and doesn't fetch it again.
// After that it behaves as UseResource: it refetches when a dep changes, and
// Refetch works.
//
//	hooks.Load("user:"+id, nil, func(ctx context.Context) (User, error) {
//	    return api.GetUser.Call(ctx, api.UserReq{ID: id})
//	}, func(res *hooks.Resource[User]) core.Node {
//	    return h.ShowResource(res, loading, failed, view)
//	})
//
// The data must be JSON-encodable. On the server, fetch runs once per
// request with the request's context and a deadline; if it fails or is late,
// the page renders the loading state and the client fetches as UseResource
// would. Resources inside data that only appears once another resource has
// loaded are fetched on the client.
//
// render builds the view from the resource. Load renders it in a component of
// its own, so the resource's state survives a parent re-render.
func Load[T any](key string, deps []core.SignalAccessor, fetch func(ctx context.Context) (T, error), render func(*Resource[T]) core.Node) core.Node {
	return data.Context.Use(func(s *core.Signal[*data.Store]) core.Node {
		store := s.Peek()
		return core.Component("Load", func() core.Node {
			if runtime.CurrentEnv() == runtime.EnvServer {
				if store.Server() {
					if raw, ok := store.Result(key); ok {
						var v T
						if json.Unmarshal(raw, &v) == nil {
							return render(useResource(deps, fetch, &v))
						}
					}
					store.Request(key, func(ctx context.Context) (any, error) { return fetch(ctx) })
				}
				return render(useResource(deps, fetch, nil))
			}
			if raw, ok := store.Take(key); ok {
				var v T
				if json.Unmarshal(raw, &v) == nil {
					return render(useResource(deps, fetch, &v))
				}
			}
			return render(useResource(deps, fetch, nil))
		})
	})
}
