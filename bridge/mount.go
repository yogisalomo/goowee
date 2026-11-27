//go:build js && wasm

package bridge

import (
	"encoding/json"
	"syscall/js"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/dom"
	"github.com/yogisalomo/goowee/internal/host"
)

// Root is an app or island mounted into the page by Mount or Islands.
type Root struct {
	r      *dom.DOMRenderer
	parent int
}

// app is what every root of this binary shares: one scheduler and frame
// loop, the handler and binding registries, and the node id counter (#154).
var app struct {
	base     *dom.DOMRenderer // the first root's renderer; the others share its state
	started  bool
	next     int  // container ids handed out to roots other than #root
	main     bool // #root is mounted
	baseUsed bool // app.base renders a root already
}

// Mount renders node into the element selector matches and returns at once,
// so a page can hold several goowee roots: an app and a widget, or islands in
// a server-rendered page (see Islands). Call Wait to keep the program alive.
// When the element holds server markup for node, Mount hydrates it.
//
// Every root shares one scheduler and frame loop, so core.Schedule,
// core.NextFrame and a signal read by two roots work across them. It returns
// nil, and logs why, when selector matches nothing.
func Mount(selector string, node core.Node) *Root {
	el := host.Call("query", selector)
	if !el.Truthy() {
		core.Log(core.LogWarn, "bridge.Mount: no element matches the selector", map[string]any{"selector": selector})
		return nil
	}
	return mountInto(el, node, false)
}

// Wait blocks forever, keeping the WASM module alive to service the mounted
// roots' events. Call it last in main, after Mount or Islands.
func Wait() { select {} }

// Islands hydrates every island in the page: each element ssr.Island wrote
// (data-goowee-island="name"), in document order, with the factory
// registered under its name, given the props the server rendered it with.
// An island whose name has no factory is left alone, with a warning. It
// returns the mounted roots, so one can be unmounted later.
//
//	bridge.Islands(map[string]func(json.RawMessage) core.Node{
//	    "cart": func(raw json.RawMessage) core.Node {
//	        var p CartProps
//	        _ = json.Unmarshal(raw, &p)
//	        return CartWidget(p)
//	    },
//	})
//	bridge.Wait()
func Islands(factories map[string]func(props json.RawMessage) core.Node) []*Root {
	list := host.Call("queryAll", "[data-goowee-island]")
	roots := make([]*Root, 0, list.Length())
	for i := 0; i < list.Length(); i++ {
		el := list.Index(i)
		name := el.Call("getAttribute", "data-goowee-island").String()
		factory, ok := factories[name]
		if !ok {
			core.Log(core.LogWarn, "bridge.Islands: no factory for this island; left as server rendered", map[string]any{"island": name})
			continue
		}
		props := json.RawMessage("null")
		if p := el.Call("getAttribute", "data-goowee-props"); p.Type() == js.TypeString {
			props = json.RawMessage(p.String())
		}
		roots = append(roots, mountInto(el, factory(props), true))
	}
	return roots
}

// Unmount removes the root's content from the page and runs its cleanups
// (effects, subscriptions). The container element stays.
func (r *Root) Unmount() {
	if r == nil || r.r == nil {
		return
	}
	r.r.Unmount()
	if fn := js.Global().Get("goowee").Get("unmountRoot"); fn.Type() == js.TypeFunction {
		fn.Invoke(r.parent)
	}
	r.r = nil
}

// mountInto renders node into el. island says el is an island container,
// whose server markup is numbered from 1 on its own.
func mountInto(el js.Value, node core.Node, island bool) *Root {
	start()
	g := js.Global().Get("goowee")
	parent := 0
	isRoot := el.Get("id").String() == "root" && !app.main
	if isRoot {
		app.main = true
	} else {
		parent = -1000 - app.next // -1000, -1001, …
		app.next++
	}
	r := app.base
	if app.baseUsed {
		r = dom.NewRoot(app.base, parent)
	} else {
		r.SetRootParent(parent)
		app.baseUsed = true
	}
	hydrate := false
	if isRoot && !island {
		hydrate = serverRendered()
	} else {
		hydrate = hasServerMarkup(el)
	}
	base := r.Walker.NextID() - 1
	if fn := g.Get("mountRoot"); fn.Type() == js.TypeFunction {
		fn.Invoke(parent, el, base, hydrate)
	} else if !isRoot {
		core.Log(core.LogWarn, "this page's goowee.js can't mount more than one root; serve the goowee.js of the app's goowee version", nil)
		return nil
	}
	r.SetHydrating(hydrate)
	muts, _ := r.Render(node)
	r.Scheduler.Enqueue(muts...)
	if isRoot {
		setInspectorRoot(node)
	}
	afterMount()
	return &Root{r: r, parent: parent}
}

// start sets up what every root shares, once: logging and dev tools, the
// scheduler core.Schedule posts to, event dispatch, and the frame loop.
func start() {
	if app.started {
		return
	}
	app.started = true
	initObservability()
	app.base = dom.New()
	// Route core.Schedule onto this scheduler before the first render, so a
	// mount effect can defer work (a ref read, say) to the first flush.
	core.SetActiveScheduler(app.base.Scheduler)
	initBridge(app.base.Scheduler, app.base.Registry, newTransport())
}
