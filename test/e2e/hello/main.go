//go:build js && wasm

// Command hello is the smallest goowee app: test/e2e/wasmmime.mjs boots it
// under different server behaviours (#109), and checks its reactive document
// title (#121). It formats with strconv rather than fmt or Textf, so it is
// also the program CI checks links none of encoding/json, fmt, log and
// net/url (#126, docs/perf/size-budget.json).
package main

import (
	"strconv"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func main() {
	bridge.Run(core.Component("Hello", func() core.Node {
		n, setN := hooks.UseState(0)
		count := func() string { return strconv.Itoa(n.Get()) }
		return Div(
			Metadata(TitleS(core.Computed([]core.SignalAccessor{n}, func() string { return "(" + count() + ") hello" }))),
			P(ID("hello"), TextS(core.Computed([]core.SignalAccessor{n}, func() string { return "clicked " + count() }))),
			Button(ID("inc"), OnClick(func() { setN(n.Get() + 1) }), Text("+")),
		)
	}))
}
