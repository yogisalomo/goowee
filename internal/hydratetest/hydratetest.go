// Package hydratetest checks, without a browser, that a client render's
// Hydrate mutations claim the HTML the server rendered for the same tree. It
// parses the server's markup and claims nodes in document order exactly as
// claimNext in runtime/goowee.js does (ADR-033), so a test can render a tree
// both ways and ask whether hydration would match.
//
// The parser is for goowee's own output: well-formed tags, void elements,
// comments and text, and raw-text elements (textarea, title, script, style).
// It doesn't apply the HTML parser's tree repair (a <div> inside a <p>);
// the browser E2E covers that.
package hydratetest

import (
	"fmt"
	"strings"

	"github.com/yogisalomo/goowee/core"
)

// Kind is a parsed node's type.
type Kind int

const (
	Element Kind = iota
	Text
	Comment
)

// Node is one node of the parsed server markup.
type Node struct {
	Kind     Kind
	Tag      string // Element: the lower-case tag; Text and Comment: the data
	Children []*Node
	parent   *Node
}

func (n *Node) String() string {
	switch n.Kind {
	case Text:
		return "#text"
	case Comment:
		return "#comment"
	}
	return n.Tag
}

var voidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

var rawTextTags = map[string]bool{"textarea": true, "title": true, "script": true, "style": true, "xmp": true}

// Parse parses html into a container node holding its top-level nodes.
func Parse(html string) *Node {
	root := &Node{Kind: Element, Tag: "#root"}
	cur := root
	add := func(n *Node) {
		n.parent = cur
		cur.Children = append(cur.Children, n)
	}
	for i := 0; i < len(html); {
		switch {
		case strings.HasPrefix(html[i:], "<!--"):
			end := strings.Index(html[i+4:], "-->")
			if end < 0 {
				end = len(html) - i - 4
			}
			add(&Node{Kind: Comment, Tag: html[i+4 : i+4+end]})
			i += 4 + end + 3
		case strings.HasPrefix(html[i:], "</"):
			end := strings.IndexByte(html[i:], '>')
			tag := strings.ToLower(strings.TrimSpace(html[i+2 : i+end]))
			for n := cur; n != root; n = n.parent {
				if n.Tag == tag {
					cur = n.parent
					break
				}
			}
			i += end + 1
		case html[i] == '<' && i+1 < len(html) && isLetter(html[i+1]):
			end := tagEnd(html, i)
			inner := html[i+1 : end]
			name := inner
			if j := strings.IndexAny(inner, " \t\n/"); j >= 0 {
				name = inner[:j]
			}
			name = strings.ToLower(name)
			el := &Node{Kind: Element, Tag: name}
			add(el)
			i = end + 1
			if voidTags[name] {
				continue
			}
			if rawTextTags[name] {
				close := strings.Index(strings.ToLower(html[i:]), "</"+name)
				if close < 0 {
					close = len(html) - i
				}
				text := html[i : i+close]
				if name == "textarea" {
					text = strings.TrimPrefix(text, "\n")
				}
				if text != "" {
					el.Children = append(el.Children, &Node{Kind: Text, Tag: text, parent: el})
				}
				i += close
				if gt := strings.IndexByte(html[i:], '>'); gt >= 0 {
					i += gt + 1
				}
				continue
			}
			cur = el
			if name == "pre" || name == "listing" {
				i += len(leadingNewline(html[i:]))
			}
		default:
			end := strings.IndexByte(html[i+1:], '<')
			if end < 0 {
				end = len(html) - i - 1
			}
			text := html[i : i+1+end]
			// Adjacent text merges into one node, as in the browser.
			if k := len(cur.Children); k > 0 && cur.Children[k-1].Kind == Text {
				cur.Children[k-1].Tag += text
			} else {
				add(&Node{Kind: Text, Tag: text})
			}
			i += 1 + end
		}
	}
	return root
}

func leadingNewline(s string) string {
	if strings.HasPrefix(s, "\n") {
		return "\n"
	}
	return ""
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

// tagEnd is the index of the '>' that ends the start tag at i, skipping
// quoted attribute values.
func tagEnd(html string, i int) int {
	var quote byte
	for j := i + 1; j < len(html); j++ {
		c := html[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j
		}
	}
	return len(html) - 1
}

// Claim runs muts' Hydrate mutations against the server markup html, in
// order, and returns what each node id claimed and every mismatch, worded as
// the runtime words them. Other mutation types are ignored.
func Claim(html string, muts []core.Mutation) (claimed map[int]*Node, mismatches []string) {
	root := Parse(html)
	claimed = map[int]*Node{}
	last, skip := root, false
	for _, m := range muts {
		if m.Type != core.MutHydrate {
			continue
		}
		want, _ := m.Val().(string)
		wantText, wantComment := want == "#text", want == "#comment"
		n := next(last, root, skip)
		for n != nil {
			after := next(n, root, true)
			if n.Kind == Comment && (n.Tag == "goowee" || n.Tag == "") {
				remove(n)
				n = after
				continue
			}
			if n.Kind == Comment && n.Tag == "t" && wantText {
				n.Kind, n.Tag = Text, ""
				break
			}
			if n.Kind == Text && !wantText && strings.TrimSpace(n.Tag) == "" {
				n = after
				continue
			}
			break
		}
		matches := n != nil && (wantText && n.Kind == Text ||
			wantComment && n.Kind == Comment && strings.HasPrefix(n.Tag, "/") ||
			!wantText && !wantComment && n.Kind == Element && n.Tag == want)
		if n == nil {
			mismatches = append(mismatches, fmt.Sprintf("no server node for %d (<%s>)", m.NodeID, want))
			continue
		}
		last, skip = n, m.Key == "opaque" || !matches
		if !matches {
			mismatches = append(mismatches, fmt.Sprintf("hydration mismatch at node %d: client expected <%s>, server rendered <%s>", m.NodeID, want, n))
			continue
		}
		claimed[m.NodeID] = n
	}
	return claimed, mismatches
}

// next is the node after n in document order within root, without entering
// n's children when skip is set.
func next(n, root *Node, skip bool) *Node {
	if !skip && len(n.Children) > 0 {
		return n.Children[0]
	}
	for n != nil && n != root {
		p := n.parent
		for i, c := range p.Children {
			if c == n && i+1 < len(p.Children) {
				return p.Children[i+1]
			}
		}
		n = p
	}
	return nil
}

func remove(n *Node) {
	p := n.parent
	for i, c := range p.Children {
		if c == n {
			p.Children = append(p.Children[:i], p.Children[i+1:]...)
			return
		}
	}
}
