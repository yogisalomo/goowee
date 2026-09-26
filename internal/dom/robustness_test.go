package dom

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/ssr"
)

func textOf(muts []core.Mutation, into map[int]string) {
	for _, m := range muts {
		if m.Type == core.MutSetProperty && m.Key == "textContent" {
			into[m.NodeID] = fmt.Sprint(m.Value)
		}
	}
}

// #63: a slice-valued prop used to panic the differ ("comparing uncomparable
// type []int"); the recover left the scope frozen on its old DOM.
func TestUncomparablePropDoesNotFreezeScope(t *testing.T) {
	tick := core.NewSignal(0)
	root := h.Div(hooks.UseScope(func() core.Node {
		return h.Div(h.Prop("data", []int{1, 2}), h.Text(fmt.Sprint(tick.Get())))
	}, tick))
	r := New()
	muts, _ := r.Render(root)
	texts := map[int]string{}
	textOf(muts, texts)
	tick.Set(1)
	out := r.Scheduler.Flush()
	textOf(out, texts)
	found, reset := false, false
	for _, v := range texts {
		found = found || v == "1"
	}
	for _, m := range out {
		reset = reset || (m.Type == core.MutSetProperty && m.Key == "data")
	}
	if !found {
		t.Fatalf("scope did not update after an uncomparable prop: %v", texts)
	}
	if !reset {
		t.Fatal("an uncomparable prop value should be treated as changed and re-set")
	}
}

// #63: an uncomparable key can't index the keyed-diff map; it must fall back to
// positional matching instead of panicking.
func TestUncomparableKeyFallsBackToPositional(t *testing.T) {
	items := core.NewSignal([]string{"a", "b"})
	root := h.Ul(hooks.UseScope(func() core.Node {
		var rows []core.Node
		for _, s := range items.Get() {
			rows = append(rows, h.Li(h.Key([]string{s}), h.Text(s)))
		}
		return h.Fragment(rows...)
	}, items))
	r := New()
	muts, _ := r.Render(root)
	texts := map[int]string{}
	textOf(muts, texts)
	items.Set([]string{"a", "b", "c"})
	textOf(r.Scheduler.Flush(), texts)
	var got []string
	for _, v := range texts {
		got = append(got, v)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 rows after append, got %v", got)
	}
}

// #63: signal equality must not panic for a struct whose interface field holds
// an uncomparable value (the Type is comparable; the value is not).
func TestSignalStructWithUncomparableField(t *testing.T) {
	type box struct{ V any }
	s := core.NewSignal(box{V: []int{1}})
	n := 0
	s.Subscribe(func() { n++ })
	s.Set(box{V: []int{1}})
	if n != 1 {
		t.Fatalf("uncomparable values count as changed; want 1 notify, got %d", n)
	}
}

// #64: one NaN used to make json.Marshal reject the whole batch, and the bridge
// silently dropped every DOM update of the frame.
func TestEncodeBatchDropsOnlyUnencodable(t *testing.T) {
	batch := []core.Mutation{
		{Type: core.MutSetProperty, NodeID: 1, Key: "textContent", Value: "fine"},
		{Type: core.MutSetProperty, NodeID: 2, Key: "value", Value: math.NaN()},
		{Type: core.MutSetAttribute, NodeID: 3, Key: "class", Value: "ok"},
		{Type: core.MutSetProperty, NodeID: 4, Key: "x", Value: math.Inf(1)},
	}
	data, dropped := EncodeBatch(batch)
	var decoded []core.Mutation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("fallback output must be valid JSON: %v (%s)", err, data)
	}
	if len(decoded) != 2 || decoded[0].NodeID != 1 || decoded[1].NodeID != 3 {
		t.Fatalf("want the two encodable mutations in order, got %+v", decoded)
	}
	if len(dropped) != 2 || dropped[0].NodeID != 2 || dropped[1].NodeID != 4 {
		t.Fatalf("want nodes 2 and 4 reported as dropped, got %+v", dropped)
	}

	good := batch[:1]
	data, dropped = EncodeBatch(good)
	if dropped != nil || !strings.HasPrefix(string(data), "[{") {
		t.Fatalf("a valid batch encodes in one go: %s %v", data, dropped)
	}
}

// #64: bound text is formatted in Go with %v — the SSR renderer's format — so a
// float reads the same before and after hydration, and NaN is plain text.
func TestBoundTextFormattedLikeSSR(t *testing.T) {
	for _, v := range []float64{1e8, 0.1 + 0.2, math.NaN(), 3} {
		sig := core.NewSignal(v)
		r := New()
		muts, _ := r.Render(h.P(h.TextS(sig)))
		texts := map[int]string{}
		textOf(muts, texts)
		var client string
		for _, s := range texts {
			client = s
		}
		for _, m := range muts {
			if m.Key == "textContent" {
				if _, ok := m.Value.(string); !ok {
					t.Fatalf("text must be sent as a string, got %T", m.Value)
				}
			}
		}
		body, _ := ssr.New().Render(h.P(h.TextS(sig)))
		if !strings.Contains(body, client) {
			t.Fatalf("client text %q does not match SSR %q", client, body)
		}

		sig.Set(v + 1)
		for _, m := range r.Scheduler.Flush() {
			if _, ok := m.Value.(string); m.Key == "textContent" && !ok {
				t.Fatalf("updated text must be a string, got %T", m.Value)
			}
		}
	}
}
