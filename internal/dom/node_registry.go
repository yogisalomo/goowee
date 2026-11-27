package dom

import (
	"strconv"

	"github.com/yogisalomo/goowee/core"
)

type HandlerEntry struct {
	Fn      func(core.EventData)
	Options core.HandlerOptions
}

type DOMNode struct {
	ID     int
	Events map[string]HandlerEntry
}

type NodeRegistry struct {
	nodes     map[int]*DOMNode
	announced map[string]bool
	ctls      []*core.EventControl // free event controls, reused by Dispatch

	OnNewEventType func(eventType string, capture bool)

	// WarnDynamicPrevent is set when Go runs in a worker (#158): a handler's
	// e.PreventDefault() that no static option or filter covers is logged,
	// once per event type, since the page dispatched the event already.
	WarnDynamicPrevent bool
	warnedPrevent      map[string]bool
}

// nonBubbling lists the events that don't bubble. The runtime listens for
// them in the capture phase (otherwise they never reach the document-level
// listener) and dispatches them to the target's handler only, like the DOM.
var nonBubbling = map[string]bool{
	"focus": true, "blur": true, "scroll": true, "scrollend": true,
	"mouseenter": true, "mouseleave": true, "pointerenter": true, "pointerleave": true,
	"load": true, "error": true, "abort": true, "invalid": true, "toggle": true, "beforetoggle": true,
	"cancel": true, "close": true,
	// goowee/canvas: a Fill canvas's size changed (canvas.ResizeEvent)
	"goowee:canvasresize": true,
	// media
	"loadstart": true, "play": true, "pause": true, "playing": true, "ended": true, "waiting": true,
	"seeking": true, "seeked": true, "timeupdate": true, "volumechange": true,
	"ratechange": true, "durationchange": true, "loadeddata": true,
	"loadedmetadata": true, "canplay": true, "canplaythrough": true,
	"emptied": true, "stalled": true, "suspend": true, "progress": true,
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:     make(map[int]*DOMNode),
		announced: make(map[string]bool),
	}
}

func (r *NodeRegistry) RegisterHandler(nodeID int, event string, fn func(core.EventData), opts core.HandlerOptions) {
	n := r.nodes[nodeID]
	if n == nil {
		n = &DOMNode{ID: nodeID, Events: map[string]HandlerEntry{}}
		r.nodes[nodeID] = n
	}
	n.Events[event] = HandlerEntry{Fn: fn, Options: opts}
	if !r.announced[event] {
		r.announced[event] = true
		if r.OnNewEventType != nil {
			r.OnNewEventType(event, nonBubbling[event])
		}
	}
}

// Options returns the options of nodeID's handler for event.
func (r *NodeRegistry) Options(nodeID int, event string) (core.HandlerOptions, bool) {
	n := r.nodes[nodeID]
	if n == nil {
		return core.HandlerOptions{}, false
	}
	e, ok := n.Events[event]
	return e.Options, ok
}

// Has reports whether nodeID has a handler for event.
func (r *NodeRegistry) Has(nodeID int, event string) bool {
	n := r.nodes[nodeID]
	if n == nil {
		return false
	}
	_, ok := n.Events[event]
	return ok
}

func (r *NodeRegistry) RemoveHandler(nodeID int, event string) {
	if n := r.nodes[nodeID]; n != nil {
		delete(n.Events, event)
	}
}

func (r *NodeRegistry) Remove(nodeID int) {
	delete(r.nodes, nodeID)
}

func (r *NodeRegistry) EventTypes() []string {
	var types []string
	for t := range r.announced {
		types = append(types, t)
	}
	return types
}

// EventCapture reports whether event doesn't bubble, i.e. must be listened
// for in the capture phase and dispatched to its target only.
func EventCapture(event string) bool {
	return nonBubbling[event]
}

func (r *NodeRegistry) Dispatch(nodeID int, event string, dataJSON string) (opts core.HandlerOptions, handled bool) {
	n := r.nodes[nodeID]
	if n == nil {
		return
	}
	entry, ok := n.Events[event]
	if !ok {
		return
	}

	// The payload stays JSON text: each accessor the handler calls reads its
	// field from it (#135). The decisions go into a control reused across
	// events; a stack, since a handler can dispatch another event (a focus()
	// inside a click handler) before it returns.
	var ctl *core.EventControl
	if n := len(r.ctls); n > 0 {
		ctl, r.ctls = r.ctls[n-1], r.ctls[:n-1]
		ctl.Reset()
	} else {
		ctl = new(core.EventControl)
	}

	handled = true
	opts = entry.Options
	ev := core.NewEventDataJSON(event, nodeID, dataJSON, ctl)
	defer func() {
		r.ctls = append(r.ctls, ctl)
		if rec := recover(); rec != nil {
			core.Log(core.LogRecoverEventHandler, "panic in event handler", core.PanicFields(rec, map[string]any{
				"event":  event,
				"nodeId": nodeID,
			}))
		}
		// Static options plus whatever the handler decided while running.
		// A PreventWhen match is the runtime's decision; report it too, so
		// gooweetest says "prevented" exactly when the browser would.
		pd, sp := ev.Decisions()
		covered := opts.PreventDefault || entry.Options.PreventWhen.Match(ev)
		if pd && !covered && r.WarnDynamicPrevent && !r.warnedPrevent[event] {
			// In a worker the event was dispatched on the page before this
			// handler ran: its decision comes too late (#158).
			if r.warnedPrevent == nil {
				r.warnedPrevent = map[string]bool{}
			}
			r.warnedPrevent[event] = true
			core.Log(core.LogWarn, "e.PreventDefault() has no effect in worker mode; use h.PreventDefault() or h.PreventDefaultWhen(...)", map[string]any{"event": event})
		}
		opts.PreventDefault = covered || pd
		opts.StopPropagation = opts.StopPropagation || sp
	}()
	// Batched: a handler that writes several signals notifies each once, after
	// it returns, so derived state never sees a half-applied update.
	core.Batch(func() { entry.Fn(ev) })
	return
}

// listen is the mutation that tells the runtime a node has (on) or no longer
// has a handler for event, so it calls into Go only for nodes that do (#132).
// A handler with static options or a PreventDefaultWhen condition sends them
// instead of true, as "flags" or "flags\nspec" (flags: 1 prevent default, 2
// stop propagation, 4 select on focus), so the runtime can apply them itself
// before Go runs: it must, when Go runs in a worker (#158).
func listen(nodeID int, event string, on bool, opts core.HandlerOptions) core.Mutation {
	var v any = on
	if on {
		flags := 0
		if opts.PreventDefault {
			flags |= 1
		}
		if opts.StopPropagation {
			flags |= 2
		}
		if opts.SelectOnFocus {
			flags |= 4
		}
		if spec := opts.PreventWhen.Spec(); spec != "" {
			v = strconv.Itoa(flags) + "\n" + spec
		} else if flags != 0 {
			v = strconv.Itoa(flags)
		}
	}
	return core.Mutation{Type: core.MutListen, NodeID: nodeID, Key: event, Value: v}
}

// listenChanged reports whether the runtime must be told about hd on id: it
// is new there, or its options changed.
func (r *NodeRegistry) listenChanged(id int, hd core.Handler) bool {
	old, ok := r.Options(id, hd.Event)
	return !ok || old != hd.Options
}

// registerHandlers registers an element's handlers and tells the runtime.
func (r *DOMRenderer) registerHandlers(id int, handlers []core.Handler) {
	for _, hd := range handlers {
		if r.Registry.listenChanged(id, hd) {
			*r.muts = append(*r.muts, listen(id, hd.Event, true, hd.Options))
		}
		r.Registry.RegisterHandler(id, hd.Event, hd.Fn, hd.Options)
	}
}

// Dispatch result bits, returned to the runtime by handleEvent as one number.
const (
	DispatchHandled = 1 << iota
	DispatchPreventDefault
	DispatchStopPropagation
	DispatchSelectOnFocus
)

// DispatchBits packs a Dispatch result for the runtime.
func DispatchBits(opts core.HandlerOptions, handled bool) int {
	if !handled {
		return 0
	}
	b := DispatchHandled
	if opts.PreventDefault {
		b |= DispatchPreventDefault
	}
	if opts.StopPropagation {
		b |= DispatchStopPropagation
	}
	if opts.SelectOnFocus {
		b |= DispatchSelectOnFocus
	}
	return b
}

// Len is how many nodes have a handler registered (tests check that removed
// nodes leave none behind).
func (r *NodeRegistry) Len() int {
	n := 0
	for _, node := range r.nodes {
		if len(node.Events) > 0 {
			n++
		}
	}
	return n
}
