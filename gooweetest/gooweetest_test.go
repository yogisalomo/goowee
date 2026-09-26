package gooweetest_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func counter() core.Node {
	return core.Component("Counter", func() core.Node {
		n, setN := hooks.UseState(0)
		return Div(
			P(Class("count"), Textf("Count: %d", n)),
			Button(OnClick(func() { setN(n.Get() + 1) }), Text("Increment")),
		)
	})
}

func TestClickUpdatesText(t *testing.T) {
	s := gooweetest.Render(t, counter())
	s.Click(s.FindByText("Increment"))
	s.Click(s.FindByText("Increment"))
	if got := s.Find("p.count").Text(); got != "Count: 2" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(s.HTML(), `<p class="count">Count: 2</p>`) {
		t.Fatalf("HTML: %s", s.HTML())
	}
}

func TestFormAndListFlow(t *testing.T) {
	s := gooweetest.Render(t, core.Component("Todos", func() core.Node {
		items, setItems := hooks.UseState([]string{})
		draft, setDraft := hooks.UseState("")
		return Div(
			Form(Class("add"),
				OnSubmit(func(v map[string]string) {
					setItems(append(append([]string{}, items.Get()...), v["title"]))
					setDraft("")
				}),
				Input(Name("title"), BindValue(draft)),
				Button(Type("submit"), Text("Add")),
			),
			Ul(For(items, func(s string) string { return s }, func(s string) core.Node {
				return Li(Attr("data-title", s), Text(s))
			})),
			P(Textf("%d items", core.Computed([]core.SignalAccessor{items}, func() int { return len(items.Get()) }))),
		)
	}))
	form := s.Find("form.add")
	input := form.Find("input[name=title]")
	s.Input(input, "milk")
	if !s.Submit(form) {
		t.Fatal("OnSubmit prevents the browser's submission")
	}
	s.Input(input, "eggs")
	s.Submit(form)
	lis := s.FindAll("ul li")
	if len(lis) != 2 || lis[0].Text() != "milk" || lis[1].Attr("data-title") != "eggs" {
		t.Fatalf("rows: %s", s.HTML())
	}
	if input.Value() != "" {
		t.Fatalf("the draft should be cleared, got %q", input.Value())
	}
	if !strings.Contains(s.Text(), "2 items") {
		t.Fatalf("text: %q", s.Text())
	}
}

func TestBubblingAndPreventDefault(t *testing.T) {
	var log []string
	s := gooweetest.Render(t, Div(Class("card"), OnClick(func() { log = append(log, "card") }),
		Button(Class("inner"), OnClick(func() { log = append(log, "inner") })),
		Button(Class("stop"), OnClickE(func(e core.EventData) { e.StopPropagation(); log = append(log, "stop") })),
		Input(Class("field"), OnKeyDownE(func(e core.EventData) {
			if e.Key() == "Enter" {
				e.PreventDefault()
			}
		})),
	))
	s.Click(s.Find("button.inner"))
	s.Click(s.Find("button.stop"))
	if strings.Join(log, ",") != "inner,card,stop" {
		t.Fatalf("got %v", log)
	}
	if !s.KeyDown(s.Find(".field"), "Enter") || s.KeyDown(s.Find(".field"), "a") {
		t.Fatal("PreventDefault is per event")
	}
}

func TestEffectsRefsAndUnmount(t *testing.T) {
	cleaned := 0
	ref := Ref()
	s := gooweetest.Render(t, core.Component("C", func() core.Node {
		hooks.OnMount(func() func() {
			ref.Focus() // the element exists by now
			return func() { cleaned++ }
		})
		return Input(RefTo(ref), Class("name"))
	}))
	if f := s.Focused(); f == nil || f.Attr("class") != "name" {
		t.Fatalf("OnMount's ref.Focus should focus the input, got %v", f)
	}
	s.Unmount()
	if cleaned != 1 || s.HTML() != "" {
		t.Fatalf("unmount runs cleanups and empties the DOM: cleaned=%d html=%q", cleaned, s.HTML())
	}
}

func TestWaitForAsyncResource(t *testing.T) {
	s := gooweetest.Render(t, core.Component("Async", func() core.Node {
		res := hooks.UseResource(nil, func(ctx context.Context) (string, error) {
			time.Sleep(5 * time.Millisecond)
			return "loaded", nil
		})
		return ShowResource(res,
			func() core.Node { return P(Text("loading")) },
			func(err error) core.Node { return P(Text(err.Error())) },
			func(d *core.Signal[string]) core.Node { return P(TextS(d)) })
	}))
	if s.Text() != "loading" {
		t.Fatalf("got %q", s.Text())
	}
	s.WaitForText("loaded")
}

func TestPortalsAndHead(t *testing.T) {
	s := gooweetest.Render(t, Div(
		Metadata(Title("Hello")),
		Portal("#modal-root", Div(Class("modal"), Text("hi"))),
	))
	if !strings.Contains(s.Head(), "<title>Hello</title>") {
		t.Fatalf("head: %s", s.Head())
	}
	if p := s.Portal("#modal-root"); p == nil || p.Find(".modal").Text() != "hi" {
		t.Fatal("portal content missing")
	}
	if s.Find(".modal").Text() != "hi" {
		t.Fatal("Find searches portal content too")
	}
}
