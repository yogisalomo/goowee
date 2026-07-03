package core

import "fmt"

type Scheduler struct {
	queue []Mutation
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Enqueue(muts ...Mutation) {
	s.queue = append(s.queue, muts...)
}

func (s *Scheduler) Flush() []Mutation {
	out := s.queue
	s.queue = nil
	return out
}

func (s *Scheduler) Len() int {
	return len(s.queue)
}

func (s *Scheduler) String() string {
	return fmt.Sprintf("Scheduler(queue=%d)", len(s.queue))
}

type Mutation struct {
	Type    MutationType `json:"type"`
	NodeID  int          `json:"nodeId"`
	Key     string       `json:"key,omitempty"`
	Value   any          `json:"value,omitempty"`
	ChildID int          `json:"childId,omitempty"`
	RefID   int          `json:"refId,omitempty"`
}

type MutationType int

const (
	MutCreateElement MutationType = iota
	MutRemoveNode
	MutSetAttribute
	MutSetProperty
	MutAppendChild
	MutInsertBefore
	MutRemoveAttribute
)

func (mt MutationType) String() string {
	switch mt {
	case MutCreateElement:
		return "CreateElement"
	case MutRemoveNode:
		return "RemoveNode"
	case MutSetAttribute:
		return "SetAttribute"
	case MutSetProperty:
		return "SetProperty"
	case MutAppendChild:
		return "AppendChild"
	case MutInsertBefore:
		return "InsertBefore"
	case MutRemoveAttribute:
		return "RemoveAttribute"
	default:
		return "Unknown"
	}
}
