package gooweetest

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/yogisalomo/goowee/core"
)

// node is one node of the in-memory DOM.
type node struct {
	id       int
	kind     nodeKind
	tag      string // elements
	text     string // text nodes
	attrs    []attr
	props    map[string]any
	parent   *node
	children []*node
	raw      string // innerHTML set as a property (h.Raw)
	calls    []Call // methods called through ref.Call
}

type nodeKind int

const (
	elementNode nodeKind = iota
	textNode
	commentNode
)

type attr struct{ name, value string }

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

func (n *node) setAttr(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].name == name {
			n.attrs[i].value = value
			return
		}
	}
	n.attrs = append(n.attrs, attr{name, value})
}

func (n *node) removeAttr(name string) {
	for i := range n.attrs {
		if n.attrs[i].name == name {
			n.attrs = append(n.attrs[:i], n.attrs[i+1:]...)
			return
		}
	}
}

func (n *node) detach() {
	if p := n.parent; p != nil {
		for i, c := range p.children {
			if c == n {
				p.children = append(p.children[:i], p.children[i+1:]...)
				break
			}
		}
		n.parent = nil
	}
}

func (n *node) insertBefore(child, ref *node) {
	child.detach()
	idx := len(n.children)
	if ref != nil {
		for i, c := range n.children {
			if c == ref {
				idx = i
				break
			}
		}
	}
	n.children = append(n.children, nil)
	copy(n.children[idx+1:], n.children[idx:])
	n.children[idx] = child
	child.parent = n
}

func (n *node) textContent() string {
	switch n.kind {
	case textNode:
		return n.text
	case commentNode:
		return ""
	}
	if n.raw != "" {
		return n.raw
	}
	var b strings.Builder
	for _, c := range n.children {
		b.WriteString(c.textContent())
	}
	return b.String()
}

func (n *node) setTextContent(s string) {
	if n.kind == textNode {
		n.text = s
		return
	}
	for _, c := range n.children {
		c.parent = nil
	}
	n.children = nil
	if s != "" {
		n.children = []*node{{kind: textNode, text: s, parent: n}}
	}
}

// document applies the renderer's mutation stream the way runtime/goowee.js
// does, so tests exercise the real render/diff/event pipeline.
type document struct {
	root   *node // the mount point (<div id="root">)
	head   *node
	body   *node
	nodes  map[int]*node
	portal map[string]*node // detached containers for unknown portal selectors
	active *node            // document.activeElement
	reads  []readReply
}

type readReply struct {
	req   int
	value any
}

func newDocument() *document {
	body := &node{kind: elementNode, tag: "body"}
	root := &node{kind: elementNode, tag: "div", attrs: []attr{{"id", "root"}}}
	body.insertBefore(root, nil)
	return &document{
		root:   root,
		head:   &node{kind: elementNode, tag: "head"},
		body:   body,
		nodes:  map[int]*node{},
		portal: map[string]*node{},
	}
}

func (d *document) byID(id int) *node {
	if id == 0 {
		return d.root
	}
	return d.nodes[id]
}

// forget drops a removed subtree from the id map (like goowee.js forget).
func (d *document) forget(n *node) {
	if n.id != 0 && d.nodes[n.id] == n {
		delete(d.nodes, n.id)
	}
	if d.active == n {
		d.active = nil
	}
	for _, c := range n.children {
		d.forget(c)
	}
}

func (d *document) container(selector string) *node {
	switch selector {
	case "head":
		return d.head
	case "body":
		return d.body
	}
	if strings.HasPrefix(selector, "#") {
		if n := findFirst(d.body, func(n *node) bool {
			v, ok := n.attr("id")
			return ok && v == selector[1:]
		}); n != nil {
			return n
		}
	}
	c := d.portal[selector]
	if c == nil {
		c = &node{kind: elementNode, tag: "div", attrs: []attr{{"data-portal", selector}}}
		d.portal[selector] = c
	}
	return c
}

func (d *document) apply(muts []core.Mutation) {
	// As in the browser, calls with arguments and then reads wait for the
	// batch's writes.
	var calls, reads []core.Mutation
	defer func() {
		for _, m := range calls {
			if n := d.nodes[m.NodeID]; n != nil {
				args, _ := m.Value.([]any)
				d.call(n, m.Key, args)
			}
		}
		for _, m := range reads {
			req, _ := m.Value.(int)
			d.reads = append(d.reads, readReply{req, d.readProp(d.nodes[m.NodeID], m.Key)})
		}
	}()
	for _, m := range muts {
		switch m.Type {
		case core.MutCreateElement, core.MutHydrate:
			if m.Type == core.MutHydrate && d.nodes[m.NodeID] != nil {
				continue
			}
			tag, _ := m.Value.(string)
			n := &node{id: m.NodeID, props: map[string]any{}}
			switch tag {
			case "#text":
				n.kind = textNode
			case "#comment":
				n.kind = commentNode
			default:
				n.kind, n.tag = elementNode, strings.ToLower(tag)
			}
			d.nodes[m.NodeID] = n
		case core.MutRemoveNode:
			if n := d.nodes[m.NodeID]; n != nil {
				n.detach()
				d.forget(n)
			}
		case core.MutSetAttribute:
			if n := d.nodes[m.NodeID]; n != nil {
				n.setAttr(m.Key, fmt.Sprint(m.Value))
			}
		case core.MutRemoveAttribute:
			if n := d.nodes[m.NodeID]; n != nil {
				n.removeAttr(m.Key)
			}
		case core.MutSetProperty:
			n := d.nodes[m.NodeID]
			if n == nil {
				continue
			}
			switch m.Key {
			case "textContent":
				n.setTextContent(fmt.Sprint(m.Value))
			case "innerHTML":
				n.raw = fmt.Sprint(m.Value)
			case "value":
				d.setValue(n, m.Value)
			default:
				n.props[m.Key] = m.Value
			}
		case core.MutAppendChild:
			p, c := d.byID(m.NodeID), d.nodes[m.ChildID]
			if p != nil && c != nil && c.parent == nil {
				p.insertBefore(c, nil)
			}
		case core.MutInsertBefore:
			c := d.nodes[m.ChildID]
			if c == nil {
				continue
			}
			var ref *node
			p := d.byID(m.NodeID)
			if m.RefID != 0 {
				if ref = d.nodes[m.RefID]; ref != nil && ref.parent != nil {
					p = ref.parent // insert at the reference's actual parent
				}
			}
			if p != nil {
				if ref != nil && ref.parent != p {
					ref = nil
				}
				p.insertBefore(c, ref)
			}
		case core.MutPortalAppend:
			if c := d.nodes[m.ChildID]; c != nil {
				sel, _ := m.Value.(string)
				if box := d.container(sel); c.parent != box {
					box.insertBefore(c, nil)
				}
			}
		case core.MutInvoke:
			if n := d.nodes[m.NodeID]; n != nil {
				switch m.Key {
				case "focus":
					d.active = n
				case "blur":
					if d.active == n {
						d.active = nil
					}
				}
			}
		case core.MutRead:
			reads = append(reads, m)
		case core.MutCall:
			calls = append(calls, m)
		}
	}
}

// setValue writes a value property the way the runtime does: a focused field
// keeps its caret (clamped to the new text), any other moves it to the end,
// as browsers do on a programmatic write.
func (d *document) setValue(n *node, v any) {
	n.props["value"] = v
	if _, ok := n.props["selectionStart"]; !ok {
		return
	}
	end := utf16Len(fmt.Sprint(v))
	if d.active == n {
		start, stop := propNum(n, "selectionStart"), propNum(n, "selectionEnd")
		n.props["selectionStart"], n.props["selectionEnd"] = min(start, end), min(stop, end)
		return
	}
	n.props["selectionStart"], n.props["selectionEnd"] = end, end
}

// call applies a ref.Call: it is recorded for Element.Calls, and the methods
// whose effect a test can observe are modelled (focus, blur, select and
// setSelectionRange, whose offsets are clamped to the value like a
// browser's).
func (d *document) call(n *node, method string, args []any) {
	n.calls = append(n.calls, Call{Method: method, Args: args})
	length := utf16Len(fmt.Sprint(n.props["value"]))
	switch method {
	case "focus":
		d.active = n
	case "blur":
		if d.active == n {
			d.active = nil
		}
	case "select":
		n.props["selectionStart"], n.props["selectionEnd"] = 0.0, length
	case "setSelectionRange":
		start, end := min(argNum(args, 0), length), min(argNum(args, 1), length)
		n.props["selectionStart"], n.props["selectionEnd"] = min(start, end), end
	}
}

func utf16Len(s string) float64 { return float64(len(utf16.Encode([]rune(s)))) }

func propNum(n *node, k string) float64 { v, _ := n.props[k].(float64); return v }

// argNum reads a numeric ref.Call argument (a Go int or float, not yet JSON).
func argNum(args []any, i int) float64 {
	if i >= len(args) {
		return 0
	}
	switch v := args[i].(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

// readProp answers a ref.Get from what the fake DOM knows; layout values
// (sizes, positions) are unknown here and read as nil.
func (d *document) readProp(n *node, key string) any {
	if n == nil {
		return nil
	}
	switch key {
	case "textContent", "innerText":
		return n.textContent()
	case "tagName":
		return strings.ToUpper(n.tag)
	}
	if v, ok := n.props[key]; ok {
		return v
	}
	if v, ok := n.attr(key); ok {
		return v
	}
	return nil
}

func findFirst(n *node, match func(*node) bool) *node {
	if n.kind == elementNode && match(n) {
		return n
	}
	for _, c := range n.children {
		if f := findFirst(c, match); f != nil {
			return f
		}
	}
	return nil
}

// outerHTML serializes n (anchors and other comments are skipped).
func outerHTML(n *node, b *strings.Builder) {
	switch n.kind {
	case textNode:
		b.WriteString(html.EscapeString(n.text))
		return
	case commentNode:
		return
	}
	b.WriteString("<" + n.tag)
	attrs := append([]attr(nil), n.attrs...)
	sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].name < attrs[j].name })
	for _, a := range attrs {
		fmt.Fprintf(b, ` %s="%s"`, a.name, html.EscapeString(a.value))
	}
	b.WriteString(">")
	if voidTag[n.tag] {
		return
	}
	if n.raw != "" {
		b.WriteString(n.raw)
	}
	for _, c := range n.children {
		outerHTML(c, b)
	}
	b.WriteString("</" + n.tag + ">")
}

var voidTag = core.VoidElements
