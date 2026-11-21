//go:build js && wasm

// Command calls is the app test/e2e/calls.mjs drives: events and frames that
// reach Go through its exports (#133), and the cases that depend on the Go
// scheduler running after an export returns, or on an export re-entered
// while the runtime applies a batch.
package main

import (
	"fmt"

	"github.com/yogisalomo/goowee/bridge"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// ballast keeps what the re-entrant handler allocates, so the module's memory
// grows while the runtime is reading a batch out of it.
var ballast [][]byte

func main() {
	bridge.Run(core.Component("CallsE2E", func() core.Node {
		n, setN := hooks.UseState(0)
		spawned, setSpawned := hooks.UseState("")
		woken, setWoken := hooks.UseState("")
		mounted, setMounted := hooks.UseState("")
		grown, setGrown := hooks.UseState("")
		labels := make([]*core.Signal[string], 300)
		for i := range labels {
			labels[i], _ = hooks.UseState("")
		}
		target := Ref()
		wake := make(chan int)

		hooks.OnMount(func() func() {
			// A goroutine an effect starts runs after the frame's export.
			go func() { core.Schedule(func() { setMounted("mounted") }) }()
			// A worker parked on a channel, woken from a handler below.
			go func() {
				for v := range wake {
					core.Schedule(func() { setWoken(fmt.Sprintf("woken %d", v)) })
				}
			}()
			return nil
		})

		return Div(
			Button(ID("inc"), OnClick(func() { setN(n.Get() + 1) }), Textf("n %d", n)),
			// Started in a handler and not waited for: it has to run without
			// another event or timer.
			Button(ID("spawn"), OnClick(func() {
				go func() { core.Schedule(func() { setSpawned("spawned") }) }()
			}), Text("spawn")),
			Button(ID("wake"), OnClick(func() { wake <- 7 }), Text("wake")),
			// The click on #target happens inside the batch (ref.Click), runs
			// a handler that grows the memory, and the batch's text updates
			// after it must still apply.
			Button(ID("grow"), OnClick(func() {
				target.Click()
				for i, l := range labels {
					l.Set(fmt.Sprintf("label %d", i))
				}
			}), Text("grow")),
			Button(ID("target"), RefTo(target), OnClick(func() {
				ballast = append(ballast, make([]byte, 64<<20))
				setGrown(fmt.Sprintf("grown %d", len(ballast)))
			}), Text("target")),
			P(ID("spawned"), TextS(spawned)),
			P(ID("woken"), TextS(woken)),
			P(ID("mounted"), TextS(mounted)),
			P(ID("grown"), TextS(grown)),
			Div(ID("labels"), Map(labels, func(l *core.Signal[string]) core.Item { return Span(TextS(l)) })),
		)
	}))
}
