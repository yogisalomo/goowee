package h_test

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func TestBindNumberClassIfAndStyleProp(t *testing.T) {
	var count *core.Signal[int]
	var distance *core.Signal[float64]
	var active *core.Signal[bool]

	app := core.Component("Binds", func() core.Node {
		count, _ = hooks.UseState(2)
		distance, _ = hooks.UseState(10.0)
		active, _ = hooks.UseState(false)
		return Div(
			ID("box"), Class("base"), ClassIf("active", active),
			StyleProp("color", core.NewSignal("red")), StylePx("left", distance),
			Input(ID("num"), Type("number"), BindNumber(count)),
			Input(ID("distance"), Type("range"), BindNumber(distance)),
			Button(ID("toggle"), OnClick(func() { active.Set(!active.Get()) }), Text("toggle")),
			P(ID("value"), Textf("%d", count)),
		)
	})
	s := gooweetest.Render(t, app)

	if got := s.Find("#num").Value(); got != "2" {
		t.Fatalf("initial number value %q, want 2", got)
	}
	if got := s.Find("#box").Attr("class"); got != "base" {
		t.Fatalf("initial class %q, want base", got)
	}
	if got := s.Find("#box").Attr("style"); got != "color:red;left:10px" {
		t.Fatalf("initial style %q", got)
	}

	s.InputNumber(s.Find("#num"), 7)
	if got := s.Find("#value").Text(); got != "7" {
		t.Fatalf("number signal %q, want 7", got)
	}
	if got := s.Find("#num").Value(); got != "7" {
		t.Fatalf("number input %q, want 7", got)
	}

	s.Click(s.Find("#toggle"))
	if got := s.Find("#box").Attr("class"); !strings.Contains(got, "active") {
		t.Fatalf("class after toggle %q, want active", got)
	}
	s.Click(s.Find("#toggle"))
	if got := s.Find("#box").Attr("class"); strings.Contains(got, "active") {
		t.Fatalf("class after second toggle %q, want no active", got)
	}

	s.InputNumber(s.Find("#distance"), 20)
	if got := s.Find("#box").Attr("style"); !strings.Contains(got, "left:20px") {
		t.Fatalf("style after move %q, want left:20px", got)
	}
}

func TestBindRadioSelectMultiFilesAndAttrs(t *testing.T) {
	var choice *core.Signal[string]
	var multi *core.Signal[[]string]
	var files *core.Signal[[]core.File]
	var open *core.Signal[bool]
	var hidden *core.Signal[bool]

	app := core.Component("Inputs", func() core.Node {
		choice, _ = hooks.UseState("one")
		multi, _ = hooks.UseState([]string{"a"})
		files, _ = hooks.UseState([]core.File{})
		open, _ = hooks.UseState(false)
		hidden, _ = hooks.UseState(false)
		return Div(
			Input(ID("one"), Type("radio"), Value("one"), BindRadio(choice, "one")),
			Input(ID("two"), Type("radio"), Value("two"), BindRadio(choice, "two")),
			Select(ID("multi"), BindSelectMulti(multi),
				Option(Value("a"), Text("A")), Option(Value("b"), Text("B"))),
			Input(ID("file"), Type("file"), BindFiles(files)),
			Button(ID("menu"), AriaExpandedS(open), HiddenS(hidden), Text("menu")),
			P(ID("state"), Textf("%s|%v|%d", choice, multi,
				core.Computed([]core.SignalAccessor{files}, func() int { return len(files.Get()) }))),
		)
	})
	s := gooweetest.Render(t, app)

	if !s.Find("#one").Checked() || s.Find("#two").Checked() {
		t.Fatal("initial radio state: one should be checked")
	}
	if got := s.Find("#menu").Attr("aria-expanded"); got != "false" {
		t.Fatalf("initial aria-expanded %q, want false", got)
	}

	s.Check(s.Find("#two"), true)
	got := s.Find("#state").Text()
	if got != "two|[a]|0" {
		t.Fatalf("after radio %q", got)
	}

	s.SelectMultiple(s.Find("#multi"), "a", "b")
	got = s.Find("#state").Text()
	if got != "two|[a b]|0" {
		t.Fatalf("after multi-select %q", got)
	}

	s.SelectFiles(s.Find("#file"), gooweetest.File{Name: "note.txt", Data: []byte("hi")})
	got = s.Find("#state").Text()
	if got != "two|[a b]|1" {
		t.Fatalf("after file pick %q", got)
	}
}

func TestAttrSRemovesFalse(t *testing.T) {
	var hidden *core.Signal[bool]
	app := core.Component("Attrs", func() core.Node {
		hidden, _ = hooks.UseState(true)
		return Div(
			ID("box"), AttrS("hidden", hidden), AttrS("data-n", core.NewSignal(3)),
			Button(ID("show"), OnClick(func() { hidden.Set(false) }), Text("show")),
		)
	})
	s := gooweetest.Render(t, app)
	if got := s.Find("#box").Attr("hidden"); got != "true" {
		t.Fatalf("hidden %q, want true", got)
	}
	if got := s.Find("#box").Attr("data-n"); got != "3" {
		t.Fatalf("data-n %q, want 3", got)
	}
	s.Click(s.Find("#show"))
	if got := s.Find("#box").Attr("hidden"); got != "" {
		t.Fatalf("hidden after click %q, want removed", got)
	}
}
