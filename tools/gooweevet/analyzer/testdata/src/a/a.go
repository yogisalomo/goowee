package a

import (
	"fmt"
	"time"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

func Snapshots() core.Node {
	return core.Component("Snap", func() core.Node {
		name, _ := hooks.UseState("ada")
		count := core.NewSignal(0)
		return P(
			Text(name.Get()),               // want `Text\(name.Get\(\)\) reads the signal once`
			Class(name.Get()),              // want `Class\(name.Get\(\)\) reads the signal once`
			Text(fmt.Sprint(count.Peek())), // Peek: an intended snapshot
			TextS(name),
			Button(OnClick(func() { _ = Text(name.Get()) })), // in a handler: runs later
		)
	})
}

func Deps() {
	a, b := core.NewSignal(1), core.NewSignal(2)
	_ = core.Computed([]core.SignalAccessor{a}, func() int {
		return a.Get() + b.Get() // want `Computed reads b.Get\(\) but doesn't list b in its deps`
	})
	_ = core.Computed(core.Deps(a, b), func() int { return a.Get() + b.Get() })
	_ = core.Computed([]core.SignalAccessor{a}, func() int { return a.Get() + b.Peek() })
	hooks.UseEffect(nil, func() func() {
		_ = a.Get() // want `UseEffect reads a.Get\(\) but doesn't list a in its deps`
		return nil
	})
	hooks.UseEffect([]core.SignalAccessor{a}, func() func() {
		tick := func() { _ = b.Get() } // runs later, not part of the effect's run
		tick()
		return nil
	})
	hooks.Watch([]core.SignalAccessor{a}, func() {
		local := core.NewSignal(0)
		_ = local.Get()                                                            // declared inside: not a dependency
		_ = core.Computed([]core.SignalAccessor{b}, func() int { return b.Get() }) // nested, its own deps
	})
	_ = hooks.UseScope(func() core.Node {
		return Text(fmt.Sprint(b.Get())) // want `UseScope reads b.Get\(\) but doesn't list b in its deps`
	}, a)
}

func OffLoop() {
	count, setCount := hooks.UseState(0)
	go func() {
		count.Set(1) // want `count.Set in a goroutine races the render loop`
		setCount(2)  // want `setCount, a UseState setter, called in a goroutine races the render loop`
		core.Schedule(func() {
			count.Set(3)
			setCount(4)
		})
	}()
	time.AfterFunc(time.Second, func() {
		setCount(5) // want `setCount, a UseState setter, called in a time.AfterFunc callback`
	})
	count.Set(6) // on the render loop
}
