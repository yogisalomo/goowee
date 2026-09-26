package core

// Ref is a handle to a rendered element's DOM node. Attach one with h.RefTo,
// then call imperative methods from event handlers or effects — focus a field
// after a validation error, scroll a message into view, etc.
//
// Commands (Focus, Blur, …) are one-way and run on the next frame, so they're
// for post-render actions, not setup. Get reads a value back: it's answered on
// the frame after pending DOM updates apply, so a handler can update state and
// measure the result in one go.
type Ref struct {
	ID int // the node id, filled in when the element renders
}

// Focus, Blur, Click, and ScrollIntoView issue the matching DOM call on the
// referenced node. No-ops before the element has rendered.
func (r *Ref) Focus()          { r.invoke("focus") }
func (r *Ref) Blur()           { r.invoke("blur") }
func (r *Ref) Click()          { r.invoke("click") }
func (r *Ref) ScrollIntoView() { r.invoke("scrollIntoView") }

// Get reads prop from the referenced node — scrollTop, offsetWidth,
// selectionStart, value, checkValidity — and hands it to fn. The read runs on
// the next frame after that frame's DOM updates are applied; fn then runs on
// the render loop, so it may Set signals. If prop names a method it is called
// with no arguments and its result is returned.
//
// v is the JSON-decoded value: float64 for numbers, string, bool,
// map[string]any for objects (e.g. getBoundingClientRect), or nil when the
// property is undefined or the node is gone.
//
//	ref.Get("getBoundingClientRect", func(v any) {
//	    rect, _ := v.(map[string]any)
//	    w, _ := rect["width"].(float64)
//	    setWidth(int(w))
//	})
//
// To measure right after mount, defer the read past setup — the element has
// no id until the component's tree is walked:
//
//	hooks.OnMount(func() func() {
//	    core.Schedule(func() { ref.Get("offsetHeight", …) })
//	    return nil
//	})
//
// No-op (fn never runs) before the element has rendered or when no scheduler
// is active (SSR, tests without a client).
func (r *Ref) Get(prop string, fn func(v any)) {
	if r == nil || r.ID == 0 || fn == nil {
		return
	}
	if s := scheduler(); s != nil {
		s.Read(r.ID, prop, fn)
	}
}

// invoke queues a method call on the node via the active scheduler, which the
// bridge applies to the real DOM node. Safe to call from event handlers and
// effects (and, like Schedule, from goroutines).
func (r *Ref) invoke(method string) {
	if r == nil || r.ID == 0 {
		return
	}
	if s := scheduler(); s != nil {
		s.Enqueue(Mutation{Type: MutInvoke, NodeID: r.ID, Key: method})
	}
}
