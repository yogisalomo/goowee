package ssr

import (
	"fmt"
	"strings"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/internal/walker"
)

// Renderer renders a node tree to HTML. Use one Renderer per render (they are
// cheap); separate renders may run concurrently (ADR-024).
type Renderer struct {
	walker.Walker
	headBuf strings.Builder
}

func New() *Renderer {
	return &Renderer{Walker: *walker.New()}
}

func (r *Renderer) Reset() {
	r.Walker.Reset()
	r.headBuf.Reset()
}

// Render returns the body HTML of n and the <head> HTML its Metadata produced.
// Effects don't run on the server, and no lock is held: concurrent requests
// render in parallel.
func (r *Renderer) Render(n core.Node) (body, head string) {
	runtime.ServerRender(func() {
		r.Reset()
		var buf strings.Builder
		v := &ssrVisitor{r: r, buf: &buf}
		r.Walker.Walk(n, v)
		body = buf.String()
		head = r.headBuf.String()
	})
	return
}

// ssrVisitor implements walker.Visitor for the SSR renderer.
type ssrVisitor struct {
	r      *Renderer
	buf    *strings.Builder
	silent bool // when true, VisitElement/VisitText don't write to buffer
}

var _ walker.Visitor = (*ssrVisitor)(nil)

// writeAttr writes one attribute, skipping invalid names and blocking script
// URLs (shared with the DOM renderer, so both sides agree).
func (v *ssrVisitor) writeAttr(name, value string) {
	if !runtime.ValidAttrName(name) {
		runtime.WarnInvalidAttr(name)
		return
	}
	v.buf.WriteByte(' ')
	v.buf.WriteString(name)
	v.buf.WriteString(`="`)
	v.buf.WriteString(escapeAttr(runtime.SafeURL(name, value)))
	v.buf.WriteByte('"')
}

func (v *ssrVisitor) writeBoolAttr(name string) {
	v.buf.WriteByte(' ')
	v.buf.WriteString(name)
}

// writeProp renders a property that has an attribute form (value, checked,
// disabled, …); other properties exist only on the live DOM node.
func (v *ssrVisitor) writeProp(name string, value any) {
	attrName, ok := propToAttr[name]
	if !ok {
		return
	}
	if attrName == "" {
		attrName = name
	}
	switch val := value.(type) {
	case bool:
		if val {
			v.writeBoolAttr(attrName)
		}
	case string:
		v.writeAttr(attrName, val)
	default:
		v.writeAttr(attrName, fmt.Sprint(val))
	}
}

func (v *ssrVisitor) VisitElement(id int, el *core.ElementNode, walkChild func(core.Node) int) {
	if el == nil {
		return
	}
	if v.silent {
		for _, child := range el.Children {
			walkChild(child)
		}
		return
	}

	v.buf.WriteString("<")
	v.buf.WriteString(el.Tag)
	fmt.Fprintf(v.buf, ` data-node-id="%d"`, id)

	for _, a := range el.Attrs {
		v.writeAttr(a.Name, a.Value)
	}
	for _, p := range el.Props {
		v.writeProp(p.Name, p.Value)
	}
	for _, b := range el.Binds {
		if b.Target == core.BindToAttr {
			v.writeAttr(b.Name, fmt.Sprint(b.Signal.Value()))
		} else {
			v.writeProp(b.Name, b.Signal.Value())
		}
	}

	v.buf.WriteString(">")

	if core.VoidElements[el.Tag] {
		return
	}

	// A textContent property (static or bound) replaces the element's
	// children on the client; render it as the content here too.
	if text, ok := textContentOf(el); ok {
		v.buf.WriteString(escapeHTML(text))
		for _, child := range el.Children {
			v.silentWalk(walkChild, child)
		}
	} else {
		for _, child := range el.Children {
			walkChild(child)
		}
	}

	v.buf.WriteString("</")
	v.buf.WriteString(el.Tag)
	v.buf.WriteString(">")
}

func textContentOf(el *core.ElementNode) (string, bool) {
	for _, b := range el.Binds {
		if b.Target == core.BindToProp && b.Name == "textContent" {
			return runtime.TextValue(b.Signal.Value()), true
		}
	}
	for _, p := range el.Props {
		if p.Name == "textContent" {
			return runtime.TextValue(p.Value), true
		}
	}
	return "", false
}

// silentWalk walks a child for id parity without writing it.
func (v *ssrVisitor) silentWalk(walkChild func(core.Node) int, child core.Node) {
	old := v.silent
	v.silent = true
	walkChild(child)
	v.silent = old
}

func (v *ssrVisitor) VisitText(id int, tn *core.TextNode) {
	if tn == nil || v.silent {
		return
	}
	fmt.Fprintf(v.buf, "<!--g%d-->", id)
	switch val := tn.Value.(type) {
	case string:
		v.buf.WriteString(escapeHTML(val))
	case core.SignalAccessor:
		v.buf.WriteString(escapeHTML(runtime.TextValue(val.Value())))
	}
}

func (v *ssrVisitor) VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int) {
	if fn == nil {
		return
	}
	for _, child := range fn.Children {
		walkChild(child)
	}
}

func (v *ssrVisitor) VisitPortal(pn *core.PortalNode, walkChild func(core.Node) int) {
	if pn == nil {
		return
	}
	// Portals are client-side: walk children for ID parity but suppress HTML
	// output so the server-rendered markup stays in the main tree.
	oldSilent := v.silent
	v.silent = true
	for _, child := range pn.Children {
		walkChild(child)
	}
	v.silent = oldSilent
}

func (v *ssrVisitor) VisitMetadata(mn *core.MetadataNode, walkChild func(core.Node) int) {
	if mn == nil {
		return
	}
	// Walk children silently for ID parity with the client renderer.
	oldSilent := v.silent
	v.silent = true
	for _, child := range mn.Children {
		walkChild(child)
	}
	// Render head children into the head buffer.
	oldBuf := v.buf
	v.buf = &v.r.headBuf
	v.silent = false
	v.renderHeadChildren(mn.Children)
	v.buf = oldBuf
	v.silent = oldSilent
}

func (v *ssrVisitor) renderHeadChildren(children []core.Node) {
	for _, child := range children {
		switch n := child.(type) {
		case *core.ElementNode:
			v.renderHeadElement(n)
		case *core.TextNode:
			v.buf.WriteString(escapeHTML(runtime.TextValue(textOf(n))))
		case *core.FragmentNode:
			// Flatten: metadata inside fragments (e.g. conditional groups).
			v.renderHeadChildren(n.Children)
		}
	}
}

func textOf(n *core.TextNode) any {
	if s, ok := n.Value.(core.SignalAccessor); ok {
		return s.Value()
	}
	return n.Value
}

func (v *ssrVisitor) renderHeadElement(el *core.ElementNode) {
	v.buf.WriteString("<")
	v.buf.WriteString(el.Tag)
	for _, a := range el.Attrs {
		v.writeAttr(a.Name, a.Value)
	}
	if core.VoidElements[el.Tag] {
		v.buf.WriteString(">")
		return
	}
	v.buf.WriteString(">")
	v.renderHeadChildren(el.Children)
	v.buf.WriteString("</")
	v.buf.WriteString(el.Tag)
	v.buf.WriteString(">")
}

func (v *ssrVisitor) VisitErrorBoundary(ebn *core.ErrorBoundaryNode, walkInner func() int) int {
	if ebn == nil {
		return 0
	}
	// Transparent on the server: render the child so IDs match the client's
	// happy path. The boundary catches *client* render panics; server-side
	// rendering is expected not to panic (ssr.Handler recovers per request).
	return walkInner()
}

func (v *ssrVisitor) VisitComponentEnter(cn *core.ComponentNode) {}

func (v *ssrVisitor) VisitComponentLeave(cn *core.ComponentNode, innerID int) {}

func (v *ssrVisitor) VisitScopeEnter(sn *core.ScopeNode) {}

// VisitScopeLeave writes the scope's end anchor as a <!--/{id}--> comment; the
// client claims it on hydration and inserts re-rendered content before it.
func (v *ssrVisitor) VisitScopeLeave(sn *core.ScopeNode, innerID int) int {
	if !v.silent {
		fmt.Fprintf(v.buf, "<!--/%d-->", sn.Anchor)
	}
	return innerID
}

func (v *ssrVisitor) VisitRaw(id int, rn *core.RawNode) {
	if rn == nil || v.silent {
		return
	}
	fmt.Fprintf(v.buf, `<div data-node-id="%d">%s</div>`, id, rn.HTML)
}

var propToAttr = map[string]string{
	"value":    "value",
	"checked":  "",
	"disabled": "",
	"selected": "",
	"multiple": "",
	"required": "",
	"readOnly": "readonly",
	"href":     "href",
	"src":      "src",
}

func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
