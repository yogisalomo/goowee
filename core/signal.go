package core

import "reflect"

type Signal[T any] struct {
	value T
	subs  []func()
}

func NewSignal[T any](v T) *Signal[T] {
	return &Signal[T]{value: v}
}

func (s *Signal[T]) Get() T {
	return s.value
}

func (s *Signal[T]) Set(v T) {
	s.value = v
	for _, fn := range s.subs {
		fn()
	}
}

func (s *Signal[T]) Subscribe(fn func()) func() {
	s.subs = append(s.subs, fn)
	return func() {
		fnPtr := reflect.ValueOf(fn).Pointer()
		for i, sub := range s.subs {
			if reflect.ValueOf(sub).Pointer() == fnPtr {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

func (s *Signal[T]) Value() any { return s.value }

var _ SignalAccessor = (*Signal[int])(nil)
var _ SignalAccessor = (*Signal[string])(nil)
