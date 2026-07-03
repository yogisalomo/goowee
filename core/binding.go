package core

import "fmt"

type SignalAccessor interface {
	Value() any
	Subscribe(func()) func()
}

type boundBinding struct {
	bind  Bind
	unsub func()
}

type BindingRegistry struct {
	bindings  map[int][]boundBinding
	scheduler *Scheduler
}

func NewBindingRegistry(scheduler *Scheduler) *BindingRegistry {
	return &BindingRegistry{
		bindings:  make(map[int][]boundBinding),
		scheduler: scheduler,
	}
}

func (r *BindingRegistry) Bind(nodeID int, b Bind) {
	unsub := b.Signal.Subscribe(func() {
		r.scheduler.Enqueue(MutationForBind(nodeID, b))
	})
	r.bindings[nodeID] = append(r.bindings[nodeID], boundBinding{bind: b, unsub: unsub})
}

func (r *BindingRegistry) Unbind(nodeID int) {
	for _, bb := range r.bindings[nodeID] {
		if bb.unsub != nil {
			bb.unsub()
		}
	}
	delete(r.bindings, nodeID)
}

func MutationForBind(nodeID int, b Bind) Mutation {
	if b.Target == BindToAttr {
		return Mutation{
			Type: MutSetAttribute, NodeID: nodeID, Key: b.Name,
			Value: fmt.Sprintf("%v", b.Signal.Value()),
		}
	}
	return Mutation{
		Type: MutSetProperty, NodeID: nodeID, Key: b.Name,
		Value: b.Signal.Value(),
	}
}
