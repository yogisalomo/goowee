package router

import (
	"reflect"
	"testing"

	"github.com/yogisalomo/goowee/core"
)

// #142: the URLs SetQueryParam and SetQueryParamPush write.
func TestSetQueryParamURLs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start string // Navigate target
		set   [][2]string
		want  string
	}{
		{"sorted keys", "/list", [][2]string{{"c", "3"}, {"a", "1"}, {"b", "2"}}, "/list?a=1&b=2&c=3"},
		{"no trailing ?", "/list?tag=go", [][2]string{{"tag", ""}}, "/list"},
		{"keeps the fragment", "/list?tag=go#results", [][2]string{{"page", "2"}}, "/list?page=2&tag=go#results"},
		{"fragment, no query left", "/list?tag=go#results", [][2]string{{"tag", ""}}, "/list#results"},
		{"escapes", "/s", [][2]string{{"q", "a b&c"}}, "/s?q=a+b%26c"},
		{"replaces every value", "/s?tag=a&tag=b", [][2]string{{"tag", "c"}}, "/s?tag=c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, push := range []bool{false, true} {
				r := New("/")
				var got string
				r.replaceFn = func(u string) { got = u }
				r.navFn = func(u string) { got = u }
				r.base = "/app"
				r.Navigate(tc.start)
				for _, kv := range tc.set {
					if push {
						r.SetQueryParamPush(kv[0], kv[1])
					} else {
						r.SetQueryParam(kv[0], kv[1])
					}
				}
				if got != "/app"+tc.want {
					t.Fatalf("push=%v: wrote %q, want %q", push, got, "/app"+tc.want)
				}
			}
		})
	}
}

// The same state always gives the same URL.
func TestEncodeQueryIsStable(t *testing.T) {
	m := map[string]string{"z": "1", "a": "2", "m": "3", "b": "4"}
	want := encodeQuery(m)
	for i := 0; i < 200; i++ {
		if got := encodeQuery(m); got != want {
			t.Fatalf("encodeQuery gave %q then %q", want, got)
		}
	}
	if want != "a=2&b=4&m=3&z=1" {
		t.Fatalf("got %q", want)
	}
}

func TestSetQueryParamWithoutChangeWritesNothing(t *testing.T) {
	r := New("/")
	writes := 0
	r.replaceFn = func(string) { writes++ }
	r.Navigate("/x?a=1")
	r.SetQueryParam("a", "1")
	r.SetQueryParam("b", "")
	if writes != 0 {
		t.Fatalf("%d writes for no change", writes)
	}
}

func TestFragment(t *testing.T) {
	r := New("/")
	r.Navigate("/doc?v=2#install")
	if r.Fragment.Peek() != "install" || r.Path.Peek() != "/doc" || r.QueryParam("v") != "2" {
		t.Fatalf("path %q query %v fragment %q", r.Path.Peek(), r.Query.Peek(), r.Fragment.Peek())
	}
	var pushed string
	r.navFn = func(u string) { pushed = u }
	r.Navigate("#usage") // same path and query
	if pushed != "/doc?v=2#usage" || r.Fragment.Peek() != "usage" {
		t.Fatalf("pushed %q, fragment %q", pushed, r.Fragment.Peek())
	}
	r.Navigate("/other") // a URL without a fragment clears it
	if r.Fragment.Peek() != "" || pushed != "/other" {
		t.Fatalf("pushed %q, fragment %q", pushed, r.Fragment.Peek())
	}
}

func TestQueryValues(t *testing.T) {
	r := New("/")
	r.Navigate("/s?tag=a&x=1&tag=b")
	if got := r.QueryValues("tag"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("QueryValues = %v", got)
	}
	if r.QueryParam("tag") != "b" || r.QueryValues("none") != nil {
		t.Fatalf("QueryParam %q, none %v", r.QueryParam("tag"), r.QueryValues("none"))
	}
	r.SetQueryParam("x", "2") // other keys keep all their values
	if got := r.QueryValues("tag"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("after SetQueryParam: %v", got)
	}
}

// Back and forward set path, query and fragment as one batch: a watcher never
// sees the new path with the old query.
func TestSyncURLIsOneBatch(t *testing.T) {
	r := New("/a")
	r.syncURL("/a", "?x=1", "")
	var seen []string
	unsub := []func(){}
	record := func() { seen = append(seen, r.Path.Peek()+"?"+r.rawQuery+"#"+r.Fragment.Peek()) }
	for _, s := range []core.SignalAccessor{r.Path, r.Query, r.Fragment} {
		unsub = append(unsub, s.Subscribe(record))
	}
	defer func() {
		for _, u := range unsub {
			u()
		}
	}()
	r.syncURL("/b", "?x=2", "#top")
	for _, s := range seen {
		if s != "/b?x=2#top" {
			t.Fatalf("a subscriber saw a mixed URL %q (all: %v)", s, seen)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no notification")
	}
}
