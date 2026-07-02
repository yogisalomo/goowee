package dom

import (
	"encoding/json"
	"goowee/core"
)

type EventHandler func(event core.EventData)

type DOMNode struct {
	ID     int
	Events map[string]EventHandler
}

type NodeRegistry struct {
	nodes map[int]*DOMNode
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{nodes: make(map[int]*DOMNode)}
}

func (r *NodeRegistry) GetOrCreate(nodeID int) *DOMNode {
	if n, ok := r.nodes[nodeID]; ok {
		return n
	}
	n := &DOMNode{ID: nodeID, Events: make(map[string]EventHandler)}
	r.nodes[nodeID] = n
	return n
}

func (r *NodeRegistry) RegisterHandler(nodeID int, eventType string, handler EventHandler) {
	r.GetOrCreate(nodeID).Events[eventType] = handler
}

func (r *NodeRegistry) Dispatch(nodeID int, eventType string, data string) {
	n, ok := r.nodes[nodeID]
	if !ok {
		return
	}
	h, ok := n.Events[eventType]
	if !ok {
		return
	}
	var dataMap map[string]any
	json.Unmarshal([]byte(data), &dataMap)
	if dataMap == nil {
		dataMap = make(map[string]any)
	}
	h(core.EventData{
		Type:   eventType,
		Target: nodeID,
		Data:   dataMap,
	})
}

func (r *NodeRegistry) Remove(nodeID int) {
	delete(r.nodes, nodeID)
}
