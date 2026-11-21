package gooweetest

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// FuzzDiffer (#161): after any sequence of updates, the document equals a
// fresh render of the final state. The input describes a tree built from the
// node kinds the differ handles (elements with static and bound attributes,
// SVG, text, fragments, Show, Switch, keyed For, components, portals),
// driven by a few signals, then a sequence of signal writes and list edits
// (insert, remove, move, change, duplicate keys). After each flush:
//
//   - the document (attributes, properties, text and portal contents) equals
//     a fresh render of the same state;
//   - the handler and binding registries are the same size as the fresh
//     render's: removed nodes left nothing behind;
//   - as many components are mounted as in the fresh render, and none was
//     cleaned up twice;
//   - no mutation named a node the document doesn't have.
//
// go test -fuzz FuzzDiffer ./gooweetest/ runs it; CI runs a short budget.
func FuzzDiffer(f *testing.F) {
	for _, seed := range differSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		g := &fuzzInput{data: input}
		spec := genSpec(g, 0)
		var steps []fuzzStep
		for len(steps) < 12 && !g.done() {
			steps = append(steps, genStep(g))
		}

		st := newFuzzState()
		s := Render(t, fuzzRender(spec, st))
		check := func(when string) {
			t.Helper()
			fresh := newFuzzState()
			fresh.copyValues(st)
			fs := Render(t, fuzzRender(spec, fresh))
			if got, want := canonical(s), canonical(fs); got != want {
				t.Fatalf("%s: the document differs from a fresh render\nspec: %s\nsteps: %v\n got: %s\nwant: %s", when, spec, steps, got, want)
			}
			if got, want := s.r.Registry.Len(), fs.r.Registry.Len(); got != want {
				t.Fatalf("%s: %d nodes have handlers, a fresh render has %d\nspec: %s\nsteps: %v", when, got, want, spec, steps)
			}
			if got, want := s.r.Bindings.Len(), fs.r.Bindings.Len(); got != want {
				t.Fatalf("%s: %d live bindings, a fresh render has %d\nspec: %s\nsteps: %v", when, got, want, spec, steps)
			}
			if got, want := st.live(), fresh.live(); got != want {
				t.Fatalf("%s: %d components mounted, a fresh render has %d\nspec: %s\nsteps: %v", when, got, want, spec, steps)
			}
			if st.doubleCleanups > 0 {
				t.Fatalf("%s: a component was cleaned up twice\nspec: %s\nsteps: %v", when, spec, steps)
			}
			if s.doc.unknown > 0 {
				t.Fatalf("%s: %d mutations named unknown nodes\nspec: %s\nsteps: %v", when, s.doc.unknown, spec, steps)
			}
			fs.Unmount()
		}
		check("first render")
		for i, step := range steps {
			step.apply(st)
			s.Flush()
			check(fmt.Sprintf("after step %d (%v)", i+1, step))
		}
	})
}

// differSeeds are regression shapes (#59: content placed before a scope's
// anchor after a parent re-render; #83: keyed rows moved across a sibling
// scope) and a mix of every kind.
var differSeeds = [][]byte{
	{kFor, 0, kText, 1, 0, opList, 0, 5, opList, 1, 0, opList, 2, 0, 2},
	{kElem, 0, 2, kShow, 0, kFor, 0, kText, 1, 0, kText, 0, 1, opBool, 0, opList, 0, 7, opBool, 0, opList, 2, 1, 0},
	{kFrag, 3, kShow, 1, kText, 1, 1, kFor, 0, kElem, 2, 1, kText, 1, 2, kSwitch, 0, kText, 0, 0, kText, 1, 0, opInt, 0, 1, opList, 0, 3, opInt, 0, 2, opBool, 1, opList, 3, 0, 9},
	{kComp, 1, kElem, 3, 2, kText, 1, 1, kPortal, kText, 1, 2, opInt, 1, 4, opInt, 2, 3},
	{kElem, 5, 2, kText, 0, 0, kShow, 2, kElem, 4, 1, kText, 1, 0, opBool, 2, opInt, 0, 1, opBool, 2},
	{kFor, 0, kComp, 0, kShow, 1, kText, 1, 1, opList, 0, 1, opList, 0, 2, opList, 4, 0, opBool, 1, opList, 1, 1, opList, 2, 1, 0},
}

// Node kinds and step kinds, the first byte of each in the input.
const (
	kText = iota
	kElem
	kFrag
	kShow
	kSwitch
	kFor
	kComp
	kPortal
	numKinds
)

const (
	opInt = iota
	opBool
	opList
	numOps
)

type fuzzInput struct {
	data []byte
	i    int
}

func (g *fuzzInput) done() bool { return g.i >= len(g.data) }

// n returns the next input byte mod max (0 once the input is used up).
func (g *fuzzInput) n(max int) int {
	if g.i >= len(g.data) {
		return 0
	}
	b := int(g.data[g.i])
	g.i++
	return b % max
}

// spec describes a node; fuzzRender turns it into one for a given state.
type spec struct {
	kind     int
	a, b     int // kind-specific: signal index, tag, attribute mode
	children []*spec
}

func (s *spec) String() string {
	if s == nil {
		return "nil"
	}
	var parts []string
	for _, c := range s.children {
		parts = append(parts, c.String())
	}
	return fmt.Sprintf("%s(%d,%d)[%s]", [...]string{"text", "elem", "frag", "show", "switch", "for", "comp", "portal"}[s.kind], s.a, s.b, strings.Join(parts, " "))
}

func genSpec(g *fuzzInput, depth int) *spec {
	kind := g.n(numKinds)
	if depth >= 4 {
		kind = kText
	}
	sp := &spec{kind: kind}
	switch kind {
	case kText:
		sp.a, sp.b = g.n(2), g.n(3) // static or bound; which int
	case kElem:
		sp.a, sp.b = g.n(6), g.n(3) // tag; attribute mode
		for i, n := 0, g.n(3); i < n; i++ {
			sp.children = append(sp.children, genSpec(g, depth+1))
		}
	case kFrag:
		for i, n := 0, 1+g.n(3); i < n; i++ {
			sp.children = append(sp.children, genSpec(g, depth+1))
		}
	case kShow, kComp:
		sp.a = g.n(3)
		sp.children = []*spec{genSpec(g, depth+1)}
	case kSwitch:
		sp.a = g.n(3)
		sp.children = []*spec{genSpec(g, depth+1), genSpec(g, depth+1)}
	case kFor:
		sp.a = g.n(2) // a row element, or a row component
		sp.children = []*spec{genSpec(g, depth+1)}
	case kPortal:
		sp.children = []*spec{genSpec(g, depth+1)}
	}
	return sp
}

type fuzzStep struct {
	op, a, b, c int
}

func (s fuzzStep) String() string {
	switch s.op {
	case opInt:
		return fmt.Sprintf("int%d=%d", s.a, s.b)
	case opBool:
		return fmt.Sprintf("toggle bool%d", s.a)
	}
	return fmt.Sprintf("list op%d(%d,%d)", s.a, s.b, s.c)
}

func genStep(g *fuzzInput) fuzzStep {
	op := g.n(numOps)
	switch op {
	case opInt:
		return fuzzStep{op: op, a: g.n(3), b: g.n(5)}
	case opBool:
		return fuzzStep{op: op, a: g.n(3)}
	}
	return fuzzStep{op: op, a: g.n(5), b: g.n(8), c: g.n(8)}
}

type item struct{ K, V int }

type fuzzState struct {
	ints  [3]*core.Signal[int]
	bools [3]*core.Signal[bool]
	list  *core.Signal[[]item]
	next  int // the next new key

	mounted, cleaned, doubleCleanups int
}

func newFuzzState() *fuzzState {
	st := &fuzzState{list: core.NewSignal([]item{{1, 1}, {2, 2}}), next: 3}
	for i := range st.ints {
		st.ints[i] = core.NewSignal(0)
		st.bools[i] = core.NewSignal(i == 0)
	}
	return st
}

func (st *fuzzState) copyValues(from *fuzzState) {
	for i := range st.ints {
		st.ints[i].Set(from.ints[i].Peek())
		st.bools[i].Set(from.bools[i].Peek())
	}
	st.list.Set(append([]item(nil), from.list.Peek()...))
}

func (st *fuzzState) live() int { return st.mounted - st.cleaned }

func (s fuzzStep) apply(st *fuzzState) {
	switch s.op {
	case opInt:
		st.ints[s.a].Set(s.b)
		return
	case opBool:
		st.bools[s.a].Set(!st.bools[s.a].Peek())
		return
	}
	list := append([]item(nil), st.list.Peek()...)
	at := func(i int) int {
		if len(list) == 0 {
			return 0
		}
		return i % len(list)
	}
	switch s.a {
	case 0: // insert a new key
		i := 0
		if len(list) > 0 {
			i = s.b % (len(list) + 1)
		}
		list = append(list[:i], append([]item{{st.next, s.c}}, list[i:]...)...)
		st.next++
	case 1: // remove
		if len(list) > 0 {
			i := at(s.b)
			list = append(list[:i], list[i+1:]...)
		}
	case 2: // move
		if len(list) > 1 {
			i, j := at(s.b), at(s.c)
			it := list[i]
			list = append(list[:i], list[i+1:]...)
			list = append(list[:j], append([]item{it}, list[j:]...)...)
		}
	case 3: // change a value, same key
		if len(list) > 0 {
			list[at(s.b)].V = s.c
		}
	case 4: // duplicate a key
		if len(list) > 0 {
			list = append(list, list[at(s.b)])
		}
	}
	st.list.Set(list)
}

var fuzzTags = [...]string{"div", "span", "p", "ul", "li", "svg"}

// fuzzRender builds the node for sp from st's signals. Everything a node
// shows comes from signals, so a component kept across a parent re-render
// shows the same as a fresh one.
func fuzzRender(sp *spec, st *fuzzState) core.Node {
	switch sp.kind {
	case kText:
		if sp.a == 0 {
			return Text("t")
		}
		sig := st.ints[sp.b]
		return TextS(core.Computed([]core.SignalAccessor{sig}, func() string { return fmt.Sprint(sig.Get()) }))
	case kElem:
		var items []core.Item
		switch sp.b {
		case 1:
			items = append(items, Class(fmt.Sprintf("s%d", sp.a)))
		case 2:
			sig := st.ints[sp.a%3]
			items = append(items, ClassS(core.Computed([]core.SignalAccessor{sig}, func() string { return fmt.Sprintf("b%d", sig.Get()) })),
				OnClick(func() {}))
		}
		tag := fuzzTags[sp.a]
		if tag == "svg" {
			items = append(items, &core.ElementNode{Tag: "circle", Attrs: []core.Attr{{Name: "r", Value: "1"}}})
		}
		for _, c := range sp.children {
			items = append(items, fuzzRender(c, st))
		}
		return El(tag, items...)
	case kFrag:
		kids := make([]core.Node, len(sp.children))
		for i, c := range sp.children {
			kids[i] = fuzzRender(c, st)
		}
		return Fragment(kids...)
	case kShow:
		return Show(st.bools[sp.a], func() core.Node { return fuzzRender(sp.children[0], st) })
	case kSwitch:
		return Switch(st.ints[sp.a], map[int]func() core.Node{
			0: func() core.Node { return fuzzRender(sp.children[0], st) },
			1: func() core.Node { return Span(Text("one")) },
		}, func() core.Node { return fuzzRender(sp.children[1], st) })
	case kComp:
		return fuzzComponent(fmt.Sprintf("C%d", sp.a), st, func() core.Node { return fuzzRender(sp.children[0], st) })
	case kFor:
		return For(st.list, func(it item) int { return it.K }, func(it item) core.Node {
			row := func() core.Node {
				return Li(Textf("%d:%d", it.K, it.V), fuzzRender(sp.children[0], st))
			}
			if sp.a == 1 {
				return fuzzComponent("Row", st, row)
			}
			return row()
		})
	case kPortal:
		return Portal("#fuzz-portal", Div(Class("portal"), fuzzRender(sp.children[0], st)))
	}
	return nil
}

// fuzzComponent counts its mounts and cleanups.
func fuzzComponent(name string, st *fuzzState, render func() core.Node) core.Node {
	return core.Component(name, func() core.Node {
		cleaned := false
		hooks.OnMount(func() func() {
			st.mounted++
			return func() {
				if cleaned {
					st.doubleCleanups++
				}
				cleaned = true
				st.cleaned++
			}
		})
		return render()
	})
}

// canonical serializes the document for comparison: elements with their
// attributes and properties (sorted), text, and the portal container's
// content; node ids and comments (scope anchors) are left out.
func canonical(s *Screen) string {
	var b strings.Builder
	var walk func(n *node)
	walk = func(n *node) {
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
			fmt.Fprintf(&b, " %s=%q", a.name, a.value)
		}
		var props []string
		for k, v := range n.props {
			props = append(props, fmt.Sprintf(" .%s=%v", k, v))
		}
		sort.Strings(props)
		b.WriteString(strings.Join(props, ""))
		b.WriteString(">")
		for _, c := range n.children {
			walk(c)
		}
		b.WriteString("</" + n.tag + ">")
	}
	for _, c := range s.doc.root.children {
		walk(c)
	}
	// A portal container exists once something was rendered into it; an
	// empty one shows the same as none. Portals sharing a container append in
	// mount order, which isn't meaningful: compare its content as a set.
	if p := s.doc.portal["#fuzz-portal"]; p != nil && len(p.children) > 0 {
		var parts []string
		for _, c := range p.children {
			start := b.Len()
			walk(c)
			parts = append(parts, b.String()[start:])
		}
		body := b.String()
		b.Reset()
		b.WriteString(body[:len(body)-len(strings.Join(parts, ""))])
		sort.Strings(parts)
		b.WriteString(" | portal: " + strings.Join(parts, ""))
	}
	return b.String()
}
