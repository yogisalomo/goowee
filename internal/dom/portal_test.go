package dom

import (
	"strconv"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/ssr"
)

// portalTree renders a portal container's children as tags, anchors included
// (as "#"), so placement relative to each portal's anchor is visible.
func portalTree(d *fakeDOM, selector string) string {
	var tags []string
	for _, id := range d.container(selector).Children {
		n := d.nodes[id]
		switch {
		case n == nil:
			tags = append(tags, "?")
		case n.Tag == "#comment":
			tags = append(tags, "#")
		default:
			tags = append(tags, d.tree(id))
		}
	}
	return strings.Join(tags, " ")
}

func removedIDs(muts []core.Mutation) map[int]bool {
	out := map[int]bool{}
	for _, m := range muts {
		if m.Type == core.MutRemoveNode {
			out[m.NodeID] = true
		}
	}
	return out
}

// A parent re-render no longer rebuilds the portal: a component inside keeps
// its frame (no remount) and its DOM nodes.
func TestPortalContentSurvivesParentReRender(t *testing.T) {
	title := core.NewSignal("a")
	mounts := 0
	var input *core.ElementNode
	modal := func() core.Node {
		return core.Component("Modal", func() core.Node {
			mounts++
			input = h.Input()
			return h.Div(h.Class("modal"), input)
		})
	}
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		return h.Section(h.H2(h.Text(title.Get())), h.Portal("#modal-root", modal()))
	}, title)))
	inputID := input.ID

	title.Set("b")
	muts := r.Scheduler.Flush()
	d.apply(muts)
	r.Scheduler.RunEffects()

	if mounts != 1 {
		t.Fatalf("the portal's component remounted on a parent re-render (mounts=%d)", mounts)
	}
	if len(removedIDs(muts)) != 0 {
		t.Fatalf("a parent re-render removed portal content: %v", removedIDs(muts))
	}
	if got := portalTree(d, "#modal-root"); got != "div(input) #" {
		t.Fatalf("portal container = %q, want the content then the anchor", got)
	}
	if d.nodes[inputID] == nil {
		t.Fatal("the input inside the portal was replaced")
	}
}

// Plain elements inside a portal are diffed in place: a changed child is
// updated, a new one inserted before the portal's anchor.
func TestPortalChildrenDiffedInPlace(t *testing.T) {
	n := core.NewSignal(1)
	var first *core.ElementNode
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		var items []core.Node
		for i := 0; i < n.Get(); i++ {
			items = append(items, h.P(h.Class("toast")))
		}
		first = items[0].(*core.ElementNode)
		return h.Portal("#toasts", items...)
	}, n)))
	firstID := first.ID

	n.Set(3)
	flush(r, d)
	if first.ID != firstID {
		t.Fatalf("the first toast was recreated (id %d -> %d)", firstID, first.ID)
	}
	if got := portalTree(d, "#toasts"); got != "p p p #" {
		t.Fatalf("after growing: %q", got)
	}
	n.Set(1)
	flush(r, d)
	if got := portalTree(d, "#toasts"); got != "p #" {
		t.Fatalf("after shrinking: %q", got)
	}
}

// Keyed children inside a portal reorder like any keyed list, relative to the
// portal's anchor.
func TestPortalKeyedReorder(t *testing.T) {
	order := core.NewSignal([]string{"a", "b", "c"})
	ids := map[string]int{}
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		var rows []core.Node
		for _, k := range order.Get() {
			el := h.El(k, h.Key(k))
			rows = append(rows, el)
		}
		return h.Portal("#list", rows...)
	}, order)))
	for _, id := range d.container("#list").Children {
		ids[d.nodes[id].Tag] = id
	}

	order.Set([]string{"c", "a", "b"})
	flush(r, d)
	if got := portalTree(d, "#list"); got != "c a b #" {
		t.Fatalf("after reorder: %q", got)
	}
	for _, id := range d.container("#list").Children {
		if n := d.nodes[id]; n.Tag != "#comment" && ids[n.Tag] != id {
			t.Fatalf("<%s> was recreated by a keyed reorder", n.Tag)
		}
	}
}

// Two portals into one container: content added to the first lands before the
// first portal's anchor, not after the second portal's content.
func TestPortalsSharingATargetKeepTheirOrder(t *testing.T) {
	n := core.NewSignal(1)
	r, d := mount(t, h.Div(
		hooks.UseScope(func() core.Node {
			var items []core.Node
			for i := 0; i < n.Get(); i++ {
				items = append(items, h.P())
			}
			return h.Portal("body", items...)
		}, n),
		h.Portal("body", h.Aside()),
	))
	if got := portalTree(d, "body"); got != "p # aside #" {
		t.Fatalf("initial: %q", got)
	}
	n.Set(2)
	flush(r, d)
	if got := portalTree(d, "body"); got != "p p # aside #" {
		t.Fatalf("after the first portal grew: %q", got)
	}
}

// A portal whose target changes is rebuilt in the new container, and the old
// container is left empty (content and anchor removed).
func TestPortalTargetChangeRebuilds(t *testing.T) {
	where := core.NewSignal("#left")
	r, d := mount(t, h.Div(hooks.UseScope(func() core.Node {
		return h.Portal(where.Get(), h.Span())
	}, where)))
	where.Set("#right")
	flush(r, d)
	if got := portalTree(d, "#left"); got != "" {
		t.Fatalf("old target still holds %q", got)
	}
	if got := portalTree(d, "#right"); got != "span #" {
		t.Fatalf("new target = %q", got)
	}
}

// Removing the subtree that owns a portal removes the portal's content and
// its anchor from the target.
func TestRemovingAPortalRemovesItsAnchor(t *testing.T) {
	show := core.NewSignal(true)
	r, d := mount(t, h.Div(h.Show(show, func() core.Node {
		return h.Portal("#modal-root", h.Div())
	})))
	show.Set(false)
	flush(r, d)
	if got := portalTree(d, "#modal-root"); got != "" {
		t.Fatalf("target still holds %q after the portal was removed", got)
	}
}

// The portal's anchor id is allocated by the shared walker, so nodes after a
// portal keep SSR/client id parity and hydrate against the right server nodes.
func TestPortalAnchorKeepsHydrationIDParity(t *testing.T) {
	page := func() core.Node {
		return h.Div(h.Portal("#modal-root", h.Span(h.Text("m"))), h.P(h.Text("after")))
	}
	html, _ := ssr.New().Render(page())

	var muts []core.Mutation
	runtime.UseContext(runtime.NewRenderContext(runtime.EnvServer), func() {
		r := New()
		r.SetHydrating(true)
		muts, _ = r.Render(page())
	})
	var pID int
	for _, m := range muts {
		if m.Type == core.MutHydrate && m.Value == "p" {
			pID = m.NodeID
		}
	}
	if pID == 0 || !strings.Contains(html, `<p data-node-id="`+strconv.Itoa(pID)+`"`) {
		t.Fatalf("client claims <p> as node %d; server HTML: %s", pID, html)
	}
}
