package runtime

import (
	"fmt"
	"strconv"

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
	r.bindings[nodeID] = append(r.bindings[nodeID], r.subscribe(nodeID, b))
}

// Update replaces nodeID's bindings with binds after a re-render, appending to
// muts only what the DOM needs (#137). A binding with the same target, name
// and signal as the one in the same position before keeps its subscription
// and sends nothing: the DOM already shows its signal's value. A binding whose
// signal changed (a per-render Computed, say, or a different signal) is
// subscribed afresh, and sends its value only when it differs from what the
// old binding of the same name showed.
func (r *BindingRegistry) Update(nodeID int, binds []core.Bind, muts *[]core.Mutation) {
	old := r.bindings[nodeID]
	if len(old) == len(binds) {
		same := true
		for i, b := range binds {
			if !sameBinding(old[i].bind, b) {
				same = false
				break
			}
		}
		if same {
			return // the common case: nothing to subscribe, nothing to send
		}
	}
	next := make([]boundBinding, 0, len(binds))
	kept := make([]bool, len(old))
	for i, b := range binds {
		if i < len(old) && sameBinding(old[i].bind, b) {
			next = append(next, old[i])
			kept[i] = true
			continue
		}
		// What the old binding of this name left in the DOM: send the new
		// value only if it differs.
		send := true
		for _, ob := range old {
			if ob.bind.Target == b.Target && ob.bind.Name == b.Name {
				send = !SafeEqual(ob.bind.Signal.Value(), b.Signal.Value())
				break
			}
		}
		if send {
			*muts = append(*muts, MutationForBind(nodeID, b))
		}
		next = append(next, r.subscribe(nodeID, b))
	}
	for i, ob := range old {
		if !kept[i] && ob.unsub != nil {
			ob.unsub()
		}
	}
	if len(next) == 0 {
		delete(r.bindings, nodeID)
		return
	}
	r.bindings[nodeID] = next
}

func sameBinding(a, b core.Bind) bool {
	return a.Target == b.Target && a.Name == b.Name && a.Signal == b.Signal
}

func (r *BindingRegistry) subscribe(nodeID int, b core.Bind) boundBinding {
	var slot core.Slot // its update's place in the frame: a rewrite replaces it
	unsub := b.Signal.Subscribe(func() {
		r.scheduler.EnqueueSlot(MutationForBind(nodeID, b), &slot)
	})
	return boundBinding{bind: b, unsub: unsub}
}

func (r *BindingRegistry) Unbind(nodeID int) {
	for _, bb := range r.bindings[nodeID] {
		if bb.unsub != nil {
			bb.unsub()
		}
	}
	delete(r.bindings, nodeID)
}

// MutationForBind turns a binding's current value into its initial DOM write.
// The same function is used when the signal changes, so SSR, hydration and
// live updates agree on what a binding means.
func MutationForBind(nodeID int, b core.Bind) core.Mutation {
	// Generic attribute and property bindings use the typed value path (#136):
	// no formatting or boxing on the hot update path. Targeted bindings need
	// their own mutation type and value shape, so they use the boxed path.
	if b.Target == core.BindToAttr || b.Target == core.BindToProp {
		if tv, ok := b.Signal.(core.TypedValuer); ok {
			m := core.Mutation{Type: core.MutSetProperty, NodeID: nodeID, Key: b.Name}
			if b.Target == core.BindToAttr {
				m.Type = core.MutSetAttribute
			}
			if m.Kind, m.Str, m.Bits = tv.TypedValue(); m.Kind != core.ValAny {
				// AttrS removes a false boolean; it must not write "false".
				if b.Target == core.BindToAttr && m.Kind == core.ValBool && !m.Bool() {
					return core.Mutation{Type: core.MutRemoveAttribute, NodeID: nodeID, Key: b.Name}
				}
				if m.Kind == core.ValString {
					m.Str = SafeURL(b.Name, m.Str) // textContent isn't a URL attribute
				} else if m.Type == core.MutSetAttribute || b.Name == "textContent" {
					m.AsText()
				}
				return m
			}
		}
	}
	switch b.Target {
	case core.BindToAttr:
		v := b.Signal.Value()
		if on, ok := v.(bool); ok && !on {
			return core.Mutation{Type: core.MutRemoveAttribute, NodeID: nodeID, Key: b.Name}
		}
		return core.Mutation{
			Type: core.MutSetAttribute, NodeID: nodeID, Key: b.Name,
			Value: SafeURL(b.Name, attrText(v)),
		}
	case core.BindToClass:
		on, _ := b.Signal.Value().(bool)
		return core.Mutation{Type: core.MutToggleClass, NodeID: nodeID, Key: b.Name, Value: on}
	case core.BindToStyle:
		return core.Mutation{
			Type: core.MutSetStyle, NodeID: nodeID, Key: b.Name,
			Value: styleValue(b.Signal.Value()), NS: b.Unit,
		}
	case core.BindToSelectMulti:
		values, _ := b.Signal.Value().([]string)
		return core.Mutation{Type: core.MutSetSelected, NodeID: nodeID, Value: values}
	}
	v := b.Signal.Value()
	if b.Name == "textContent" {
		v = TextValue(v)
	} else {
		v = SafeURLValue(b.Name, v)
	}
	return core.Mutation{
		Type: core.MutSetProperty, NodeID: nodeID, Key: b.Name,
		Value: v,
	}
}

// Len is how many bindings are live, across all nodes (tests check that
// removed nodes leave none behind).
func (r *BindingRegistry) Len() int {
	n := 0
	for _, bs := range r.bindings {
		n += len(bs)
	}
	return n
}

func attrText(v any) string {
	if on, ok := v.(bool); ok {
		if on {
			return "true"
		}
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// styleValue keeps numbers as numbers so the runtime can append a unit (px,
// %, …) without a Go-side format and allocation on every update. Strings are
// passed through, e.g. StyleProp("transform", sig) with "rotate(10deg)".
func styleValue(v any) any {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return fmt.Sprintf("%v", v)
	}
}

// StyleText is the SSR form of a style binding: the property and its value,
// with a unit appended to numeric values.
func StyleText(v any, unit string) string {
	s, ok := styleValue(v).(string)
	if ok {
		return s + unit
	}
	return strconv.FormatFloat(v.(float64), 'f', -1, 64) + unit
}
