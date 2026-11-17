package core

// Ref is a handle to a rendered element's DOM node. Attach one with h.RefTo,
// then call imperative methods from event handlers or effects — focus a field
// after a validation error, scroll a message into view, etc.
//
// Commands (Focus, Blur, …, Call) are one-way and run on the next frame, so
// they're for post-render actions, not setup. Get reads a value back: it's answered on
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

// Call calls method on the referenced node with args, once the frame's DOM
// updates are applied, so it sees state the same handler just set. Use it for
// DOM methods that take arguments, such as putting the caret after text a
// handler inserted into a textarea:
//
//	setDraft(before + mention + after)
//	at := len(utf16.Encode([]rune(before + mention))) // JS offsets are UTF-16 units
//	ref.Call("setSelectionRange", at, at)
//
// Like Focus it is fire-and-forget: the result is discarded (read values with
// Get), and an exception is logged to the console rather than stopping the
// frame. args must encode to JSON: numbers, strings, bools, slices, maps.
// No-op before the element has rendered.
func (r *Ref) Call(method string, args ...any) {
	if r == nil || r.ID == 0 {
		return
	}
	if s := scheduler(); s != nil {
		s.Enqueue(Mutation{Type: MutCall, NodeID: r.ID, Key: method, Value: append([]any{}, args...)})
	}
}

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
// To measure right after mount, read from OnMount — it runs once the
// component's DOM is in the document:
//
//	hooks.OnMount(func() func() {
//	    ref.Get("offsetHeight", func(v any) { … })
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

// Draw sends a goowee/canvas command buffer to the referenced <canvas>. It is
// drawn on the next frame, after that frame's DOM updates, and replaces the
// canvas's previous picture unless the buffer says not to clear. The slice is
// read when the frame is sent: until then, change it only to record a newer
// picture for the same canvas; after that, reuse it freely. A later Draw to
// the same canvas before the frame replaces this one. No-op before the element
// has rendered.
//
// Most apps use canvas.View, which records and draws for you; use Draw with a
// canvas.Ctx when the drawing is driven from elsewhere:
//
//	c := canvas.NewCtx(300, 150)
//	c.FillRect(10, 10, 50, 50)
//	ref.Draw(c.Commands())
//
// The runtime sizes the canvas's backing store for the screen's device pixel
// ratio from the width and height in the buffer; give the element the same
// size in CSS pixels (canvas.View does), or it is displayed at the backing
// store's size.
func (r *Ref) Draw(cmds []uint32) {
	if r == nil || r.ID == 0 || len(cmds) == 0 {
		return
	}
	if s := scheduler(); s != nil {
		s.Enqueue(Mutation{Type: MutDraw, NodeID: r.ID, Value: cmds})
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
