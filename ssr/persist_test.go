package ssr

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/storage"
)

// The server renders a persisted signal's own value and leaves storage alone:
// browser storage is the client's, read once the page has hydrated.
func TestSSRUsePersistRendersTheSignalsValue(t *testing.T) {
	storage.Local.Set("n", "5") // the native build's store, standing in for a browser's
	defer storage.Local.Clear()
	n := core.NewSignal(0)
	body, _ := New().Render(core.Component("C", func() core.Node {
		hooks.UsePersist(n, "n")
		return h.P(h.Textf("%d", n))
	}))
	if !strings.Contains(body, "-->0</p>") || n.Get() != 0 {
		t.Fatalf("n=%d body %s", n.Get(), body)
	}
}
