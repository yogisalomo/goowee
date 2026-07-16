package h

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
)

// ShowResource renders a Resource's three states (loading, error, data) with
// caller-provided views. This eliminates the repetitive Show/ShowElse
// boilerplate that every UseResource call site otherwise needs.
//
//	res := hooks.UseResource(nil, fetchUser)
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
	hasErr := core.Computed([]core.SignalAccessor{res.Err}, func() bool {
		return res.Err.Get() != nil
	})
	return &core.FragmentNode{
		Children: []core.Node{
			Show(res.Loading, loading),
			ShowElse(hasErr,
				func() core.Node { return errFn(res.Err.Get()) },
				func() core.Node { return data(res.Data) },
			),
		},
	}
}
