package ssr

import (
	"fmt"
	"goowee/core"
	"log"
	"strings"
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
	nextID int
	Meta   HydrationMeta
}

func New() *Renderer {
	return &Renderer{
		nextID: 1,
		Meta: HydrationMeta{
			NodeMap: make(map[int]string),
			Deps:    make(map[int][]SlotRef),
		},
	}
}

func (r *Renderer) Reset() {
	r.nextID = 1
	r.Meta = HydrationMeta{
		NodeMap: make(map[int]string),
		Deps:    make(map[int][]SlotRef),
	}
}

func (r *Renderer) allocID() int {
	id := r.nextID
	r.nextID++
	return id
}

func (r *Renderer) Render(n core.Node) string {
	var out string
	// Server render context: per-request frame stack (no shared global) and
	// EnvServer so component effects/OnMount don't run or leak goroutines.
	core.UseContext(core.NewRenderContext(core.EnvServer), func() {
		r.Reset()
		var buf strings.Builder
		r.renderNode(n, &buf, "")
		out = buf.String()
	})
	return out
}

func (r *Renderer) RenderWithMeta(n core.Node, path string, hooks []core.SignalAccessor) (string, HydrationMeta) {
	var out string
	var meta HydrationMeta
	core.UseContext(core.NewRenderContext(core.EnvServer), func() {
		r.Reset()
		var buf strings.Builder
		r.renderNodeWithMeta(n, &buf, path, hooks, 0)
		out = buf.String()
		meta = r.Meta
	})
	return out, meta
}

func (r *Renderer) renderNode(n core.Node, buf *strings.Builder, path string) {
	r.renderNodeWithMeta(n, buf, path, nil, 0)
}

func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
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

func (r *Renderer) renderNodeWithMeta(n core.Node, buf *strings.Builder, path string, hooks []core.SignalAccessor, hookIdx int) {
	switch v := n.(type) {
	case *core.ElementNode:
		if v == nil {
			return
		}
		id := r.allocID()
		r.Meta.NodeMap[id] = path

		buf.WriteString("<")
		buf.WriteString(v.Tag)
		fmt.Fprintf(buf, ` data-node-id="%d"`, id)

		for _, a := range v.Attrs {
			if a.Name == "" || !isValidAttrName(a.Name) {
				log.Printf("goowee: skipping invalid attribute name %q", a.Name)
				continue
			}
			fmt.Fprintf(buf, ` %s="%s"`, a.Name, escapeAttr(a.Value))
		}

		for _, p := range v.Props {
			attrName, ok := propToAttr[p.Name]
			if !ok {
				continue
			}
			switch p.Value.(type) {
			case bool:
				if p.Value.(bool) {
					if attrName == "" {
						fmt.Fprintf(buf, ` %s`, p.Name)
					} else {
						fmt.Fprintf(buf, ` %s`, attrName)
					}
				}
			case string:
				val := p.Value.(string)
				if attrName == "" {
					attrName = p.Name
				}
				fmt.Fprintf(buf, ` %s="%s"`, attrName, escapeAttr(val))
			default:
				if attrName == "" {
					attrName = p.Name
				}
				fmt.Fprintf(buf, ` %s="%v"`, attrName, escapeAttr(fmt.Sprintf("%v", p.Value)))
			}
		}

		for _, b := range v.Binds {
			val := fmt.Sprintf("%v", b.Signal.Value())
			r.Meta.Deps[id] = append(r.Meta.Deps[id], SlotRef{
				ComponentPath: path,
				HookIndex:     hookIdx + len(r.Meta.Deps[id]),
			})
			if b.Target == core.BindToAttr {
				fmt.Fprintf(buf, ` %s="%s"`, b.Name, escapeAttr(val))
			} else {
				attrName, ok := propToAttr[b.Name]
				if !ok {
					continue
				}
				switch b.Signal.Value().(type) {
				case bool:
					if b.Signal.Value().(bool) {
						if attrName == "" {
							fmt.Fprintf(buf, ` %s`, b.Name)
						} else {
							fmt.Fprintf(buf, ` %s`, attrName)
						}
					}
				default:
					if attrName == "" {
						attrName = b.Name
					}
					fmt.Fprintf(buf, ` %s="%s"`, attrName, escapeAttr(val))
				}
			}
		}

		buf.WriteString(">")

		if core.VoidElements[v.Tag] {
			return
		}

		for i, child := range v.Children {
			r.renderNodeWithMeta(child, buf, path+"/"+itoa(i), hooks, hookIdx)
		}

		buf.WriteString("</")
		buf.WriteString(v.Tag)
		buf.WriteString(">")

	case *core.TextNode:
		if v == nil {
			return
		}
		id := r.allocID()
		r.Meta.NodeMap[id] = path
		switch val := v.Value.(type) {
		case string:
			buf.WriteString(escapeHTML(val))
		case core.SignalAccessor:
			r.Meta.Deps[id] = append(r.Meta.Deps[id], SlotRef{
				ComponentPath: path,
				HookIndex:     hookIdx,
			})
			buf.WriteString(escapeHTML(fmt.Sprintf("%v", val.Value())))
		}

	case *core.FragmentNode:
		if v == nil {
			return
		}
		for _, child := range v.Children {
			r.renderNodeWithMeta(child, buf, path+"/frag", hooks, hookIdx)
		}

	case *core.ComponentNode:
		if v == nil {
			return
		}
		core.PushComponent()
		inner := v.Render()
		r.renderNodeWithMeta(inner, buf, path, hooks, hookIdx)
		core.PopComponent()

	case *core.ScopeNode:
		if v == nil {
			return
		}
		inner := v.Render()
		r.renderNodeWithMeta(inner, buf, path, hooks, hookIdx)
	}
}

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
