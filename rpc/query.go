package rpc

import (
	"context"
	"sync"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

// UseQuery calls f with req's value once the component mounts, and again
// whenever req changes, as a hooks.Resource: Data, Loading and Err are
// signals to bind. A call superseded by a newer request is cancelled.
//
//	filter := core.NewSignal(api.ListReq{Done: false})
//	todos := rpc.UseQuery(api.ListTodos, filter)
//	h.ShowResource(todos, …)
//
// Like UseResource, it never runs during server rendering.
func UseQuery[Req, Res any](f *Func[Req, Res], req *core.Signal[Req]) *hooks.Resource[Res] {
	// The call runs on a goroutine; it reads the request captured on the
	// render loop, not the signal.
	var mu sync.Mutex
	current := req.Peek()
	hooks.Watch([]core.SignalAccessor{req}, func() {
		mu.Lock()
		current = req.Peek()
		mu.Unlock()
	})
	return hooks.UseResource([]core.SignalAccessor{req}, func(ctx context.Context) (Res, error) {
		mu.Lock()
		r := current
		mu.Unlock()
		return f.Call(ctx, r)
	})
}
