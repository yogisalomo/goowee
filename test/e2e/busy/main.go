//go:build js && wasm

// Command busy is the app test/e2e/busy.mjs measures (#158): one button runs
// about two seconds of pure Go computation in its handler, the other
// increments a counter. On the main thread the computation blocks the page;
// in worker mode the page keeps painting and the counter's clicks are queued
// for the worker.
package main

import (
	"strconv"
	"time"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// work spins for d, doing arithmetic the compiler can't remove.
func work(d time.Duration) uint64 {
	var x uint64 = 1
	for end := time.Now().Add(d); time.Now().Before(end); {
		for i := 0; i < 10000; i++ {
			x = x*6364136223846793005 + 1442695040888963407
		}
	}
	return x
}

func main() {
	bridge.Run(core.Component("Busy", func() core.Node {
		n, setN := hooks.UseState(0)
		result, setResult := hooks.UseState("idle")
		return Div(
			Button(ID("work"), OnClick(func() { setResult("done " + strconv.FormatUint(work(2*time.Second)%1000, 10)) }), Text("work")),
			P(ID("result"), TextS(result)),
			Button(ID("inc"), OnClick(func() { setN(n.Get() + 1) }), Text("+")),
			P(ID("count"), TextS(core.Computed([]core.SignalAccessor{n}, func() string { return strconv.Itoa(n.Get()) }))),
		)
	}))
}
