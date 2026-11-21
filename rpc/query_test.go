package rpc_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/rpc"
)

type search struct{ Q string }

var find = rpc.Define[search, []string]("test.find")

// UseQuery calls on mount and again when the request changes, and binds the
// result like a resource.
func TestUseQuery(t *testing.T) {
	calls := 0
	rpc.Fake(t, find, func(ctx context.Context, req search) ([]string, error) {
		calls++
		return []string{req.Q + "1", req.Q + "2"}, nil
	})
	q := core.NewSignal(search{Q: "a"})
	s := gooweetest.Render(t, core.Component("Find", func() core.Node {
		res := rpc.UseQuery(find, q)
		text := core.Computed([]core.SignalAccessor{res.Data, res.Loading}, func() string {
			if res.Loading.Get() {
				return "loading"
			}
			return fmt.Sprint(res.Data.Get())
		})
		return P(ID("out"), TextS(text))
	}))
	s.WaitForText("[a1 a2]")
	q.Set(search{Q: "b"})
	s.WaitForText("[b1 b2]")
	if calls != 2 {
		t.Fatalf("%d calls, want 2", calls)
	}
}
