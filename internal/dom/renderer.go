package dom

import (
	"fmt"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/internal/walker"
)

// detached is pushed on parentStack while a fresh subtree is rendered inside a
// diff (renderFresh): the subtree's top-level nodes are not attached during the
// walk, because the diff knows where they belong and places them itself.
const detached = -1

// scopeInfo is the renderer's state for one mounted scope. It moves from the
// old ScopeNode to the new one when a parent re-render adopts the scope.
type scopeInfo struct {
	seq         int                  // mount order; parents re-render before children
	parentID    int                  // DOM parent (best effort; JS inserts at the anchor's real parent)
	owner       *core.ComponentFrame // owns the hooks called by the current render
	ownerParent *core.ComponentFrame // the frame the scope was mounted under
}

type scopeState struct {
	seq      int
	parentID int
	detached bool
}

type DOMRenderer struct {
	walker.Walker
	Bindings  *runtime.BindingRegistry
	Scheduler *core.Scheduler
	Registry  *NodeRegistry

	parentStack    []int
	freshParent    int // the real parent while parentStack's top is `detached`
	scopeSeq       int // monotonic mount order
	scopeStack     []scopeState
	scopes         map[*core.ScopeNode]*scopeInfo
	hydrating      bool   // initial render claims server-rendered nodes
	hydrateDynamic bool   // within a Dynamic subtree: re-apply values so client wins
	currentNS      string // XML namespace inherited by the subtree being rendered (SVG)
	muts           *[]core.Mutation

	// Recovery bookkeeping. While a walk that may be abandoned is in progress
	// (an error boundary's child, a scope re-render), every scope subscribed or
	// adopted is recorded here so a panic can release the ones the failed walk
	// touched. Truncated once the outermost such walk ends.
	recoveryDepth  int
	recoveryScopes []*core.ScopeNode
}

// SetHydrating puts the renderer into hydration mode for the next Render: it
// claims the server-rendered DOM (MutHydrate) and skips the create/attribute/
// property/append/initial-bind mutations the SSR already applied, while still
// registering handlers and bindings. It relies on SSR/DOM node-id parity
// (see TestSSRDOMIDParity); a claim that finds no server node falls back to
// creating one (logged) on the JS side. The flag is cleared after Render, so
// subsequent signal-driven re-renders emit normal mutations.
func (r *DOMRenderer) SetHydrating(h bool) { r.hydrating = h }

func New() *DOMRenderer {
	sched := core.NewScheduler()
	return &DOMRenderer{
		Walker:    *walker.New(),
		Scheduler: sched,
		Bindings:  runtime.NewBindingRegistry(sched),
		Registry:  NewNodeRegistry(),
		scopes:    map[*core.ScopeNode]*scopeInfo{},
	}
}

func (r *DOMRenderer) Reset() {
	r.Scheduler = core.NewScheduler()
	r.Walker.Reset()
	r.Bindings = runtime.NewBindingRegistry(r.Scheduler)
	r.Registry = NewNodeRegistry()
	r.scopes = map[*core.ScopeNode]*scopeInfo{}
}

var _ walker.Visitor = (*DOMRenderer)(nil)

func (r *DOMRenderer) Render(n core.Node) ([]core.Mutation, int) {
	// Hydration is a property of this one initial render; re-renders are normal.
	hydrating := r.hydrating
	defer func() { r.hydrating = false }()

	r.parentStack = nil
	r.scopeStack = nil
	r.currentNS = ""
	r.hydrateDynamic = false

	// Effect first-runs wait on this renderer's scheduler for the DOM.
	defer runtime.SetEffectQueue(runtime.SetEffectQueue(r.Scheduler.QueueEffect))

	var muts []core.Mutation
	r.muts = &muts
	n = core.FlatTree(n)
	rootID := r.Walker.Walk(n, r)
	r.muts = nil
	if rootID != 0 && !hydrating {
		// When hydrating, the root is already attached to #root by the server.
		// Fragments and scopes attach their own content to the root container.
		muts = append(muts, core.Mutation{
			Type: core.MutAppendChild, NodeID: 0, ChildID: rootID,
		})
	}
	return muts, firstRoot(n)
}

// renderFresh walks n as a brand-new subtree inside a diff. Its top-level
// nodes are left unattached — the caller places rootIDs(n) — while everything
// below them is attached as usual.
func (r *DOMRenderer) renderFresh(n core.Node, parentID int, muts *[]core.Mutation) {
	savedMuts, savedFresh := r.muts, r.freshParent
	r.muts, r.freshParent = muts, parentID
	r.parentStack = append(r.parentStack, detached)
	r.Walker.Walk(n, r)
	r.parentStack = r.parentStack[:len(r.parentStack)-1]
	r.muts, r.freshParent = savedMuts, savedFresh
}

// currentParent is where the walk attaches top-level nodes: the enclosing
// element, 0 for the root container, or `detached`.
func (r *DOMRenderer) currentParent() int {
	if len(r.parentStack) == 0 {
		return 0
	}
	return r.parentStack[len(r.parentStack)-1]
}

func (r *DOMRenderer) appendChild(parentID, childID int) {
	*r.muts = append(*r.muts, core.Mutation{Type: core.MutAppendChild, NodeID: parentID, ChildID: childID})
}

// ---------------------------------------------------------------------------
// walker.Visitor implementation
// ---------------------------------------------------------------------------

func (r *DOMRenderer) VisitElement(id int, el *core.ElementNode, walkChild func(core.Node) int) {
	if el == nil {
		return
	}
	if r.hydrating {
		*r.muts = append(*r.muts, core.Mutation{
			Type: core.MutHydrate, NodeID: id, Key: "tag", Value: el.Tag,
		})
		dynamic := r.hydrateDynamic || el.Dynamic
		if dynamic {
			r.emitAttrs(id, el)
		}
		for _, b := range el.Binds {
			r.Bindings.Bind(id, b)
			if dynamic {
				*r.muts = append(*r.muts, runtime.MutationForBind(id, b))
			}
		}
		for _, hd := range el.Handlers {
			r.Registry.RegisterHandler(id, hd.Event, hd.Fn, hd.Options)
		}
		prevDynamic := r.hydrateDynamic
		r.hydrateDynamic = dynamic
		r.parentStack = append(r.parentStack, id)
		for _, child := range el.Children {
			walkChild(child)
		}
		r.parentStack = r.parentStack[:len(r.parentStack)-1]
		r.hydrateDynamic = prevDynamic
		return
	}

	ns := el.Namespace
	if ns == "" {
		ns = r.currentNS
	}
	*r.muts = append(*r.muts, core.Mutation{
		Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: el.Tag, NS: ns,
	})

	r.emitAttrs(id, el)
	for _, b := range el.Binds {
		r.Bindings.Bind(id, b)
		*r.muts = append(*r.muts, runtime.MutationForBind(id, b))
	}
	for _, hd := range el.Handlers {
		r.Registry.RegisterHandler(id, hd.Event, hd.Fn, hd.Options)
	}
	r.parentStack = append(r.parentStack, id)
	prevNS := r.currentNS
	r.currentNS = ns
	for _, child := range el.Children {
		// Fragments and scopes attach their own content (and return 0).
		if childID := walkChild(child); childID != 0 {
			r.appendChild(id, childID)
		}
	}
	r.currentNS = prevNS
	r.parentStack = r.parentStack[:len(r.parentStack)-1]
}

// emitAttrs emits an element's static attributes and properties — skipping
// invalid attribute names (setAttribute would throw) and blocking script URLs,
// with the same rules as SSR.
func (r *DOMRenderer) emitAttrs(id int, el *core.ElementNode) {
	for _, a := range el.Attrs {
		if !runtime.ValidAttrName(a.Name) {
			runtime.WarnInvalidAttr(a.Name)
			continue
		}
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetAttribute, NodeID: id, Key: a.Name, Value: runtime.SafeURL(a.Name, a.Value)})
	}
	for _, p := range el.Props {
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: p.Name, Value: runtime.SafeURLValue(p.Name, p.Value)})
	}
}

func (r *DOMRenderer) VisitText(id int, tn *core.TextNode) {
	if tn == nil {
		return
	}
	if r.hydrating {
		*r.muts = append(*r.muts, core.Mutation{
			Type: core.MutHydrate, NodeID: id, Key: "tag", Value: "#text",
		})
		if sig, ok := tn.Value.(core.SignalAccessor); ok {
			r.Bindings.Bind(id, core.Bind{Target: core.BindToProp, Name: "textContent", Signal: sig})
			if r.hydrateDynamic {
				*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: runtime.TextValue(sig.Value())})
			}
		} else if r.hydrateDynamic {
			if s, ok := tn.Value.(string); ok {
				*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: s})
			}
		}
		return
	}
	*r.muts = append(*r.muts, core.Mutation{
		Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: "#text",
	})
	switch val := tn.Value.(type) {
	case string:
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: val})
	case core.SignalAccessor:
		r.Bindings.Bind(id, core.Bind{Target: core.BindToProp, Name: "textContent", Signal: val})
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: runtime.TextValue(val.Value())})
	}
}

func (r *DOMRenderer) VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int) {
	if fn == nil {
		return
	}
	parentID := r.currentParent()
	for _, child := range fn.Children {
		cid := walkChild(child)
		if cid != 0 && !r.hydrating && parentID != detached {
			r.appendChild(parentID, cid)
		}
	}
}

func (r *DOMRenderer) VisitMetadata(mn *core.MetadataNode, walkChild func(core.Node) int) {
	if mn == nil {
		return
	}
	if r.hydrating {
		// The server already rendered these tags into <head> (ssr collects
		// Metadata children into its head buffer). Walk the children so node-id
		// allocation stays in parity with SSR — otherwise the body nodes after
		// this Metadata would hydrate against the wrong server nodes — but
		// discard the mutations. Re-creating and appending head nodes here would
		// duplicate the server's tags (two <title>, duplicate <meta>, etc.),
		// since the server-rendered head nodes carry no ids to claim. Head
		// content is static (see ssr renderHeadChildren), so there are no live
		// bindings/handlers to preserve.
		saved := r.muts
		var discard []core.Mutation
		r.muts = &discard
		for _, child := range mn.Children {
			walkChild(child)
		}
		r.muts = saved
		return
	}
	// Fresh render (pure-client app, or a re-render): create the head nodes and
	// append them to <head>.
	r.renderInto("head", mn.Children, r.muts)
}

func (r *DOMRenderer) VisitPortal(pn *core.PortalNode, walkChild func(core.Node) int) {
	if pn == nil {
		return
	}
	r.renderInto(pn.Target, pn.Children, r.muts)
}

// renderInto renders children fresh and appends their roots to the container
// matched by selector (a portal target, or "head" for metadata). The content
// is client-side: never claimed, even during hydration, since the server does
// not render it in place.
func (r *DOMRenderer) renderInto(selector string, children []core.Node, muts *[]core.Mutation) {
	prevHydrating, prevNS := r.hydrating, r.currentNS
	r.hydrating, r.currentNS = false, ""
	for _, child := range children {
		r.renderFresh(child, 0, muts)
		for _, id := range rootIDs(child) {
			*muts = append(*muts, core.Mutation{
				Type: core.MutPortalAppend, NodeID: 0, ChildID: id, Value: selector,
			})
		}
	}
	r.hydrating, r.currentNS = prevHydrating, prevNS
}

func (r *DOMRenderer) VisitErrorBoundary(ebn *core.ErrorBoundaryNode, walkInner func() int) int {
	if ebn == nil {
		return 0
	}
	baseStack := len(r.parentStack)
	frameDepth := runtime.SaveFrameStack()
	savedNS, savedDyn := r.currentNS, r.hydrateDynamic
	mark := r.beginRecoverable()
	owner := runtime.PushOwner() // every frame the child creates hangs off this

	var childMuts []core.Mutation
	savedMuts := r.muts
	r.muts = &childMuts
	id, rec := func() (id int, rec any) {
		defer func() { rec = recover() }()
		id = walkInner()
		return
	}()
	r.muts = savedMuts
	if rec != nil {
		r.parentStack = r.parentStack[:baseStack]
		runtime.RestoreFrameStack(frameDepth)
		r.currentNS, r.hydrateDynamic = savedNS, savedDyn
		r.abandon(mark, owner)
		core.Log(core.LogRecoverErrorBoundary, "caught panic, rendering fallback", map[string]any{
			"phase": "render",
			"panic": rec,
		})
		fb := core.FlatTree(ebn.Fallback(rec))
		ebn.Prev = fb
		return r.Walker.Walk(fb, r)
	}
	runtime.PopComponent() // owner
	r.endRecoverable()
	*r.muts = append(*r.muts, childMuts...)
	ebn.Prev = ebn.Child
	return id
}

func (r *DOMRenderer) VisitComponentEnter(cn *core.ComponentNode) {
	// The walker handles PushComponent / PopComponent and Render.
}

func (r *DOMRenderer) VisitComponentLeave(cn *core.ComponentNode, innerID int) {
	if cn.Frame != nil {
		cn.Frame.RootNodeIDs = []int{innerID}
	}
}

func (r *DOMRenderer) VisitScopeEnter(sn *core.ScopeNode) {
	// seq is taken on entry, so an enclosing scope always orders before the
	// scopes inside it (Flush re-renders parent-before-child).
	r.scopeSeq++
	st := scopeState{seq: r.scopeSeq, parentID: r.currentParent()}
	if st.parentID == detached {
		st.parentID, st.detached = r.freshParent, true
	}
	r.scopeStack = append(r.scopeStack, st)
	// Hooks called directly by the scope's render function belong to it.
	runtime.PushOwner()
}

func (r *DOMRenderer) VisitScopeLeave(sn *core.ScopeNode, innerID int) int {
	owner := runtime.CurrentComponent()
	runtime.PopComponent()
	st := r.scopeStack[len(r.scopeStack)-1]
	r.scopeStack = r.scopeStack[:len(r.scopeStack)-1]

	if r.hydrating {
		*r.muts = append(*r.muts, core.Mutation{
			Type: core.MutHydrate, NodeID: sn.Anchor, Key: "tag", Value: "#comment",
		})
	} else {
		*r.muts = append(*r.muts, core.Mutation{
			Type: core.MutCreateElement, NodeID: sn.Anchor, Key: "tag", Value: "#comment",
		})
		// A scope attaches its own content and end anchor, in order, so the
		// anchor always sits right after the content.
		if !st.detached {
			if innerID != 0 {
				r.appendChild(st.parentID, innerID)
			}
			r.appendChild(st.parentID, sn.Anchor)
		}
	}

	info := &scopeInfo{seq: st.seq, parentID: st.parentID, owner: owner}
	if owner != nil {
		info.ownerParent = owner.Parent
	}
	r.scopes[sn] = info
	r.subscribeScope(sn, info)
	return 0
}

func (r *DOMRenderer) subscribeScope(sn *core.ScopeNode, info *scopeInfo) {
	if r.recoveryDepth > 0 {
		r.recoveryScopes = append(r.recoveryScopes, sn)
	}
	sn.Unsubs = make([]func(), len(sn.Deps))
	for i, dep := range sn.Deps {
		sn.Unsubs[i] = dep.Subscribe(func() {
			r.Scheduler.MarkDirty(sn, info.seq, func() { r.reRenderScope(sn) })
		})
	}
}

func unsubscribeScope(sn *core.ScopeNode) {
	for _, u := range sn.Unsubs {
		if u != nil {
			u()
		}
	}
	sn.Unsubs = nil
}

func (r *DOMRenderer) VisitRaw(id int, rn *core.RawNode) {
	if rn == nil {
		return
	}
	if r.hydrating {
		// SSR renders a raw node as a <div> wrapper; claim it as one.
		*r.muts = append(*r.muts, core.Mutation{
			Type: core.MutHydrate, NodeID: id, Key: "tag", Value: "div",
		})
		return
	}
	*r.muts = append(*r.muts, core.Mutation{
		Type: core.MutCreateElement, NodeID: id, Key: "tag", Value: "div",
	})
	*r.muts = append(*r.muts, core.Mutation{
		Type: core.MutSetProperty, NodeID: id, Key: "innerHTML", Value: rn.HTML,
	})
}

// ---------------------------------------------------------------------------
// Scope updates
// ---------------------------------------------------------------------------

// reRenderScope re-renders a dirty scope (called from Scheduler.Flush). A panic
// is contained: this scope's partial work is discarded and its DOM keeps its
// previous state, instead of aborting the whole flush and blanking the page.
func (r *DOMRenderer) reRenderScope(sn *core.ScopeNode) {
	info := r.scopes[sn]
	if info == nil {
		return // removed or adopted since it was marked dirty
	}
	defer runtime.SetEffectQueue(runtime.SetEffectQueue(r.Scheduler.QueueEffect))
	baseStack := len(r.parentStack)
	frameDepth := runtime.SaveFrameStack()
	mark := r.beginRecoverable()
	var muts []core.Mutation
	ok := func() (ok bool) {
		defer func() {
			if rec := recover(); rec != nil {
				r.parentStack = r.parentStack[:baseStack]
				runtime.RestoreFrameStack(frameDepth)
				r.abandon(mark, nil)
				core.Log(core.LogRecoverReRender, "panic during scope re-render; subtree kept previous state", map[string]any{
					"panic": rec,
				})
			}
		}()
		r.updateScope(sn, info, &muts)
		return true
	}()
	if !ok {
		return
	}
	r.endRecoverable()
	if len(muts) > 0 {
		r.Scheduler.Enqueue(muts...)
	}
}

// updateScope re-runs the scope's render and diffs the result against what it
// rendered last time, inserting anything new before the scope's anchor. The
// render runs under a fresh owner frame; once it succeeds, the previous
// render's owner — the Watch/UseEffect/OnMount calls that render made — is
// disposed, since this render re-created whatever it still needs.
func (r *DOMRenderer) updateScope(sn *core.ScopeNode, info *scopeInfo, muts *[]core.Mutation) {
	owner := runtime.NewOwner(info.ownerParent)
	runtime.PushFrame(owner)
	defer func() {
		if rec := recover(); rec != nil {
			// Components mounted by the failed diff hang off owner; release
			// them, then let the enclosing recovery handle the rest.
			hooks.DisposeFrameTree(owner)
			panic(rec)
		}
	}()
	newTree := core.FlatTree(sn.Render())
	r.parentStack = append(r.parentStack, info.parentID)
	r.diffChildren(info.parentID, contentList(sn.Prev), contentList(newTree), sn.Anchor, muts)
	r.parentStack = r.parentStack[:len(r.parentStack)-1]
	runtime.PopComponent()

	sn.Prev = newTree
	prev := info.owner
	info.owner = owner
	if prev != nil {
		hooks.RunFrameCleanup(prev)
	}
}

// adoptScope carries a mounted scope over to the new ScopeNode that a parent
// re-render produced in its place (the parent's render builds new nodes every
// time). The DOM, anchor, and renderer state move across and the content is
// diffed against the new render — so components inside survive, instead of
// the whole scope being torn down and rebuilt.
func (r *DOMRenderer) adoptScope(old, nw *core.ScopeNode, parentID int, muts *[]core.Mutation) {
	info := r.scopes[old]
	delete(r.scopes, old)
	unsubscribeScope(old)
	r.Scheduler.CancelDirty(old) // the adopting render supersedes a pending one
	nw.Prev, nw.Anchor = old.Prev, old.Anchor
	info.parentID = parentID
	r.scopes[nw] = info
	r.updateScope(nw, info, muts)
	r.subscribeScope(nw, info)
}

// beginRecoverable starts a walk that may be abandoned by a panic.
func (r *DOMRenderer) beginRecoverable() recoveryMark {
	r.recoveryDepth++
	return recoveryMark{startID: r.Walker.NextID(), scopes: len(r.recoveryScopes)}
}

func (r *DOMRenderer) endRecoverable() {
	r.recoveryDepth--
	if r.recoveryDepth == 0 {
		r.recoveryScopes = r.recoveryScopes[:0]
	}
}

type recoveryMark struct {
	startID int // first node id the walk could allocate
	scopes  int // recoveryScopes length when it began
}

// abandon releases everything a failed walk set up: scopes it subscribed or
// adopted, the frame tree under owner (effects, Watch, …), and the bindings
// and handlers of every node id it allocated — none of which ever reached the
// DOM, since the walk's mutations are discarded.
func (r *DOMRenderer) abandon(m recoveryMark, owner *core.ComponentFrame) {
	for _, sc := range r.recoveryScopes[m.scopes:] {
		unsubscribeScope(sc)
		r.Scheduler.CancelDirty(sc)
		if info := r.scopes[sc]; info != nil {
			if info.owner != nil {
				hooks.RunFrameCleanup(info.owner)
			}
			delete(r.scopes, sc)
		}
	}
	r.recoveryScopes = r.recoveryScopes[:m.scopes]
	r.recoveryDepth--
	if owner != nil {
		hooks.DisposeFrameTree(owner)
	}
	for id := m.startID; id < r.Walker.NextID(); id++ {
		r.Bindings.Unbind(id)
		r.Registry.Remove(id)
	}
}

// contentList is a scope's content as a child list.
func contentList(n core.Node) []core.Node {
	switch v := n.(type) {
	case nil:
		return nil
	case *core.FragmentNode:
		return v.Children
	default:
		return []core.Node{n}
	}
}

// ---------------------------------------------------------------------------
// Removal
// ---------------------------------------------------------------------------

// emitRemoveTree unmounts n. Go-side state is released for every node in the
// subtree, but the DOM needs only its roots removed — descendants go with
// them, and the JS runtime forgets the whole removed subtree — plus the
// content of any portal or head metadata inside n, which lives elsewhere.
func (r *DOMRenderer) emitRemoveTree(n core.Node, muts *[]core.Mutation) {
	r.disposeReactive(n)
	for _, id := range runtime.CollectIDs(n) {
		r.Bindings.Unbind(id)
		r.Registry.Remove(id)
	}
	var roots []int
	appendRoots(n, &roots)
	appendDetachedRoots(n, &roots)
	for _, id := range roots {
		*muts = append(*muts, core.Mutation{Type: core.MutRemoveNode, NodeID: id})
	}
}

// appendDetachedRoots adds the roots of portal and metadata content found
// anywhere inside n: those nodes are not DOM descendants of n's roots.
func appendDetachedRoots(n core.Node, out *[]int) {
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil {
			for _, c := range v.Children {
				appendDetachedRoots(c, out)
			}
		}
	case *core.FragmentNode:
		if v != nil {
			for _, c := range v.Children {
				appendDetachedRoots(c, out)
			}
		}
	case *core.ComponentNode:
		if v != nil {
			appendDetachedRoots(v.Prev, out)
		}
	case *core.ScopeNode:
		if v != nil {
			appendDetachedRoots(v.Prev, out)
		}
	case *core.ErrorBoundaryNode:
		if v != nil {
			appendDetachedRoots(v.Prev, out)
		}
	case *core.PortalNode:
		if v != nil {
			for _, c := range v.Children {
				appendRoots(c, out)
				appendDetachedRoots(c, out)
			}
		}
	case *core.MetadataNode:
		if v != nil {
			for _, c := range v.Children {
				appendRoots(c, out)
				appendDetachedRoots(c, out)
			}
		}
	}
}

// disposeReactive tears down the reactive resources of a subtree being
// removed: each component frame (its effects, and the Watch subscriptions
// registered on it) and each scope's dep-subscriptions and render owner. It
// walks the node tree so every frame is disposed exactly once
// (RunFrameCleanup does not recurse into child frames).
func (r *DOMRenderer) disposeReactive(n core.Node) {
	switch v := n.(type) {
	case *core.ElementNode:
		for _, c := range v.Children {
			r.disposeReactive(c)
		}
	case *core.FragmentNode:
		for _, c := range v.Children {
			r.disposeReactive(c)
		}
	case *core.MetadataNode:
		for _, c := range v.Children {
			r.disposeReactive(c)
		}
	case *core.PortalNode:
		for _, c := range v.Children {
			r.disposeReactive(c)
		}
	case *core.ErrorBoundaryNode:
		if v.Prev != nil {
			r.disposeReactive(v.Prev)
		}
	case *core.ComponentNode:
		if v.Prev != nil {
			r.disposeReactive(v.Prev)
		}
		if v.Frame != nil {
			hooks.RunFrameCleanup(v.Frame)
		}
	case *core.ScopeNode:
		unsubscribeScope(v)
		// Drop any pending re-render: this scope's subtree is being removed.
		r.Scheduler.CancelDirty(v)
		if info := r.scopes[v]; info != nil {
			if info.owner != nil {
				hooks.RunFrameCleanup(info.owner)
			}
			delete(r.scopes, v)
		}
		if v.Prev != nil {
			r.disposeReactive(v.Prev)
		}
	case *core.RawNode:
		// Leaf node — nothing to dispose.
	}
}

// ---------------------------------------------------------------------------
// Roots
// ---------------------------------------------------------------------------

// rootIDs lists the DOM nodes n contributes to its parent, in document order:
// an element/text/raw node itself; a fragment's, component's, or boundary's
// content; a scope's content followed by its anchor. Portals and metadata
// contribute nothing — their content lives in another container.
func rootIDs(n core.Node) []int {
	var out []int
	appendRoots(n, &out)
	return out
}

func appendRoots(n core.Node, out *[]int) {
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil && v.ID != 0 {
			*out = append(*out, v.ID)
		}
	case *core.TextNode:
		if v != nil && v.ID != 0 {
			*out = append(*out, v.ID)
		}
	case *core.RawNode:
		if v != nil && v.ID != 0 {
			*out = append(*out, v.ID)
		}
	case *core.FragmentNode:
		if v != nil {
			for _, c := range v.Children {
				appendRoots(c, out)
			}
		}
	case *core.ComponentNode:
		if v != nil {
			appendRoots(v.Prev, out)
		}
	case *core.ErrorBoundaryNode:
		if v != nil {
			appendRoots(v.Prev, out)
		}
	case *core.ScopeNode:
		if v != nil {
			appendRoots(v.Prev, out)
			if v.Anchor != 0 {
				*out = append(*out, v.Anchor)
			}
		}
	}
}

// firstRoot is rootIDs(n)[0] without allocating (0 when n has no roots).
func firstRoot(n core.Node) int {
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil {
			return v.ID
		}
	case *core.TextNode:
		if v != nil {
			return v.ID
		}
	case *core.RawNode:
		if v != nil {
			return v.ID
		}
	case *core.FragmentNode:
		if v != nil {
			for _, c := range v.Children {
				if id := firstRoot(c); id != 0 {
					return id
				}
			}
		}
	case *core.ComponentNode:
		if v != nil {
			return firstRoot(v.Prev)
		}
	case *core.ErrorBoundaryNode:
		if v != nil {
			return firstRoot(v.Prev)
		}
	case *core.ScopeNode:
		if v != nil {
			if id := firstRoot(v.Prev); id != 0 {
				return id
			}
			return v.Anchor
		}
	}
	return 0
}

// devMode reports whether dev-only diagnostics are on (?goowee-dev); a
// variable so tests can switch it.
var devMode = devtools.Enabled

var warnedPreserved = map[string]bool{}

// warnPreservedPlainComponent tells a developer (once per component name, in
// dev mode) that a component survived a re-render of its parent with the
// values it captured at mount — the classic stale-props surprise (#57).
func warnPreservedPlainComponent(name string) {
	if warnedPreserved[name] || !devMode() {
		return
	}
	warnedPreserved[name] = true
	core.Log(core.LogWarn, "component kept across a parent re-render: plain values it was called with are from its first render", map[string]any{
		"component": name,
		"fix":       "pass signals, use core.ComponentWithProps, or key it (core.SetKey) to remount on change",
	})
}

var warnedKeyTypes = map[string]bool{}

func warnUncomparableKey(k any) {
	t := fmt.Sprintf("%T", k)
	if warnedKeyTypes[t] {
		return
	}
	warnedKeyTypes[t] = true
	core.Log(core.LogWarn, "uncomparable key; row matched positionally instead", map[string]any{
		"keyType": t,
	})
}
