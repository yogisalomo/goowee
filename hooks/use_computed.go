package hooks

import "github.com/yogisalomo/goowee/core"

// UseComputed creates a derived signal that recomputes when deps change.
// It is a convenience re-export of core.Computed so that hooks can be the
// single import for all state/effect/computed functionality.
func UseComputed[T any](deps []core.SignalAccessor, compute func() T) *core.Signal[T] {
	return core.Computed(deps, compute)
}
