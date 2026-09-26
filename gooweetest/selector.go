package gooweetest

import (
	"fmt"
	"strings"
)

// A selector is a small subset of CSS: compound selectors — tag, #id, .class,
// [attr], [attr=value] (value optionally quoted) — joined by whitespace
// (descendant combinator). E.g. "form button.primary", "li[data-id=3]",
// "#main .item".
type compound struct {
	tag     string
	id      string
	classes []string
	attrs   []attrSel
}

type attrSel struct {
	name, value string
	hasValue    bool
}

func parseSelector(sel string) ([]compound, error) {
	var out []compound
	for _, part := range strings.Fields(sel) {
		c, err := parseCompound(part)
		if err != nil {
			return nil, fmt.Errorf("selector %q: %w", sel, err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty selector")
	}
	return out, nil
}

func parseCompound(s string) (compound, error) {
	var c compound
	i := 0
	readName := func() string {
		start := i
		for i < len(s) && !strings.ContainsRune("#.[", rune(s[i])) {
			i++
		}
		return s[start:i]
	}
	if i < len(s) && !strings.ContainsRune("#.[", rune(s[0])) {
		c.tag = strings.ToLower(readName())
		if c.tag == "*" {
			c.tag = ""
		}
	}
	for i < len(s) {
		switch s[i] {
		case '#':
			i++
			c.id = readName()
		case '.':
			i++
			c.classes = append(c.classes, readName())
		case '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return c, fmt.Errorf("unclosed [")
			}
			body := s[i+1 : i+end]
			i += end + 1
			name, value, has := strings.Cut(body, "=")
			value = strings.Trim(value, `"'`)
			c.attrs = append(c.attrs, attrSel{strings.TrimSpace(name), value, has})
		default:
			return c, fmt.Errorf("unexpected %q", s[i])
		}
	}
	return c, nil
}

func (c compound) matches(n *node) bool {
	if n.kind != elementNode || (c.tag != "" && n.tag != c.tag) {
		return false
	}
	if c.id != "" {
		if v, _ := n.attr("id"); v != c.id {
			return false
		}
	}
	if len(c.classes) > 0 {
		have := strings.Fields(func() string { v, _ := n.attr("class"); return v }())
		for _, want := range c.classes {
			found := false
			for _, h := range have {
				found = found || h == want
			}
			if !found {
				return false
			}
		}
	}
	for _, a := range c.attrs {
		v, ok := n.attr(a.name)
		if !ok || (a.hasValue && v != a.value) {
			return false
		}
	}
	return true
}

// selectAll returns the elements under root (document order) matching sel.
func selectAll(root *node, sel []compound) []*node {
	var out []*node
	var walk func(n *node)
	walk = func(n *node) {
		if n != root && matchesChain(n, sel) {
			out = append(out, n)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// matchesChain: the last compound matches n, and the earlier ones match
// ancestors, in order.
func matchesChain(n *node, sel []compound) bool {
	last := len(sel) - 1
	if !sel[last].matches(n) {
		return false
	}
	i := last - 1
	for p := n.parent; p != nil && i >= 0; p = p.parent {
		if sel[i].matches(p) {
			i--
		}
	}
	return i < 0
}
