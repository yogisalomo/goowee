package core

type SignalAccessor interface {
	Value() any
	Subscribe(func()) func()
}
