package ssr

import (
	"fmt"
	"strings"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/internal/walker"
)

type SlotRef struct {
	ComponentPath string
	HookIndex     int
}

type HydrationMeta struct {
	NodeMap map[int]string
	Deps    map[int][]SlotRef
}

type Renderer struct {
	walker.Walker
	Meta HydrationMeta
}

func New() *Renderer {
	return &Renderer{
		Walker: *walker.New(),
		Meta: HydrationMeta{
			NodeMap: make(map[int]string),
			Deps:    make(map[int][]SlotRef),
		},
	}
}

func (r *Renderer) Reset() {
	r.Walker.Reset()
	r.Meta = HydrationMeta{
		NodeMap: make(map[int]string),
		Deps:    make(map[int][]SlotRef),
	}
}

func (r *Renderer) Render(n core.Node) string {
	var out string
	runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
		r.Reset()
		var buf strings.Builder
		v := &ssrVisitor{r: r, buf: &buf, path: ""}
		r.Walker.Walk(n, v)
		out = buf.String()
	})
	return out
}

func (r *Renderer) RenderWithMeta(n core.Node, path string, hooks []core.SignalAccessor) (string, HydrationMeta) {
	var out string
	var meta HydrationMeta
	runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
		r.Reset()
		var buf strings.Builder
		v := &ssrVisitor{r: r, buf: &buf, path: path, hooks: hooks}
		r.Walker.Walk(n, v)
		out = buf.String()
		meta = r.Meta
	})
	return out, meta
}

// ssrVisitor implements walker.Visitor for the SSR renderer.
type ssrVisitor struct {
	r       *Renderer
	buf     *strings.Builder
	path    string
	hooks   []core.SignalAccessor
	hookIdx int
	silent  bool // when true, VisitElement/VisitText don't write to buffer
}

var _ walker.Visitor = (*ssrVisitor)(nil)

func (v *ssrVisitor) VisitElement(id int, el *core.ElementNode, walkChild func(core.Node) int) {
	if el == nil {
		return
	}
	v.r.Meta.NodeMap[id] = v.path

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
		if a.Name == "" || !isValidAttrName(a.Name) {
			core.Log(core.LogWarn, "skipping invalid attribute name", map[string]any{
				"name": a.Name,
			})
			continue
		}
		fmt.Fprintf(v.buf, ` %s="%s"`, a.Name, escapeAttr(a.Value))
	}

	for _, p := range el.Props {
		attrName, ok := propToAttr[p.Name]
		if !ok {
			continue
		}
		switch p.Value.(type) {
		case bool:
			if p.Value.(bool) {
				if attrName == "" {
					fmt.Fprintf(v.buf, ` %s`, p.Name)
				} else {
					fmt.Fprintf(v.buf, ` %s`, attrName)
				}
			}
		case string:
			val := p.Value.(string)
			if attrName == "" {
				attrName = p.Name
			}
			fmt.Fprintf(v.buf, ` %s="%s"`, attrName, escapeAttr(val))
		default:
			if attrName == "" {
				attrName = p.Name
			}
			fmt.Fprintf(v.buf, ` %s="%v"`, attrName, escapeAttr(fmt.Sprintf("%v", p.Value)))
		}
	}

	for _, b := range el.Binds {
		val := fmt.Sprintf("%v", b.Signal.Value())
		v.r.Meta.Deps[id] = append(v.r.Meta.Deps[id], SlotRef{
			ComponentPath: v.path,
			HookIndex:     v.hookIdx + len(v.r.Meta.Deps[id]),
		})
		if b.Target == core.BindToAttr {
			fmt.Fprintf(v.buf, ` %s="%s"`, b.Name, escapeAttr(val))
		} else {
			attrName, ok := propToAttr[b.Name]
			if !ok {
				continue
			}
			switch b.Signal.Value().(type) {
			case bool:
				if b.Signal.Value().(bool) {
					if attrName == "" {
						fmt.Fprintf(v.buf, ` %s`, b.Name)
					} else {
						fmt.Fprintf(v.buf, ` %s`, attrName)
					}
				}
			default:
				if attrName == "" {
					attrName = b.Name
				}
				fmt.Fprintf(v.buf, ` %s="%s"`, attrName, escapeAttr(val))
			}
		}
	}

	v.buf.WriteString(">")

	if core.VoidElements[el.Tag] {
		return
	}

	oldPath := v.path
	for i, child := range el.Children {
		v.path = oldPath + "/" + itoa(i)
		walkChild(child)
	}
	v.path = oldPath

	v.buf.WriteString("</")
	v.buf.WriteString(el.Tag)
	v.buf.WriteString(">")
}

func (v *ssrVisitor) VisitText(id int, tn *core.TextNode) {
	if tn == nil {
		return
	}
	v.r.Meta.NodeMap[id] = v.path
	if v.silent {
		return
	}
	fmt.Fprintf(v.buf, "<!--g%d-->", id)
	switch val := tn.Value.(type) {
	case string:
		v.buf.WriteString(escapeHTML(val))
	case core.SignalAccessor:
		v.r.Meta.Deps[id] = append(v.r.Meta.Deps[id], SlotRef{
			ComponentPath: v.path,
			HookIndex:     v.hookIdx,
		})
		v.buf.WriteString(escapeHTML(fmt.Sprintf("%v", val.Value())))
	}
}

func (v *ssrVisitor) VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int) {
	if fn == nil {
		return
	}
	oldPath := v.path
	for _, child := range fn.Children {
		v.path = oldPath + "/frag"
		walkChild(child)
	}
	v.path = oldPath
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

func (v *ssrVisitor) VisitErrorBoundary(ebn *core.ErrorBoundaryNode, walkInner func() int) int {
	if ebn == nil {
		return 0
	}
	// Transparent on the server: render the child so IDs match the client's
	// happy path. The boundary catches *client* render panics; server-side
	// rendering is expected not to panic (recover at the HTTP layer).
	return walkInner()
}

func (v *ssrVisitor) VisitComponentEnter(cn *core.ComponentNode) {}

func (v *ssrVisitor) VisitComponentLeave(cn *core.ComponentNode, innerID int) {}

func (v *ssrVisitor) VisitScopeEnter(sn *core.ScopeNode) {}

func (v *ssrVisitor) VisitScopeLeave(sn *core.ScopeNode, innerID int) {}

func isValidAttrName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if i == 0 && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

var propToAttr = map[string]string{
	"value":    "value",
	"checked":  "",
	"disabled": "",
	"selected": "",
	"multiple": "",
	"required": "",
	"readOnly": "readonly",
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
