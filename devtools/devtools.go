package devtools

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yogisalomo/goowee/core"
)

var (
	mu      sync.RWMutex
	enabled bool
	root    core.Node

	nextSignalID int
	signals      map[uintptr]signalMeta
)

type signalMeta struct {
	ID        int
	Kind      string
	OwnerPath string
	Sig       core.SignalAccessor
}

// Enable turns on signal registration and the component/scope inspector.
// Safe to call multiple times.
func Enable() {
	mu.Lock()
	enabled = true
	if signals == nil {
		signals = make(map[uintptr]signalMeta)
	}
	mu.Unlock()
	wireHooks()
}

// Enabled reports whether devtools are active.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// SetRoot stores the mounted app root for Snapshot. Call after the initial
// render so ComponentNode/ScopeNode Prev and Frame fields are populated.
func SetRoot(n core.Node) {
	mu.Lock()
	root = n
	mu.Unlock()
}

// RegisterSignal records a signal for the inspector when devtools are enabled.
// kind is a short label ("state", "resource.data", "computed", …).
func RegisterSignal(sig core.SignalAccessor, kind string) {
	if sig == nil {
		return
	}
	mu.RLock()
	on := enabled
	mu.RUnlock()
	if !on {
		return
	}

	key := signalKey(sig)
	mu.Lock()
	defer mu.Unlock()
	if _, ok := signals[key]; ok {
		return
	}
	nextSignalID++
	path := ""
	if core.CurrentComponentPath != nil {
		path = core.CurrentComponentPath()
	}
	signals[key] = signalMeta{
		ID:        nextSignalID,
		Kind:      kind,
		OwnerPath: path,
		Sig:       sig,
	}
}

// SignalID returns the inspector id for sig, or 0 when unknown/disabled.
func SignalID(sig core.SignalAccessor) int {
	if sig == nil {
		return 0
	}
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := signals[signalKey(sig)]; ok {
		return m.ID
	}
	return 0
}

func signalKey(sig core.SignalAccessor) uintptr {
	v := reflect.ValueOf(sig)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

// Snapshot returns a JSON-serializable view of the component/scope tree and
// registered signal graph. Requires SetRoot after mount.
func Snapshot() map[string]any {
	mu.RLock()
	r := root
	sigCopy := make([]map[string]any, 0, len(signals))
	for _, m := range signals {
		sigCopy = append(sigCopy, signalInfo(m))
	}
	mu.RUnlock()

	out := map[string]any{
		"signals": sigCopy,
	}
	if r != nil {
		out["tree"] = walkNode(r)
	}
	return out
}

func signalInfo(m signalMeta) map[string]any {
	info := map[string]any{
		"id":    m.ID,
		"kind":  m.Kind,
		"owner": m.OwnerPath,
		"value": formatValue(m.Sig.Value()),
	}
	if sc, ok := m.Sig.(subscriberCount); ok {
		info["subscribers"] = sc.SubscriberCount()
	}
	return info
}

type subscriberCount interface {
	SubscriberCount() int
}

func formatValue(v any) string {
	if v == nil {
		return "nil"
	}
	if err, ok := v.(error); ok {
		if err == nil {
			return "nil"
		}
		return err.Error()
	}
	return fmt.Sprintf("%v", v)
}

func walkNode(n core.Node) any {
	if n == nil {
		return nil
	}
	switch v := n.(type) {
	case *core.ComponentNode:
		node := map[string]any{
			"type": "component",
			"name": v.Name,
		}
		if v.Frame != nil {
			node["path"] = v.Frame.Path
			node["hooks"] = frameHooks(v.Frame)
		}
		if v.Prev != nil {
			node["children"] = walkChildren(v.Prev)
		}
		return node
	case *core.ScopeNode:
		node := map[string]any{
			"type": "scope",
			"deps": depIDs(v.Deps),
		}
		if v.Prev != nil {
			node["children"] = walkChildren(v.Prev)
		}
		return node
	case *core.ElementNode:
		node := map[string]any{
			"type": "element",
			"tag":  v.Tag,
			"id":   v.ID,
		}
		if len(v.Children) > 0 {
			node["children"] = walkChildList(v.Children)
		}
		return node
	case *core.TextNode:
		return map[string]any{
			"type":  "text",
			"id":    v.ID,
			"value": v.Value,
		}
	case *core.FragmentNode:
		return map[string]any{
			"type":     "fragment",
			"children": walkChildList(v.Children),
		}
	case *core.ErrorBoundaryNode:
		node := map[string]any{"type": "error_boundary"}
		if v.Prev != nil {
			node["children"] = walkChildren(v.Prev)
		}
		return node
	case *core.PortalNode:
		node := map[string]any{
			"type":   "portal",
			"target": v.Target,
		}
		if len(v.Children) > 0 {
			node["children"] = walkChildList(v.Children)
		}
		return node
	default:
		return map[string]any{"type": fmt.Sprintf("%T", n)}
	}
}

func walkChildren(n core.Node) any {
	switch v := n.(type) {
	case *core.FragmentNode:
		return walkChildList(v.Children)
	default:
		child := walkNode(n)
		if child == nil {
			return nil
		}
		return []any{child}
	}
}

func walkChildList(children []core.Node) []any {
	out := make([]any, 0, len(children))
	for _, c := range children {
		if w := walkNode(c); w != nil {
			out = append(out, w)
		}
	}
	return out
}

func frameHooks(frame *core.ComponentFrame) []map[string]any {
	var hooks []map[string]any
	for _, h := range frame.Hooks {
		switch v := h.(type) {
		case core.SignalAccessor:
			hooks = append(hooks, map[string]any{
				"kind":   "signal",
				"signal": SignalID(v),
			})
		default:
			hooks = append(hooks, map[string]any{
				"kind": fmt.Sprintf("%T", h),
			})
		}
	}
	return hooks
}

func depIDs(deps []core.SignalAccessor) []int {
	ids := make([]int, 0, len(deps))
	for _, d := range deps {
		if id := SignalID(d); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}
