package dom

import (
	"encoding/json"

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

	OnNewEventType func(eventType string, capture bool)
}

// nonBubbling lists the events that don't bubble. The runtime listens for
// them in the capture phase (otherwise they never reach the document-level
// listener) and dispatches them to the target's handler only, like the DOM.
var nonBubbling = map[string]bool{
	"focus": true, "blur": true, "scroll": true, "scrollend": true,
	"mouseenter": true, "mouseleave": true, "pointerenter": true, "pointerleave": true,
	"load": true, "error": true, "abort": true, "invalid": true, "toggle": true,
	"cancel": true, "close": true,
	// media
	"play": true, "pause": true, "playing": true, "ended": true, "waiting": true,
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

	var data map[string]any
	_ = json.Unmarshal([]byte(dataJSON), &data)

	handled = true
	opts = entry.Options
	ev := core.NewEventData(event, nodeID, data)
	defer func() {
		if rec := recover(); rec != nil {
			core.Log(core.LogRecoverEventHandler, "panic in event handler", map[string]any{
				"event":  event,
				"nodeId": nodeID,
				"panic":  rec,
			})
		}
		// Static options plus whatever the handler decided while running.
		pd, sp := ev.Decisions()
		opts.PreventDefault = opts.PreventDefault || pd
		opts.StopPropagation = opts.StopPropagation || sp
	}()
	// Batched: a handler that writes several signals notifies each once, after
	// it returns, so derived state never sees a half-applied update.
	core.Batch(func() { entry.Fn(ev) })
	return
}
