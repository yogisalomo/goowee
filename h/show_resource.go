package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

// ShowResource renders a Resource's three states (loading, error, data) with
// caller-provided views. This eliminates the repetitive Show/ShowElse
// boilerplate that every UseResource call site otherwise needs.
//
//	res := hooks.UseResource(nil, fetchUser) // fetchUser(ctx) (User, error)
//	return ShowResource(res,
//	    func() core.Node { return P(Text("Loading…")) },
//	    func(err error) core.Node { return P(Textf("Error: %s", err)) },
//	    func(data *core.Signal[User]) core.Node { return userView(data) },
//	)
func ShowResource[T any](
	res *hooks.Resource[T],
	loading func() core.Node,
	errFn func(error) core.Node,
	data func(*core.Signal[T]) core.Node,
) core.Node {
	// One view at a time: loading, else the error, else the data.
	state := core.Computed([]core.SignalAccessor{res.Loading, res.Err}, func() int {
		switch {
		case res.Loading.Get():
			return 0
		case res.Err.Get() != nil:
			return 1
		default:
			return 2
		}
	})
	return Switch(state, map[int]func() core.Node{
		0: loading,
		1: func() core.Node { return errFn(res.Err.Peek()) }, // the state computed covers Err
		2: func() core.Node { return data(res.Data) },
	}, nil)
}
