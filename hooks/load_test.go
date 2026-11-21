package hooks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/data"
)

func greeting(calls *int) core.Node {
	return hooks.Load("greeting", nil, func(ctx context.Context) (string, error) {
		*calls++
		return "fetched", nil
	}, func(res *hooks.Resource[string]) core.Node {
		return P(ID("out"), TextS(core.Computed([]core.SignalAccessor{res.Data, res.Loading}, func() string {
			if res.Loading.Get() {
				return "loading"
			}
			return res.Data.Get()
		})))
	})
}

// #152: with the page's data, a Load starts loaded and doesn't fetch; the
// data is used once, so a later mount fetches.
func TestLoadStartsWithPageData(t *testing.T) {
	defer data.SetPageData(map[string]json.RawMessage{"greeting": json.RawMessage(`"from the server"`)})()
	calls := 0
	s := gooweetest.Render(t, greeting(&calls))
	if got := s.Find("#out").Text(); got != "from the server" || calls != 0 {
		t.Fatalf("text %q, %d fetches; want the page's data and no fetch", got, calls)
	}
	s.Unmount()

	s2 := gooweetest.Render(t, greeting(&calls))
	s2.WaitForText("fetched")
	if calls != 1 {
		t.Fatalf("a second mount made %d fetches, want 1", calls)
	}
}

// Without page data a Load is UseResource.
func TestLoadWithoutPageData(t *testing.T) {
	defer data.SetPageData(nil)()
	calls := 0
	s := gooweetest.Render(t, greeting(&calls))
	s.WaitForText("fetched")
	if calls != 1 {
		t.Fatalf("%d fetches", calls)
	}
}
