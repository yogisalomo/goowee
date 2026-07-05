package dom

import (
	"encoding/json"
	"github.com/yogisalomo/goowee/core"
	"log"
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

var eventUsesCapture = map[string]bool{
	"focus": true, "blur": true, "scroll": true,
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
			r.OnNewEventType(event, eventUsesCapture[event])
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

func EventCapture(event string) bool {
	return eventUsesCapture[event]
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
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("goowee: panic in %s handler for node %d: %v", event, nodeID, rec)
		}
	}()
	entry.Fn(core.EventData{Type: event, Target: nodeID, Data: data})
	opts = entry.Options
	return
}
