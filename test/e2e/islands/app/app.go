// Package app is the islands test/e2e/islands.mjs drives (#154): a counter
// that starts at a server-given value, rendered as an island into a plain
// server page, twice.
package app

import (
	"encoding/json"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// CounterProps are the island's props, sent by the server as JSON.
type CounterProps struct {
	Label string `json:"label"`
	Start int    `json:"start"`
}

// Counter is the island component.
func Counter(p CounterProps) core.Node {
	return core.Component("Counter", func() core.Node {
		n, setN := hooks.UseState(p.Start)
		return Div(Class("counter"),
			Span(Class("label"), Text(p.Label)),
			Button(Class("inc"), OnClick(func() { setN(n.Get() + 1) }), Textf("%d", n)),
		)
	})
}

// Factory builds Counter from the props an island carries.
func Factory(raw json.RawMessage) core.Node {
	var p CounterProps
	_ = json.Unmarshal(raw, &p)
	return Counter(p)
}
