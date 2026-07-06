package core

// Ref is a handle to a rendered element's DOM node. Attach one with h.RefTo,
// then call imperative methods from event handlers or effects — focus a field
// after a validation error, scroll a message into view, etc.
//
// A ref can't read from the DOM (measuring is not supported yet — that needs a
// value channel back to Go); it issues one-way commands. The command runs on
// the next frame, so it's for post-render actions, not setup.
type Ref struct {
	ID int // the node id, filled in when the element renders
}

// Focus, Blur, Click, and ScrollIntoView issue the matching DOM call on the
// referenced node. No-ops before the element has rendered.
func (r *Ref) Focus()          { r.invoke("focus") }
func (r *Ref) Blur()           { r.invoke("blur") }
func (r *Ref) Click()          { r.invoke("click") }
func (r *Ref) ScrollIntoView() { r.invoke("scrollIntoView") }

// invoke queues a method call on the node via the active scheduler, which the
// bridge applies to the real DOM node. Safe to call from event handlers and
// effects (and, like Schedule, from goroutines).
func (r *Ref) invoke(method string) {
	if r == nil || r.ID == 0 {
		return
	}
	activeSchedulerMu.Lock()
	s := activeScheduler
	activeSchedulerMu.Unlock()
	if s != nil {
		s.Enqueue(Mutation{Type: MutInvoke, NodeID: r.ID, Key: method})
	}
}
