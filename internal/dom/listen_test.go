package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
)

func listens(muts []core.Mutation) []string {
	var out []string
	for _, m := range muts {
		if m.Type == core.MutListen {
			s := "+"
			if m.Val() != true {
				s = "-"
			}
			out = append(out, s+m.Key)
		}
	}
	return out
}

// #132: the runtime learns which nodes have handlers. A re-render that keeps
// a handler sends nothing; one that adds or drops a handler says so.
func TestListenFollowsHandlerChanges(t *testing.T) {
	sig := core.NewSignal(0)
	nop := func(core.EventData) {}
	scope := &core.ScopeNode{
		Render: func() core.Node {
			hs := []core.Handler{{Event: "click", Fn: nop}}
			switch sig.Get() {
			case 1:
				hs = append(hs, core.Handler{Event: "input", Fn: nop})
			case 2:
				hs = []core.Handler{{Event: "input", Fn: nop}}
			}
			return &core.ElementNode{Tag: "input", Handlers: hs}
		},
		Deps: []core.SignalAccessor{sig},
	}
	r := New()
	muts, _ := renderFx(r, scope)
	if got := listens(muts); len(got) != 1 || got[0] != "+click" {
		t.Fatalf("first render: %v", got)
	}
	for _, tc := range []struct {
		v    int
		want string
	}{
		{1, "[+input]"},
		{1, "[]"}, // the same value: no re-render, nothing sent
		{2, "[-click]"},
		{0, "[+click -input]"},
	} {
		sig.Set(tc.v)
		if got := listens(flushFx(r)); fmtList(got) != tc.want {
			t.Fatalf("after Set(%d): %v, want %s", tc.v, got, tc.want)
		}
	}
}

func fmtList(s []string) string {
	out := "["
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out + "]"
}

func TestDispatchBits(t *testing.T) {
	if DispatchBits(core.HandlerOptions{PreventDefault: true}, false) != 0 {
		t.Fatal("unhandled must be 0")
	}
	got := DispatchBits(core.HandlerOptions{PreventDefault: true, StopPropagation: true, SelectOnFocus: true}, true)
	if got != 1|2|4|8 {
		t.Fatalf("got %b", got)
	}
	if DispatchBits(core.HandlerOptions{}, true) != 1 {
		t.Fatal("handled alone must be 1")
	}
}

// A handler's Listen tells the runtime its static options and its
// PreventDefaultWhen condition, which the page applies itself when Go runs in
// a worker (#158).
func TestListenCarriesOptions(t *testing.T) {
	cases := []struct {
		opts core.HandlerOptions
		want any
	}{
		{core.HandlerOptions{}, true},
		{core.HandlerOptions{PreventDefault: true}, "1"},
		{core.HandlerOptions{StopPropagation: true, SelectOnFocus: true}, "6"},
		{core.HandlerOptions{PreventWhen: core.PlainClick()}, "0\nplain"},
		{core.HandlerOptions{PreventDefault: true, PreventWhen: core.KeyIs("Enter").Without(core.ModShift)}, "1\nkey\n0\n4\nEnter"},
	}
	for _, c := range cases {
		if got := listen(1, "click", true, c.opts).Value; got != c.want {
			t.Errorf("%+v: Listen value %#v, want %#v", c.opts, got, c.want)
		}
	}
	if got := listen(1, "click", false, core.HandlerOptions{PreventDefault: true}).Value; got != false {
		t.Errorf("removed handler: %#v, want false", got)
	}
}
