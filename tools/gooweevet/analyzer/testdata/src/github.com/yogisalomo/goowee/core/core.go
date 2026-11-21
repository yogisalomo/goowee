// Package core is a stub of goowee/core for the analyzer's tests.
package core

type Node interface{}
type Item interface{}
type SignalAccessor interface{ Subscribe(func()) func() }

type Signal[T any] struct{ v T }

func NewSignal[T any](v T) *Signal[T]                              { return &Signal[T]{v: v} }
func (s *Signal[T]) Get() T                                        { return s.v }
func (s *Signal[T]) Peek() T                                       { return s.v }
func (s *Signal[T]) Set(v T)                                       { s.v = v }
func (s *Signal[T]) Subscribe(func()) func()                       { return func() {} }
func Computed[T any](deps []SignalAccessor, f func() T) *Signal[T] { return &Signal[T]{v: f()} }
func Deps(d ...SignalAccessor) []SignalAccessor                    { return d }
func Component(name string, render func() Node) Node               { return render() }
func Schedule(fn func())                                           { fn() }
