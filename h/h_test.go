package h

import (
	"github.com/yogisalomo/goowee/core"
	"strconv"
	"strings"
	"testing"
)

func TestElAppliesItemsInOrder(t *testing.T) {
	el := El("div", Class("foo"), ID("bar"))
	if len(el.Attrs) != 2 {
		t.Fatalf("expected 2 attrs, got %d", len(el.Attrs))
	}
	if el.Attrs[0].Value != "foo" || el.Attrs[1].Name != "id" {
		t.Fatalf("unexpected attrs: %v", el.Attrs)
	}
}

func TestNilItemsSkipped(t *testing.T) {
	el := El("div", nil, Class("foo"), nil)
	if len(el.Attrs) != 1 {
		t.Fatalf("expected 1 attr, got %d", len(el.Attrs))
	}
}

func TestTypedNilNodeItemSkipped(t *testing.T) {
	el := El("div", (*core.ElementNode)(nil))
	if len(el.Children) != 0 {
		t.Fatal("expected no children for typed nil")
	}
}

func TestClassConcatenation(t *testing.T) {
	el := El("div", Class("foo"), Class("bar"))
	if len(el.Attrs) != 1 {
		t.Fatalf("expected 1 attr, got %d", len(el.Attrs))
	}
	if el.Attrs[0].Value != "foo bar" {
		t.Fatalf("expected 'foo bar', got %q", el.Attrs[0].Value)
	}
}

func TestIfElseMapGroup(t *testing.T) {
	el := El("div",
		If(true, Class("yes")),
		If(false, Class("no")),
		IfElse(true, Class("a"), Class("b")),
	)
	if len(el.Attrs) != 1 {
		t.Fatalf("expected 1 attr (class concatenation), got %d", len(el.Attrs))
	}
	if el.Attrs[0].Value != "yes a" {
		t.Fatalf("expected 'yes a', got %q", el.Attrs[0].Value)
	}

	items := Map([]int{1, 2, 3}, func(i int) core.Item { return Attr(strconv.Itoa(i), "v") })
	el2 := El("div", items)
	if len(el2.Attrs) != 3 {
		t.Fatalf("expected 3 attrs from Map, got %d", len(el2.Attrs))
	}

	el3 := El("div", Group(Attr("x", "a"), Attr("y", "b")))
	if len(el3.Attrs) != 2 {
		t.Fatalf("expected 2 attrs from Group, got %d", len(el3.Attrs))
	}
}

func TestTextfStatic(t *testing.T) {
	n := Textf("hello %s", "world")
	tn, ok := n.(*core.TextNode)
	if !ok {
		t.Fatalf("expected *core.TextNode, got %T", n)
	}
	if tn.Value != "hello world" {
		t.Fatalf("expected 'hello world', got %v", tn.Value)
	}
}

func TestTextfReactive(t *testing.T) {
	sig := core.NewSignal(42)
	args := []any{sig}
	n := Textf("Count: %d", args...)
	tn, ok := n.(*core.TextNode)
	if !ok {
		t.Fatalf("expected *core.TextNode, got %T", n)
	}
	if _, ok := tn.Value.(core.SignalAccessor); !ok {
		t.Fatalf("expected SignalAccessor, got %T", tn.Value)
	}
}

func TestOnSubmitForcesPreventDefault(t *testing.T) {
	item := OnSubmit(func(vals map[string]string) {})
	el := El("form", item)
	if len(el.Handlers) != 1 {
		t.Fatal("expected 1 handler")
	}
	if !el.Handlers[0].Options.PreventDefault {
		t.Fatal("OnSubmit should force PreventDefault")
	}
}

func TestDiv(t *testing.T) {
	el := Div(Class("test"), Text("hello"))
	if el.Tag != "div" {
		t.Fatalf("expected div, got %s", el.Tag)
	}
	if len(el.Attrs) != 1 || el.Attrs[0].Value != "test" {
		t.Fatal("expected class attr")
	}
}

func TestButton(t *testing.T) {
	el := Button(OnClick(func() {}), Text("click"))
	if el.Tag != "button" {
		t.Fatalf("expected button, got %s", el.Tag)
	}
	if len(el.Handlers) != 1 {
		t.Fatal("expected 1 handler")
	}
}

func TestInput(t *testing.T) {
	el := Input(Type("text"), Name("email"))
	if el.Tag != "input" {
		t.Fatalf("expected input, got %s", el.Tag)
	}
}

func TestFragment(t *testing.T) {
	f := Fragment(Text("a"), Text("b"))
	if len(f.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(f.Children))
	}
}

func TestTextS(t *testing.T) {
	sig := core.NewSignal("hello")
	tn := TextS(sig)
	if tn.Value != sig {
		t.Fatal("expected signal value in text node")
	}
}

func TestEventListenerOptions(t *testing.T) {
	item := OnClick(func() {}, PreventDefault(), StopPropagation())
	el := El("button", item)
	if len(el.Handlers) != 1 {
		t.Fatal("expected 1 handler")
	}
	if !el.Handlers[0].Options.PreventDefault {
		t.Fatal("expected PreventDefault")
	}
	if !el.Handlers[0].Options.StopPropagation {
		t.Fatal("expected StopPropagation")
	}
}

func TestKey(t *testing.T) {
	el := El("div", Key("mykey"))
	if el.Key != "mykey" {
		t.Fatalf("expected key 'mykey', got %v", el.Key)
	}
}

func TestBindValue(t *testing.T) {
	sig := core.NewSignal("")
	item := BindValue(sig)
	el := El("input", item)
	if len(el.Binds) != 1 {
		t.Fatal("expected 1 bind for value")
	}
	if len(el.Handlers) != 1 {
		t.Fatal("expected 1 handler for input event")
	}
}

func TestBindChecked(t *testing.T) {
	sig := core.NewSignal(false)
	item := BindChecked(sig)
	el := El("input", item)
	if len(el.Binds) != 1 {
		t.Fatal("expected 1 bind for checked")
	}
	if len(el.Handlers) != 1 {
		t.Fatal("expected 1 handler for input event")
	}
}

func TestAttrDataAria(t *testing.T) {
	el := El("div", Data("foo", "bar"), Aria("label", "test"))
	if len(el.Attrs) != 2 {
		t.Fatalf("expected 2 attrs, got %d", len(el.Attrs))
	}
	if el.Attrs[0].Name != "data-foo" || el.Attrs[0].Value != "bar" {
		t.Fatalf("unexpected data attr: %v", el.Attrs[0])
	}
	if el.Attrs[1].Name != "aria-label" || el.Attrs[1].Value != "test" {
		t.Fatalf("unexpected aria attr: %v", el.Attrs[1])
	}
}

func TestIntAttrs(t *testing.T) {
	el := El("div", Rows(5), Cols(3))
	if el.Attrs[0].Value != "5" || el.Attrs[1].Value != "3" {
		t.Fatalf("expected stringified ints, got %v", el.Attrs)
	}
}

func TestPropBool(t *testing.T) {
	el := El("input", Disabled(true), Checked(false))
	if len(el.Props) != 2 {
		t.Fatalf("expected 2 props, got %d", len(el.Props))
	}
	if el.Props[0].Value != true || el.Props[1].Value != false {
		t.Fatalf("unexpected prop values: %v", el.Props)
	}
}

func TestOnClickE(t *testing.T) {
	var called bool
	item := OnClickE(func(e core.EventData) { called = true })
	el := El("button", item)
	r := core.NewSignal(0)
	el.Handlers[0].Fn(core.EventData{Data: map[string]any{}})
	if !called {
		t.Fatal("OnClickE handler should have been called")
	}
	_ = r
}

func TestOnScroll(t *testing.T) {
	var st float64
	item := OnScroll(func(scrollTop float64) { st = scrollTop })
	el := El("div", item)
	el.Handlers[0].Fn(core.EventData{Data: map[string]any{"scrollTop": 100.0}})
	if st != 100.0 {
		t.Fatalf("expected 100, got %f", st)
	}
}

func TestInputElement(t *testing.T) {
	if Input().Tag != "input" {
		t.Fatal("Input should create input element")
	}
}

func TestBrElement(t *testing.T) {
	if Br().Tag != "br" {
		t.Fatal("Br should create br element")
	}
}

func TestPWithText(t *testing.T) {
	el := P(Text("hello"))
	if len(el.Children) != 1 {
		t.Fatal("expected 1 child")
	}
}

func TestAWithHref(t *testing.T) {
	el := A(Href("/test"), Text("link"))
	if el.Tag != "a" {
		t.Fatal("expected a tag")
	}
	if len(el.Attrs) == 0 || el.Attrs[0].Value != "/test" {
		t.Fatalf("expected href /test, got %v", el.Attrs)
	}
}

func TestTableElements(t *testing.T) {
	table := Table(
		Thead(Tr(Th(Text("h")))),
		Tbody(Tr(Td(Text("d")))),
		Tfoot(Tr(Td(Text("f")))),
	)
	if table.Tag != "table" {
		t.Fatal("expected table")
	}
	if len(table.Children) != 3 {
		t.Fatalf("expected 3 children (thead, tbody, tfoot), got %d", len(table.Children))
	}
}

func TestFormElements(t *testing.T) {
	form := Form(
		Label(Text("Name"), Input(Type("text"), Name("name"))),
		Select(
			Option(Value("a"), Text("A")),
			Option(Value("b"), Text("B")),
		),
		Textarea(Placeholder("bio")),
		Button(Text("Submit")),
	)
	if form.Tag != "form" {
		t.Fatal("expected form")
	}
	if len(form.Children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(form.Children))
	}
}

func TestSemanticElements(t *testing.T) {
	_ = Nav(Class("nav"))
	_ = Main()
	_ = Section()
	_ = Article()
	_ = Aside()
	_ = Header()
	_ = Footer()
}

func TestListElements(t *testing.T) {
	ul := Ul(
		Li(Text("a")),
		Li(Text("b")),
	)
	if ul.Tag != "ul" {
		t.Fatal("expected ul")
	}
	if len(ul.Children) != 2 {
		t.Fatalf("expected 2 lis, got %d", len(ul.Children))
	}
}

func TestHeadingElements(t *testing.T) {
	for _, fn := range []func(...core.Item) *core.ElementNode{H1, H2, H3, H4, H5, H6} {
		el := fn(Text("heading"))
		if strings.HasPrefix(el.Tag, "h") {
			continue
		}
		t.Fatalf("expected heading tag, got %s", el.Tag)
	}
}

func TestMediaElements(t *testing.T) {
	_ = Img(Src("pic.jpg"), Alt("a picture"))
	_ = Video(Source(Src("vid.mp4")))
	_ = Audio(Source(Src("aud.mp3")))
}

func TestMiscElements(t *testing.T) {
	_ = Code(Text("code"))
	_ = Pre(Text("pre"))
	_ = Blockquote(Text("quote"))
	_ = Strong(Text("bold"))
	_ = Em(Text("italic"))
	_ = Hr()
	_ = Br()
}

func TestAttrsOverride(t *testing.T) {
	el := El("div", ID("first"), ID("second"))
	if len(el.Attrs) != 1 {
		t.Fatalf("expected 1 attr after override, got %d", len(el.Attrs))
	}
	if el.Attrs[0].Value != "second" {
		t.Fatalf("expected 'second', got %q", el.Attrs[0].Value)
	}
}

func TestPropValue(t *testing.T) {
	el := El("input", Value("test"))
	if len(el.Props) != 1 || el.Props[0].Value != "test" {
		t.Fatalf("expected value prop 'test', got %v", el.Props)
	}
}

func TestStyleS(t *testing.T) {
	sig := core.NewSignal("color:red")
	el := El("div", StyleS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Name != "style" {
		t.Fatalf("expected bind for style, got %v", el.Binds)
	}
}

func TestClassS(t *testing.T) {
	sig := core.NewSignal("foo")
	el := El("div", ClassS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Target != core.BindToAttr {
		t.Fatalf("expected attr bind for class, got %v", el.Binds)
	}
}

func TestHrefS(t *testing.T) {
	sig := core.NewSignal("/test")
	el := El("a", HrefS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Name != "href" {
		t.Fatalf("expected bind for href, got %v", el.Binds)
	}
}

func TestBindProp(t *testing.T) {
	sig := core.NewSignal("val")
	el := El("input", BindProp("value", sig))
	if len(el.Binds) != 1 || el.Binds[0].Target != core.BindToProp {
		t.Fatalf("expected prop bind, got %v", el.Binds)
	}
}

func TestBindAttr(t *testing.T) {
	sig := core.NewSignal("val")
	el := El("div", BindAttr("title", sig))
	if len(el.Binds) != 1 || el.Binds[0].Target != core.BindToAttr {
		t.Fatalf("expected attr bind, got %v", el.Binds)
	}
}

func TestMapEmpty(t *testing.T) {
	el := El("div", Map([]int{}, func(i int) core.Item { return Class("x") }))
	if len(el.Attrs) != 0 {
		t.Fatal("expected no attrs from empty map")
	}
}

func TestPropGeneral(t *testing.T) {
	el := El("div", Prop("foo", 42))
	if len(el.Props) != 1 || el.Props[0].Value != 42 {
		t.Fatalf("expected prop foo=42, got %v", el.Props)
	}
}

func TestOnInput(t *testing.T) {
	var val string
	item := OnInput(func(value string) { val = value })
	el := El("input", item)
	el.Handlers[0].Fn(core.EventData{Data: map[string]any{"value": "hello"}})
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
}

func TestOnChange(t *testing.T) {
	var val string
	item := OnChange(func(value string) { val = value })
	el := El("input", item)
	el.Handlers[0].Fn(core.EventData{Data: map[string]any{"value": "changed"}})
	if val != "changed" {
		t.Fatalf("expected 'changed', got %q", val)
	}
}

func TestOnKeyDown(t *testing.T) {
	var key string
	item := OnKeyDown(func(k string) { key = k })
	el := El("input", item)
	el.Handlers[0].Fn(core.EventData{Data: map[string]any{"key": "Enter"}})
	if key != "Enter" {
		t.Fatalf("expected 'Enter', got %q", key)
	}
}

func TestOnFocus(t *testing.T) {
	var called bool
	item := OnFocus(func() { called = true })
	el := El("input", item)
	el.Handlers[0].Fn(core.EventData{})
	if !called {
		t.Fatal("expected focus handler to be called")
	}
}

func TestOnBlur(t *testing.T) {
	var called bool
	item := OnBlur(func() { called = true })
	el := El("input", item)
	el.Handlers[0].Fn(core.EventData{})
	if !called {
		t.Fatal("expected blur handler to be called")
	}
}

func TestValueS(t *testing.T) {
	sig := core.NewSignal("hello")
	el := El("input", ValueS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Name != "value" {
		t.Fatalf("expected bind for value, got %v", el.Binds)
	}
	if el.Binds[0].Target != core.BindToProp {
		t.Fatal("expected BindToProp for value")
	}
}

func TestCheckedS(t *testing.T) {
	sig := core.NewSignal(true)
	el := El("input", CheckedS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Name != "checked" {
		t.Fatalf("expected bind for checked, got %v", el.Binds)
	}
}

func TestDisabledS(t *testing.T) {
	sig := core.NewSignal(true)
	el := El("button", DisabledS(sig))
	if len(el.Binds) != 1 || el.Binds[0].Name != "disabled" {
		t.Fatalf("expected bind for disabled, got %v", el.Binds)
	}
}

func TestShowTogglesSubtree(t *testing.T) {
	show := core.NewSignal(true)
	seen := false
	n := Show(show, func() core.Node {
		seen = true
		return Text("visible")
	})
	scope, ok := n.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", n)
	}
	scope.Render()
	if !seen {
		t.Fatal("expected then to be called when true")
	}

	show.Set(false)
	scope.Render()
	_ = n
}

func TestShowElseBranches(t *testing.T) {
	cond := core.NewSignal(true)
	var thenCalled, elseCalled bool
	n := ShowElse(cond,
		func() core.Node { thenCalled = true; return Text("t") },
		func() core.Node { elseCalled = true; return Text("e") },
	)
	scope, ok := n.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", n)
	}
	scope.Render()
	if !thenCalled || elseCalled {
		t.Fatalf("expected then called=true, else=false, got then=%v else=%v", thenCalled, elseCalled)
	}

	cond.Set(false)
	thenCalled, elseCalled = false, false
	scope.Render()
	if thenCalled || !elseCalled {
		t.Fatalf("expected then=false, else=true when cond=false, got then=%v else=%v", thenCalled, elseCalled)
	}
}

func TestSwitchSelectsCaseAndDefault(t *testing.T) {
	sig := core.NewSignal(1)
	var matched int
	n := Switch(sig, map[int]func() core.Node{
		1: func() core.Node { matched = 1; return Text("one") },
		2: func() core.Node { matched = 2; return Text("two") },
	}, func() core.Node { matched = -1; return Text("default") })
	scope, ok := n.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", n)
	}
	scope.Render()
	if matched != 1 {
		t.Fatalf("expected case 1 to match, got %d", matched)
	}

	sig.Set(3)
	matched = 0
	scope.Render()
	if matched != -1 {
		t.Fatalf("expected default (-1), got %d", matched)
	}
}

func TestForRendersKeyedChildren(t *testing.T) {
	items := core.NewSignal([]string{"a", "b", "c"})
	fn := For(items, func(s string) string { return s }, func(s string) core.Node {
		return &core.ElementNode{Tag: "span"}
	})
	scope, ok := fn.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", fn)
	}
	inner := scope.Render()
	frag, ok := inner.(*core.FragmentNode)
	if !ok {
		t.Fatalf("expected *core.FragmentNode, got %T", inner)
	}
	if len(frag.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(frag.Children))
	}
	for _, c := range frag.Children {
		el, ok := c.(*core.ElementNode)
		if !ok {
			t.Fatalf("expected *core.ElementNode, got %T", c)
		}
		if el.Key == nil {
			t.Fatal("expected non-nil Key on For children")
		}
	}
}

func TestForNilAndEmptyLists(t *testing.T) {
	items := core.NewSignal([]string{})
	fn := For(items, func(s string) string { return s }, func(s string) core.Node {
		return &core.ElementNode{Tag: "span"}
	})
	scope, ok := fn.(*core.ScopeNode)
	if !ok {
		t.Fatalf("expected *core.ScopeNode, got %T", fn)
	}
	inner := scope.Render()
	frag, ok := inner.(*core.FragmentNode)
	if !ok {
		t.Fatalf("expected *core.FragmentNode, got %T", inner)
	}
	if len(frag.Children) != 0 {
		t.Fatalf("expected 0 children for empty list, got %d", len(frag.Children))
	}
}

func TestSVGElementSetsNamespace(t *testing.T) {
	svg := Svg(ViewBox("0 0 100 100"))
	if svg.Namespace != core.NamespaceSVG {
		t.Fatalf("expected Namespace=%q, got %q", core.NamespaceSVG, svg.Namespace)
	}
	if svg.Tag != "svg" {
		t.Fatalf("expected Tag=svg, got %q", svg.Tag)
	}
	if len(svg.Attrs) != 1 || svg.Attrs[0].Name != "viewBox" {
		t.Fatalf("expected viewBox attr, got %v", svg.Attrs)
	}
}

func TestSVGChildElementsInheritNamespace(t *testing.T) {
	svg := Svg(
		ViewBox("0 0 200 200"),
		Circle(Cx("100"), Cy("100"), R("50"), Fill("red")),
		Rect(SvgX("10"), SvgY("10"), Width("50"), Height("50")),
		Path(D("M10 10 L100 100"), Stroke("black"), StrokeWidth("2")),
		Ellipse(Cx("50"), Cy("50"), Rx("30"), Ry("20")),
		Line(SvgX("0"), SvgY("0"), Dx("100"), Dy("100")),
		Polyline(Points("0,0 50,50 100,0")),
		Polygon(Points("10,10 50,50 90,10")),
		SvgText(SvgX("10"), SvgY("20"), Text("hello")),
		Tspan(Dx("5"), Text("world")),
		Use(Attr("href", "#icon")),
		Defs(LinearGradient(
			Attr("id", "grad"),
			Stop(Attr("offset", "0%"), Attr("stop-color", "red")),
		)),
		ClipPath(Path(D("M0 0 L100 0 L100 100 Z"))),
		Mask(Rect(SvgX("0"), SvgY("0"), Width("100"), Height("100"), Fill("white"))),
	)

	if svg.Namespace != core.NamespaceSVG {
		t.Fatal("root svg missing namespace")
	}
	for _, child := range svg.Children {
		el, ok := child.(*core.ElementNode)
		if !ok {
			continue
		}
		if el.Namespace != core.NamespaceSVG {
			t.Fatalf("child %s missing SVG namespace", el.Tag)
		}
	}
}

func TestSVGAttributeHelpers(t *testing.T) {
	el := Circle(Cx("10"), Cy("20"), R("5"), Fill("blue"), Stroke("red"), StrokeWidth("1.5"))
	if len(el.Attrs) != 6 {
		t.Fatalf("expected 6 attrs, got %d: %v", len(el.Attrs), el.Attrs)
	}
	check := func(name, want string) {
		for _, a := range el.Attrs {
			if a.Name == name {
				if a.Value != want {
					t.Fatalf("%s: want %q, got %q", name, want, a.Value)
				}
				return
			}
		}
		t.Fatalf("missing attr %s", name)
	}
	check("cx", "10")
	check("cy", "20")
	check("r", "5")
	check("fill", "blue")
	check("stroke", "red")
	check("stroke-width", "1.5")
}

func TestSVGPathLengthFillOpacity(t *testing.T) {
	el := Path(D("M0 0"), PathLength("100"), FillOpacity("0.5"), StrokeOpacity("0.8"),
		StrokeLinecap("round"), StrokeLinejoin("round"))
	if len(el.Attrs) != 6 {
		t.Fatalf("expected 6 attrs, got %d", len(el.Attrs))
	}
}

func TestElNSSetsNamespace(t *testing.T) {
	el := ElNS("custom-elem", "urn:example:ns", Class("test"))
	if el.Namespace != "urn:example:ns" {
		t.Fatalf("expected ns=urn:example:ns, got %q", el.Namespace)
	}
	if el.Tag != "custom-elem" {
		t.Fatalf("expected tag=custom-elem, got %q", el.Tag)
	}
	if len(el.Attrs) != 1 || el.Attrs[0].Value != "test" {
		t.Fatal("expected class=test attr")
	}
}
