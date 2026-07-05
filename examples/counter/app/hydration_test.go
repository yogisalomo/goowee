package app

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/dom"
	"github.com/yogisalomo/goowee/router"
	"github.com/yogisalomo/goowee/ssr"
)

var (
	reElemID = regexp.MustCompile(`<([a-z0-9]+) data-node-id="(\d+)"`)
	reTextID = regexp.MustCompile(`<!--g(\d+)-->`)
)

// ssrIDKinds parses the id -> kind map the SSR walker produced: elements carry
// data-node-id, text nodes carry a <!--g{id}--> marker.
func ssrIDKinds(html string) map[int]string {
	out := map[int]string{}
	for _, m := range reElemID.FindAllStringSubmatch(html, -1) {
		id, _ := strconv.Atoi(m[2])
		out[id] = m[1]
	}
	for _, m := range reTextID.FindAllStringSubmatch(html, -1) {
		id, _ := strconv.Atoi(m[1])
		out[id] = "#text"
	}
	return out
}

// domIDKinds is the id -> kind map the DOM walker produced, from its
// CreateElement mutations.
func domIDKinds(muts []core.Mutation) map[int]string {
	out := map[int]string{}
	for _, m := range muts {
		if m.Type == core.MutCreateElement {
			out[m.NodeID] = m.Value.(string)
		}
	}
	return out
}

// Hydration reuses server-rendered nodes by id, which only works if the SSR
// and DOM walkers assign the SAME id to the SAME node. This asserts that
// invariant for every example page — elements and text alike. Rendered under
// EnvServer on both sides so component effects don't run (ids don't depend on
// env, and this keeps the stopwatch's ticker from spawning).
func TestSSRDOMIDParity(t *testing.T) {
	pages := map[string]func() core.Node{
		"home":      homePage,
		"counter":   counterPage,
		"about":     aboutPage,
		"form":      formPage,
		"todos":     todosPage,
		"stopwatch": stopwatchPage,
		"dashboard": dashboardPage,
	}

	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			html := ssr.New().Render(page())

			var domMuts []core.Mutation
			core.UseContext(core.NewRenderContext(core.EnvServer), func() {
				domMuts, _ = dom.New().Render(page())
			})

			ssrMap := ssrIDKinds(html)
			domMap := domIDKinds(domMuts)

			if len(ssrMap) == 0 || len(domMap) == 0 {
				t.Fatalf("no nodes parsed (ssr=%d dom=%d)", len(ssrMap), len(domMap))
			}
			if len(ssrMap) != len(domMap) {
				t.Fatalf("node count mismatch: ssr=%d dom=%d\nssr=%v\ndom=%v",
					len(ssrMap), len(domMap), sortedKinds(ssrMap), sortedKinds(domMap))
			}
			for id, kind := range domMap {
				if ssrMap[id] != kind {
					t.Fatalf("id %d: dom=%q ssr=%q (parity broken → hydration would reuse the wrong node)\nssr=%v\ndom=%v",
						id, kind, ssrMap[id], sortedKinds(ssrMap), sortedKinds(domMap))
				}
			}
		})
	}
}

func sortedKinds(m map[int]string) string {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	s := ""
	for _, id := range ids {
		s += strconv.Itoa(id) + ":" + m[id] + " "
	}
	return s
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
	pages := map[string]func() core.Node{
		"home":    homePage,
		"counter": counterPage,
		"form":    formPage,
		"todos":   todosPage,
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			var serverMuts []core.Mutation
			core.UseContext(core.NewRenderContext(core.EnvServer), func() {
				serverMuts, _ = dom.New().Render(page())
			})
			fd := newFakeDOM()
			fd.apply(serverMuts)
			nodesBefore := len(fd.nodes)
			textBefore := fd.text(0)

			var clientMuts []core.Mutation
			core.UseContext(core.NewRenderContext(core.EnvServer), func() {
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

// SSR must emit a hydration marker for each text node, carrying the same id
// the DOM walker will assign it.
func TestSSRTextHydrationMarkers(t *testing.T) {
	html := ssr.New().Render(homePage()) // H1 with a "Welcome to Goowee" text node
	markers := reTextID.FindAllStringSubmatch(html, -1)
	if len(markers) == 0 {
		t.Fatalf("expected text hydration markers in %q", html)
	}
	// The marker must sit immediately before its text content.
	if !regexp.MustCompile(`<!--g\d+-->Welcome to Goowee`).MatchString(html) {
		t.Fatalf("text marker not positioned before its content: %q", html)
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
	for name, page := range map[string]func() core.Node{
		"home": homePage, "counter": counterPage, "form": formPage, "todos": todosPage,
	} {
		t.Run(name, func(t *testing.T) {
			var serverMuts []core.Mutation
			core.UseContext(core.NewRenderContext(core.EnvServer), func() {
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
			core.UseContext(core.NewRenderContext(core.EnvServer), func() {
				clientMuts, _ = r.Render(page())
			})

			if len(clientMuts) == 0 {
				t.Fatal("expected claim mutations")
			}
			for _, m := range clientMuts {
				if m.Type != core.MutHydrate {
					t.Fatalf("hydrate render emitted %v (want only Hydrate): %+v", m.Type, m)
				}
			}
			// Exactly one claim per server-rendered node — nothing rebuilt.
			if len(clientMuts) != serverNodes {
				t.Fatalf("expected %d claims (one per server node), got %d", serverNodes, len(clientMuts))
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

// After a hydrate render, handlers and bindings are wired, so the app is
// interactive without a full client re-render.
func TestHydrateStaysReactive(t *testing.T) {
	var serverMuts []core.Mutation
	core.UseContext(core.NewRenderContext(core.EnvServer), func() {
		serverMuts, _ = dom.New().Render(counterPage())
	})
	fd := newFakeDOM()
	fd.apply(serverMuts)

	r := dom.New()
	r.SetHydrating(true)
	var clientMuts []core.Mutation
	r.Render(counterPage()) // hydrate; sets client ids == server ids (parity)
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
