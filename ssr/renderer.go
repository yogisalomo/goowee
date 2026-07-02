package ssr

import (
	"fmt"
	"goowee/core"
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
	r.Reset()
	var buf strings.Builder
	r.renderNode(n, &buf, "")
	return buf.String()
}

func (r *Renderer) RenderWithMeta(n core.Node, path string, hooks []core.SignalAccessor) (string, HydrationMeta) {
	r.Reset()
	var buf strings.Builder
	r.renderNodeWithMeta(n, &buf, path, hooks, 0)
	return buf.String(), r.Meta
}

func (r *Renderer) renderNode(n core.Node, buf *strings.Builder, path string) {
	r.renderNodeWithMeta(n, buf, path, nil, 0)
}

func (r *Renderer) renderNodeWithMeta(n core.Node, buf *strings.Builder, path string, hooks []core.SignalAccessor, hookIdx int) {
	switch v := n.(type) {
	case *core.ElementNode:
		id := r.allocID()
		r.Meta.NodeMap[id] = path

		buf.WriteString("<")
		buf.WriteString(v.Tag)
		fmt.Fprintf(buf, ` data-node-id="%d"`, id)

		var sigIdx int
		for key, val := range v.Props {
			switch actual := val.(type) {
			case string:
				fmt.Fprintf(buf, ` %s="%s"`, key, actual)
			case core.SignalAccessor:
				r.Meta.Deps[id] = append(r.Meta.Deps[id], SlotRef{
					ComponentPath: path,
					HookIndex:     hookIdx + sigIdx,
				})
				sigIdx++
				v := actual.Value()
				if b, ok := v.(bool); ok && isBoolAttr(key) {
					if b {
						fmt.Fprintf(buf, ` %s`, key)
					}
				} else {
					fmt.Fprintf(buf, ` %s="%v"`, key, v)
				}
			}
		}
		buf.WriteString(">")

		for _, child := range v.Children {
			r.renderNodeWithMeta(child, buf, path+"/"+itoa(len(v.Children)), hooks, hookIdx)
		}

		buf.WriteString("</")
		buf.WriteString(v.Tag)
		buf.WriteString(">")

	case *core.TextNode:
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
		for _, child := range v.Children {
			r.renderNodeWithMeta(child, buf, path+"/frag", hooks, hookIdx)
		}

	case *core.ComponentNode:
		core.PushComponent()
		inner := v.Render()
		r.renderNodeWithMeta(inner, buf, path, hooks, hookIdx)
		core.PopComponent()

	case *core.ScopeNode:
		inner := v.Render()
		r.renderNodeWithMeta(inner, buf, path, hooks, hookIdx)
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func isBoolAttr(key string) bool {
	switch key {
	case "checked", "disabled", "selected", "readonly", "required", "multiple":
		return true
	}
	return false
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
