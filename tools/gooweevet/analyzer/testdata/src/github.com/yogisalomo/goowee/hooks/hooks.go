// Package hooks is a stub of goowee/hooks for the analyzer's tests.
package hooks

import "github.com/yogisalomo/goowee/core"

func UseState[T any](v T) (*core.Signal[T], func(T))                      { s := core.NewSignal(v); return s, s.Set }
func UseEffect(deps []core.SignalAccessor, fn func() func())              {}
func Watch(deps []core.SignalAccessor, fn func())                         {}
func UseScope(fn func() core.Node, deps ...core.SignalAccessor) core.Node { return fn() }
