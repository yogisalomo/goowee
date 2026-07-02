package hooks

import "goowee/core"

func UseScope(fn func() core.Node, deps ...core.SignalAccessor) core.Node {
	return &core.ScopeNode{Render: fn, Deps: deps}
}
