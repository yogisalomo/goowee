package devtools

import (
	"fmt"
	"reflect"
	"sync"
	"time"

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
	Name      string // from Signal.Named
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
	if started.IsZero() {
		started = time.Now() // the change log's clock
	}
	mu.Unlock()
	wireHooks()
	core.SetDevChecks(true) // report undeclared signal dependencies (#74)
	core.SetSignalWriteHook(recordChange)
	for _, n := range core.TakeEarlyNames() { // a store in a package-level var
		NameSignal(n.Signal, n.Name)
	}
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

	mu.Lock()
	defer mu.Unlock()
	register(sig, kind)
}

// NameSignal gives sig a name for the inspector and the change log,
// registering it first if needed. core calls it for Signal.Named; it reports
// false, keeping nothing, when devtools are off.
func NameSignal(sig core.SignalAccessor, name string) bool {
	if sig == nil {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return false
	}
	m := register(sig, "signal")
	m.Name = name
	signals[signalKey(sig)] = m
	return true
}

// register records sig under kind unless it is known already, and returns its
// entry. mu must be held.
func register(sig core.SignalAccessor, kind string) signalMeta {
	key := signalKey(sig)
	if m, ok := signals[key]; ok {
		return m
	}
	nextSignalID++
	path := ""
	if core.CurrentComponentPath != nil {
		path = core.CurrentComponentPath()
	}
	m := signalMeta{
		ID:        nextSignalID,
		Kind:      kind,
		OwnerPath: path,
		Sig:       sig,
	}
	signals[key] = m
	return m
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
	metas := make([]signalMeta, 0, len(signals))
	for _, m := range signals {
		metas = append(metas, m)
	}
	mu.RUnlock()
	// Read values unlocked: a read can recompute a Computed, and a write
	// during it would wait on mu for the change log.
	sigCopy := make([]map[string]any, 0, len(metas))
	for _, m := range metas {
		sigCopy = append(sigCopy, signalInfo(m))
	}

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
	if m.Name != "" {
		info["name"] = m.Name
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
	return fmt.Sprintf("%+v", v) // a struct with its field names
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
