package runtime

import (
	"fmt"
	"github.com/yogisalomo/goowee/core"
)

type boundBinding struct {
	bind  core.Bind
	unsub func()
}

type BindingRegistry struct {
	bindings  map[int][]boundBinding
	scheduler *core.Scheduler
}

func NewBindingRegistry(scheduler *core.Scheduler) *BindingRegistry {
	return &BindingRegistry{
		bindings:  make(map[int][]boundBinding),
		scheduler: scheduler,
	}
}

func (r *BindingRegistry) Bind(nodeID int, b core.Bind) {
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

func MutationForBind(nodeID int, b core.Bind) core.Mutation {
	if b.Target == core.BindToAttr {
		return core.Mutation{
			Type: core.MutSetAttribute, NodeID: nodeID, Key: b.Name,
			Value: fmt.Sprintf("%v", b.Signal.Value()),
		}
	}
	return core.Mutation{
		Type: core.MutSetProperty, NodeID: nodeID, Key: b.Name,
		Value: b.Signal.Value(),
	}
}
