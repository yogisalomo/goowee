package app

import (
	"regexp"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/dom"
	"github.com/yogisalomo/goowee/internal/hydratetest"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/router"
	"github.com/yogisalomo/goowee/ssr"
)

// hydrateMuts is the client's hydrating render of page: the Hydrate
// mutations it sends to claim the server's markup. Rendered under EnvServer
// so component effects don't run (the stopwatch's ticker).
func hydrateMuts(page func() core.Node) []core.Mutation {
	var muts []core.Mutation
	runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
		r := dom.New()
		r.SetHydrating(true)
		muts, _ = r.Render(page())
	})
	return muts
}

// Hydration claims the server's nodes in document order (ADR-033), which only
// works if the client walks the same tree the server rendered. This asserts
// it for every example page: each Hydrate lands on a server node of the kind
// it expects, elements, text and scope anchors alike.
func TestSSRHydrationParity(t *testing.T) {
	r := testRouter()
	pages := map[string]func() core.Node{
		"home":      homePage,
		"counter":   func() core.Node { return counterPage(r) },
		"about":     aboutPage,
		"form":      func() core.Node { return formPage(r) },
		"todos":     func() core.Node { return todosPage(r) },
		"stopwatch": func() core.Node { return stopwatchPage(r) },
		"dashboard": func() core.Node { return dashboardPage(r) },
	}

	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			html, _ := ssr.New().Render(page())
			muts := hydrateMuts(page)
			claims := 0
			for _, m := range muts {
				if m.Type == core.MutHydrate {
					claims++
				}
			}
			claimed, mismatches := hydratetest.Claim(html, muts)
			if claims == 0 {
				t.Fatal("the client sent no Hydrate mutations")
			}
			if len(mismatches) > 0 || len(claimed) != claims {
				t.Fatalf("%d of %d claims landed; %v\nhtml: %s", len(claimed), claims, mismatches, html)
			}
		})
	}
}

// applyHydrating mimics the browser hydration path: a CreateElement whose id
// already exists (a server-rendered node) is reused rather than recreated;
// everything else applies normally (AppendChild only attaches detached nodes).
func (d *fakeDOM) applyHydrating(muts []core.Mutation) (created int) {
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			if _, ok := d.nodes[m.NodeID]; ok {
				continue // reuse the server node
			}
			created++
		}
		d.apply([]core.Mutation{m})
	}
	return created
}

// Hydrating a page over its server-rendered DOM must reuse every node and add
// nothing — no recreated elements, no duplicated text. We use a DOM render as
// the stand-in for the server DOM; TestSSRDOMIDParity is what guarantees that
// stand-in has the same ids as the real SSR output.
func TestHydrationReusesServerNodesNoDuplicates(t *testing.T) {
	r := testRouter()
	pages := map[string]func() core.Node{
		"home":    homePage,
		"counter": func() core.Node { return counterPage(r) },
		"form":    func() core.Node { return formPage(r) },
		"todos":   func() core.Node { return todosPage(r) },
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			var serverMuts []core.Mutation
			runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
				serverMuts, _ = dom.New().Render(page())
			})
			fd := newFakeDOM()
			fd.apply(serverMuts)
			nodesBefore := len(fd.nodes)
			textBefore := fd.text(0)

			var clientMuts []core.Mutation
			runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
				clientMuts, _ = dom.New().Render(page())
			})
			created := fd.applyHydrating(clientMuts)

			if created != 0 {
				t.Fatalf("hydration created %d new nodes, want 0 (all reused)", created)
			}
			if len(fd.nodes) != nodesBefore {
				t.Fatalf("node count changed on hydration: %d -> %d", nodesBefore, len(fd.nodes))
			}
			if got := fd.text(0); got != textBefore {
				t.Fatalf("text changed on hydration (duplication?):\nbefore=%q\nafter =%q", textBefore, got)
			}
		})
	}
}

// Server markup carries no node ids and no per-text markers (#141): it opens
// with the hydration marker, and the client claims nodes by position.
func TestSSRMarkupHasNoIDs(t *testing.T) {
	html, _ := ssr.New().Render(homePage())
	if !strings.HasPrefix(html, ssr.HydrationMarker) {
		t.Fatalf("no hydration marker at the start: %q", html)
	}
	if strings.Contains(html, "data-node-id") || regexp.MustCompile(`<!--g\d+-->`).MatchString(html) {
		t.Fatalf("ids or text markers in the markup: %q", html)
	}
	if !strings.Contains(html, "<h1>Welcome to Goowee</h1>") {
		t.Fatalf("heading not rendered plainly: %q", html)
	}
}

// A URL-param route: the component mounts once and is preserved across param
// changes; it reads the name reactively (ParamSignal), so navigating between
// /greet/alice and /greet/bob updates the text in place.
func TestGreetParamRouteReactive(t *testing.T) {
	r := router.New("/greet/alice")
	h := mount(t, App(r))
	if !strings.Contains(h.dom.text(0), "Hello, alice!") {
		t.Fatalf("want 'Hello, alice!' on mount, tree = %q", h.dom.text(0))
	}

	r.Navigate("/greet/bob")
	h.flush()
	if !strings.Contains(h.dom.text(0), "Hello, bob!") {
		t.Fatalf("param change should update text in place, tree = %q", h.dom.text(0))
	}
	if strings.Contains(h.dom.text(0), "Hello, alice!") {
		t.Fatalf("stale 'alice' text remained after nav, tree = %q", h.dom.text(0))
	}
}

// In hydrate mode the client render must emit ONLY claim mutations — no
// create/set/append — and applying them over the server DOM must change
// nothing (the optimization: reuse, don't rebuild).
func TestHydrateRenderEmitsOnlyClaims(t *testing.T) {
	r := testRouter()
	for name, page := range map[string]func() core.Node{
		"home": homePage, "counter": func() core.Node { return counterPage(r) }, "form": func() core.Node { return formPage(r) }, "todos": func() core.Node { return todosPage(r) },
	} {
		t.Run(name, func(t *testing.T) {
			var serverMuts []core.Mutation
			runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
				serverMuts, _ = dom.New().Render(page())
			})
			serverNodes := 0
			for _, m := range serverMuts {
				if m.Type == core.MutCreateElement {
					serverNodes++
				}
			}
			fd := newFakeDOM()
			fd.apply(serverMuts)
			nodesBefore := len(fd.nodes)
			textBefore := fd.text(0)

			r := dom.New()
			r.SetHydrating(true)
			var clientMuts []core.Mutation
			runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
				clientMuts, _ = r.Render(page())
			})

			if len(clientMuts) == 0 {
				t.Fatal("expected claim mutations")
			}
			// Claims, plus a Listen per handler (it marks the node for the
			// runtime's dispatch and doesn't touch the DOM).
			claims := 0
			for _, m := range clientMuts {
				switch m.Type {
				case core.MutHydrate:
					claims++
				case core.MutListen:
				default:
					t.Fatalf("hydrate render emitted %v (want only Hydrate and Listen): %+v", m.Type, m)
				}
			}
			// Exactly one claim per server-rendered node — nothing rebuilt.
			if claims != serverNodes {
				t.Fatalf("expected %d claims (one per server node), got %d", serverNodes, claims)
			}

			fd.apply(clientMuts) // claims — must not change the DOM
			if len(fd.nodes) != nodesBefore {
				t.Fatalf("hydration changed node count %d -> %d", nodesBefore, len(fd.nodes))
			}
			if fd.text(0) != textBefore {
				t.Fatalf("hydration changed text:\nbefore=%q\nafter =%q", textBefore, fd.text(0))
			}
		})
	}
}

// h.Dynamic marks a subtree as non-deterministic: hydration still claims the
// server nodes, but re-applies the client's values so the client wins over a
// differing server value. A plain (non-Dynamic) subtree trusts the server
// value (the optimization). Simulates SSR "SERVER" vs client "CLIENT".
func TestHydrateDynamicReappliesValues(t *testing.T) {
	render := func(n core.Node, hydrate bool) []core.Mutation {
		r := dom.New()
		if hydrate {
			r.SetHydrating(true)
		}
		var muts []core.Mutation
		runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
			muts, _ = r.Render(n)
		})
		return muts
	}
	server := func() core.Node {
		return &core.ElementNode{Tag: "span", Children: []core.Node{&core.TextNode{Value: "SERVER"}}}
	}
	client := func(dyn bool) core.Node {
		return &core.ElementNode{Tag: "span", Dynamic: dyn, Children: []core.Node{&core.TextNode{Value: "CLIENT"}}}
	}

	t.Run("dynamic: client value wins", func(t *testing.T) {
		fd := newFakeDOM()
		fd.apply(render(server(), false))
		if got := fd.text(0); got != "SERVER" {
			t.Fatalf("server DOM text = %q, want SERVER", got)
		}
		fd.apply(render(client(true), true))
		if got := fd.text(0); got != "CLIENT" {
			t.Fatalf("Dynamic hydrate should re-apply client text, got %q", got)
		}
	})

	t.Run("plain: server value trusted", func(t *testing.T) {
		fd := newFakeDOM()
		fd.apply(render(server(), false))
		fd.apply(render(client(false), true))
		if got := fd.text(0); got != "SERVER" {
			t.Fatalf("non-Dynamic hydrate should trust server text, got %q", got)
		}
	})
}

// After a hydrate render, handlers and bindings are wired, so the app is
// interactive without a full client re-render.
func TestHydrateStaysReactive(t *testing.T) {
	rt := testRouter()
	var serverMuts []core.Mutation
	runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
		serverMuts, _ = dom.New().Render(counterPage(rt))
	})
	fd := newFakeDOM()
	fd.apply(serverMuts)

	r := dom.New()
	r.SetHydrating(true)
	var clientMuts []core.Mutation
	r.Render(counterPage(rt)) // hydrate; sets client ids == server ids (parity)
	// (clientMuts unused; claims don't change fd)
	_ = clientMuts

	incr := fd.buttonWithText("Count: 0")
	if incr == 0 {
		t.Fatalf("increment button not found after hydrate; tree=%q", fd.text(0))
	}
	r.Registry.Dispatch(incr, "click", "{}")
	fd.apply(r.Scheduler.Flush())
	if !strings.Contains(fd.text(incr), "Count: 1") {
		t.Fatalf("not reactive after hydration: %q", fd.text(incr))
	}
}
