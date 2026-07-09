package core

import "fmt"

type Node interface {
	nodeMarker()
	String() string
	Apply(el *ElementNode)
}

type ElementNode struct {
	ID       int
	Tag      string
	Key      any
	Ref      *Ref
	Attrs    []Attr
	Props    []Prop
	Binds    []Bind
	Handlers []Handler
	Children []Node
	// Dynamic marks a subtree as non-deterministic (its server and client
	// output may differ — dates, locale, random). During hydration the nodes
	// are still claimed, but their attributes/properties/text are re-applied
	// so the client value wins. See h.Dynamic and ADR-016.
	Dynamic bool
	// Namespace is the XML namespace URI for this element (e.g. SVG). Empty for
	// ordinary HTML. Descendants inherit it, so only the subtree root (e.g. the
	// <svg>) needs it set. The DOM backend creates namespaced elements with
	// createElementNS. See h.Svg.
	Namespace string
}

// SVGNamespace is the XML namespace URI for SVG elements.
const SVGNamespace = "http://www.w3.org/2000/svg"

func (e *ElementNode) nodeMarker() {}
func (e *ElementNode) String() string {
	return fmt.Sprintf("Element(%s)", e.Tag)
}

type Attr struct {
	Name  string
	Value string
}

type Prop struct {
	Name  string
	Value any
}

type BindTarget int

const (
	BindToProp BindTarget = iota
	BindToAttr
)

type Bind struct {
	Target BindTarget
	Name   string
	Signal SignalAccessor
}

type Handler struct {
	Event   string
	Fn      func(EventData)
	Options HandlerOptions
}

type HandlerOptions struct {
	PreventDefault  bool
	StopPropagation bool
	SelectOnFocus   bool // select all text when the element receives focus
}

type Item interface {
	Apply(el *ElementNode)
}

func (e *ElementNode) Apply(parent *ElementNode) {
	if e == nil {
		return
	}
	parent.Children = append(parent.Children, e)
}

func (t *TextNode) Apply(parent *ElementNode) {
	if t == nil {
		return
	}
	parent.Children = append(parent.Children, t)
}

func (f *FragmentNode) Apply(parent *ElementNode) {
	if f == nil {
		return
	}
	parent.Children = append(parent.Children, f)
}

func (c *ComponentNode) Apply(parent *ElementNode) {
	if c == nil {
		return
	}
	parent.Children = append(parent.Children, c)
}

func (s *ScopeNode) Apply(parent *ElementNode) {
	if s == nil {
		return
	}
	parent.Children = append(parent.Children, s)
}

type TextNode struct {
	ID    int
	Value any
}

func (t *TextNode) nodeMarker() {}
func (t *TextNode) String() string {
	return fmt.Sprintf("Text(%v)", t.Value)
}

type FragmentNode struct {
	Children []Node
}

func (f *FragmentNode) nodeMarker() {}
func (f *FragmentNode) String() string {
	return fmt.Sprintf("Fragment(%d children)", len(f.Children))
}

// PortalNode renders its children into a different container in the DOM — a
// modal or tooltip that lives outside the current subtree. Target is a CSS
// selector resolved at apply time. Portals are client-side: they are not
// server-rendered, and their content is created fresh (not hydrated). Because
// the target is a selector (not a node we can reconcile against), portal
// children are rebuilt on every re-render — keep portal content simple, or
// hold its state in signals outside the portal. See ADR-017.
type PortalNode struct {
	Target   string
	Children []Node
}

func (p *PortalNode) nodeMarker() {}
func (p *PortalNode) String() string {
	return fmt.Sprintf("Portal(%s)", p.Target)
}
func (p *PortalNode) Apply(el *ElementNode) {
	if p == nil {
		return
	}
	el.Children = append(el.Children, p)
}

// ErrorBoundaryNode renders Child, but if rendering Child panics it renders
// Fallback(err) instead — so a failure in one subtree shows a message rather
// than blanking the page. See h.ErrorBoundary / ADR-018. Update-time panics in
// the subtree are contained (the subtree keeps its previous state) but do not
// switch to the fallback; fallback is for render-time failures.
type ErrorBoundaryNode struct {
	Fallback func(err any) Node
	Child    Node
	Prev     Node // what is currently rendered (Child's tree, or the fallback)
}

func (e *ErrorBoundaryNode) nodeMarker()    {}
func (e *ErrorBoundaryNode) String() string { return "ErrorBoundary" }
func (e *ErrorBoundaryNode) Apply(parent *ElementNode) {
	if e == nil {
		return
	}
	parent.Children = append(parent.Children, e)
}

// ComponentFrame tracks a component's hook state, lifcycle disposers, and
// position in the component tree. Created by PushComponent / destroyed by
// PopComponent, which live in internal/runtime.
type ComponentFrame struct {
	Path        string
	Parent      *ComponentFrame
	Children    []*ComponentFrame
	RootNodeIDs []int
	Hooks       []any
	Disposers   []func()
}

type ComponentNode struct {
	Name   string
	Key    any // used by keyed reconciliation (For); nil = unkeyed
	Render func() Node
	Prev   Node
	Frame  *ComponentFrame
}

// SetKey sets the reconciliation key on a node that carries one (elements and
// components). Other node types have no key and are left unchanged. Used by
// list helpers (For, VirtualList) so keyed matching preserves identity across
// reorders regardless of whether rows are elements or components.
func SetKey(n Node, key any) {
	switch v := n.(type) {
	case *ElementNode:
		if v != nil {
			v.Key = key
		}
	case *ComponentNode:
		if v != nil {
			v.Key = key
		}
	}
}

func (c *ComponentNode) nodeMarker() {}
func (c *ComponentNode) String() string {
	return fmt.Sprintf("Component(%s)", c.Name)
}

func Component(name string, render func() Node) *ComponentNode {
	return &ComponentNode{Name: name, Render: render}
}

type ScopeNode struct {
	Render func() Node
	Deps   []SignalAccessor
	Prev   Node
	Unsubs []func()
}

func (s *ScopeNode) nodeMarker() {}
func (s *ScopeNode) String() string {
	return fmt.Sprintf("Scope(%d deps)", len(s.Deps))
}

var VoidElements = map[string]bool{
	"br": true, "hr": true, "img": true, "input": true, "source": true,
	"track": true, "wbr": true, "area": true, "col": true, "embed": true,
}

// FlatTree collapses nested fragments into their parent's child list so the
// renderer never has to append a fragment (which has no single DOM node).
// ComponentNode and ScopeNode pass through untouched: they are reconciled
// lazily by the renderer (a component runs its setup once on mount and is
// then a stable, self-updating boundary; a scope re-renders on its deps).
func FlatTree(n Node) Node {
	switch v := n.(type) {
	case *ElementNode:
		v.Children = flattenChildren(v.Children)
		return v
	case *FragmentNode:
		v.Children = flattenChildren(v.Children)
		return v
	default:
		return n
	}
}

func flattenChildren(children []Node) []Node {
	var flat []Node
	for _, child := range children {
		f := FlatTree(child)
		if f == nil {
			continue
		}
		if frag, ok := f.(*FragmentNode); ok {
			flat = append(flat, frag.Children...)
		} else {
			flat = append(flat, f)
		}
	}
	return flat
}

type EventData struct {
	Type   string
	Target int
	Data   map[string]any
}

func (e EventData) str(k string) string {
	if v, ok := e.Data[k].(string); ok {
		return v
	}
	return ""
}

func (e EventData) num(k string) float64 {
	if v, ok := e.Data[k].(float64); ok {
		return v
	}
	return 0
}

func (e EventData) Value() string { return e.str("value") }
func (e EventData) Key() string   { return e.str("key") }
func (e EventData) Checked() bool {
	v, _ := e.Data["checked"].(bool)
	return v
}
func (e EventData) ScrollTop() float64  { return e.num("scrollTop") }
func (e EventData) ScrollLeft() float64 { return e.num("scrollLeft") }
func (e EventData) ClientX() float64    { return e.num("clientX") }
func (e EventData) ClientY() float64    { return e.num("clientY") }

func (e EventData) FormValues() map[string]string {
	out := map[string]string{}
	if m, ok := e.Data["values"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
