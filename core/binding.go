package core

type SignalAccessor interface {
	Value() any
	Subscribe(func()) func()
}

type Binding struct {
	NodeID   int
	Signal   SignalAccessor
	Property string
}

type BindingRegistry struct {
	bindings  map[int][]Binding
	scheduler *Scheduler
}

func NewBindingRegistry(scheduler *Scheduler) *BindingRegistry {
	return &BindingRegistry{
		bindings:  make(map[int][]Binding),
		scheduler: scheduler,
	}
}

func (r *BindingRegistry) Bind(nodeID int, sig SignalAccessor, prop string) {
	r.bindings[nodeID] = append(r.bindings[nodeID], Binding{
		NodeID: nodeID, Signal: sig, Property: prop,
	})
	sig.Subscribe(func() {
		r.scheduler.Enqueue(r.GetMutationsFor(nodeID)...)
	})
}

func (r *BindingRegistry) Unbind(nodeID int) {
	delete(r.bindings, nodeID)
}

func (r *BindingRegistry) GetMutationsFor(nodeID int) []Mutation {
	var muts []Mutation
	for _, b := range r.bindings[nodeID] {
		muts = append(muts, Mutation{
			Type:    MutSetProperty,
			NodeID:  nodeID,
			Key:     b.Property,
			Value:   b.Signal.Value(),
		})
	}
	return muts
}
