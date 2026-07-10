package dom

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/runtime"
	"github.com/yogisalomo/goowee/internal/walker"
)

type scopeState struct {
	seq      int
	parentID int
}

type DOMRenderer struct {
	walker.Walker
	Bindings       *runtime.BindingRegistry
	Scheduler      *core.Scheduler
	Registry       *NodeRegistry
	parentStack    []int
	scopeSeq       int    // monotonic mount order; parents mount before children
	scopeStack     []scopeState
	hydrating      bool   // initial render claims server-rendered nodes
	hydrateDynamic bool   // within a Dynamic subtree: re-apply values so client wins
	currentNS      string // XML namespace inherited by the subtree being rendered (SVG)
	muts           *[]core.Mutation
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
	}
}

func (r *DOMRenderer) Reset() {
	r.Scheduler = core.NewScheduler()
	r.Walker.Reset()
	r.Bindings = runtime.NewBindingRegistry(r.Scheduler)
	r.Registry = NewNodeRegistry()
}

var _ walker.Visitor = (*DOMRenderer)(nil)

func (r *DOMRenderer) Render(n core.Node) ([]core.Mutation, int) {
	// Hydration is a property of this one initial render; re-renders are normal.
	hydrating := r.hydrating
	defer func() { r.hydrating = false }()

	r.parentStack = nil
	r.scopeSeq = 0
	r.scopeStack = nil
	r.currentNS = ""
	r.hydrateDynamic = false

	var muts []core.Mutation
	r.muts = &muts
	n = core.FlatTree(n)
	rootID := r.Walker.Walk(n, r)
	r.muts = nil
	if rootID != 0 && !hydrating {
		// When hydrating, the root is already attached to #root by the server.
		muts = append(muts, core.Mutation{
			Type: core.MutAppendChild, NodeID: 0, ChildID: rootID,
		})
	}
	// A top-level fragment attaches its own children to the root container
	// (node 0) inside renderNode; nothing more to do here.
	return muts, rootID
}

// renderNode delegatesto the shared walker for fresh subtree renders.
// For scope re-renders (where Prev != nil) it handles the diff inline
// since that path is DOM-specific and bypasses the walker.
func (r *DOMRenderer) renderNode(n core.Node, muts *[]core.Mutation) int {
	if sn, ok := n.(*core.ScopeNode); ok && sn.Prev != nil {
		newTree := core.FlatTree(sn.Render())
		var diffMuts []core.Mutation
		r.diffNode(sn.Prev, newTree, &diffMuts)
		r.Scheduler.Enqueue(diffMuts...)
		sn.Prev = newTree
		return rootIDFromTree(newTree)
	}
	savedMuts := r.muts
	r.muts = muts
	id := r.Walker.Walk(n, r)
	r.muts = savedMuts
	return id
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
			for _, a := range el.Attrs {
				*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetAttribute, NodeID: id, Key: a.Name, Value: a.Value})
			}
			for _, p := range el.Props {
				*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: p.Name, Value: p.Value})
			}
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
		for _, child := range el.Children {
			walkChild(child)
		}
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

	for _, a := range el.Attrs {
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetAttribute, NodeID: id, Key: a.Name, Value: a.Value})
	}
	for _, p := range el.Props {
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: p.Name, Value: p.Value})
	}
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
		childID := walkChild(child)
		if childID != 0 {
			*r.muts = append(*r.muts, core.Mutation{Type: core.MutAppendChild, NodeID: id, ChildID: childID})
		}
	}
	r.currentNS = prevNS
	r.parentStack = r.parentStack[:len(r.parentStack)-1]
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
				*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: sig.Value()})
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
		*r.muts = append(*r.muts, core.Mutation{Type: core.MutSetProperty, NodeID: id, Key: "textContent", Value: val.Value()})
	}
}

func (r *DOMRenderer) VisitFragment(fn *core.FragmentNode, walkChild func(core.Node) int) {
	if fn == nil {
		return
	}
	parentID := 0
	if len(r.parentStack) > 0 {
		parentID = r.parentStack[len(r.parentStack)-1]
	}
	for _, child := range fn.Children {
		cid := walkChild(child)
		if cid != 0 && !r.hydrating {
			*r.muts = append(*r.muts, core.Mutation{Type: core.MutAppendChild, NodeID: parentID, ChildID: cid})
		}
	}
}

func (r *DOMRenderer) VisitMetadata(mn *core.MetadataNode, walkChild func(core.Node) int) {
	if mn == nil {
		return
	}
	// Walk children normally and append them to <head>.
	prevHydrating, prevNS := r.hydrating, r.currentNS
	r.hydrating, r.currentNS = false, ""
	for _, child := range mn.Children {
		childID := walkChild(child)
		if childID != 0 {
			*r.muts = append(*r.muts, core.Mutation{
				Type: core.MutPortalAppend, NodeID: 0, ChildID: childID, Value: "head",
			})
		}
	}
	r.hydrating, r.currentNS = prevHydrating, prevNS
}

func (r *DOMRenderer) VisitPortal(pn *core.PortalNode, walkChild func(core.Node) int) {
	if pn == nil {
		return
	}
	prevHydrating, prevNS := r.hydrating, r.currentNS
	r.hydrating, r.currentNS = false, ""
	for _, child := range pn.Children {
		childID := walkChild(child)
		if childID != 0 {
			*r.muts = append(*r.muts, core.Mutation{
				Type: core.MutPortalAppend, NodeID: 0, ChildID: childID, Value: pn.Target,
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
		core.Log(core.LogRecoverErrorBoundary, "caught panic, rendering fallback", map[string]any{
			"phase": "render",
			"panic": rec,
		})
		fb := core.FlatTree(ebn.Fallback(rec))
		ebn.Prev = fb
		return r.renderNode(fb, r.muts)
	}
	*r.muts = append(*r.muts, childMuts...)
	ebn.Prev = ebn.Child
	return id
}

func (r *DOMRenderer) VisitComponentEnter(cn *core.ComponentNode) {
	// The walker handles PushComponent / PopComponent and Render.
	// We just track the frame after the walk.
}

func (r *DOMRenderer) VisitComponentLeave(cn *core.ComponentNode, innerID int) {
	if cn.Frame != nil {
		cn.Frame.RootNodeIDs = []int{innerID}
	}
}

func (r *DOMRenderer) VisitScopeEnter(sn *core.ScopeNode) {
	r.scopeSeq++
	seq := r.scopeSeq
	parentID := 0
	if len(r.parentStack) > 0 {
		parentID = r.parentStack[len(r.parentStack)-1]
	}
	r.scopeStack = append(r.scopeStack, scopeState{seq: seq, parentID: parentID})
}

func (r *DOMRenderer) VisitScopeLeave(sn *core.ScopeNode, innerID int) {
	if len(r.scopeStack) == 0 {
		return
	}
	state := r.scopeStack[len(r.scopeStack)-1]
	r.scopeStack = r.scopeStack[:len(r.scopeStack)-1]

	sn.Unsubs = make([]func(), len(sn.Deps))
	for i, dep := range sn.Deps {
		dep := dep
		sn.Unsubs[i] = dep.Subscribe(func() {
			r.Scheduler.MarkDirty(sn, state.seq, func() {
				r.reRenderScope(sn, state.parentID)
			})
		})
	}
}

func nodeID(n core.Node) int {
	if n == nil {
		return 0
	}
	switch v := n.(type) {
	case *core.ElementNode:
		return v.ID
	case *core.TextNode:
		return v.ID
	case *core.ComponentNode:
		return rootIDFromTree(v.Prev)
	case *core.ScopeNode:
		return rootIDFromTree(v.Prev)
	case *core.MetadataNode:
		return 0
	}
	return 0
}

func rootIDFromTree(n core.Node) int {
	switch v := n.(type) {
	case *core.ElementNode:
		return v.ID
	case *core.TextNode:
		return v.ID
	case *core.FragmentNode:
		if len(v.Children) > 0 {
			return rootIDFromTree(v.Children[0])
		}
	case *core.ComponentNode:
		return rootIDFromTree(v.Prev)
	case *core.ScopeNode:
		return rootIDFromTree(v.Prev)
	}
	return 0
}

func rootTypeChanged(old, new core.Node) bool {
	if old == nil || new == nil {
		return old != new
	}
	switch a := old.(type) {
	case *core.ElementNode:
		b, ok := new.(*core.ElementNode)
		return !ok || a.Tag != b.Tag
	case *core.TextNode:
		_, ok := new.(*core.TextNode)
		return !ok
	case *core.FragmentNode:
		_, ok := new.(*core.FragmentNode)
		return !ok
	case *core.ScopeNode:
		_, ok := new.(*core.ScopeNode)
		return !ok
	case *core.ComponentNode:
		b, ok := new.(*core.ComponentNode)
		return !ok || a.Name != b.Name
	}
	return true
}

func (r *DOMRenderer) reRenderScope(s *core.ScopeNode, parentID int) {
	// Contain a panic during re-render: mutations are built into the local
	// `muts` and only enqueued at the end, so a panic here discards this
	// scope's partial work (its DOM keeps its previous state) instead of
	// aborting the whole flush and blanking the page. Restore parentStack,
	// which a mid-render panic would leave unbalanced.
	baseStack := len(r.parentStack)
	frameDepth := runtime.SaveFrameStack()
	defer func() {
		if rec := recover(); rec != nil {
			r.parentStack = r.parentStack[:baseStack]
			runtime.RestoreFrameStack(frameDepth)
			core.Log(core.LogRecoverReRender, "panic during scope re-render; subtree kept previous state", map[string]any{
				"panic": rec,
			})
		}
	}()

	newTree := core.FlatTree(s.Render())

	var muts []core.Mutation

	// Push the scope's parent so any multi-root (fragment) content mounted
	// during this diff attaches under the real parent, not the root.
	r.parentStack = append(r.parentStack, parentID)

	oldFrag, oldIsFrag := s.Prev.(*core.FragmentNode)
	newFrag, newIsFrag := newTree.(*core.FragmentNode)

	switch {
	case rootTypeChanged(s.Prev, newTree):
		newRootID := r.renderNode(newTree, &muts)
		oldRootID := rootIDFromTree(s.Prev)
		if newRootID != 0 {
			switch {
			case parentID != 0 && oldRootID != 0:
				// Place the new root where the old one sits, then remove old.
				muts = append(muts, core.Mutation{
					Type: core.MutInsertBefore, NodeID: parentID,
					ChildID: newRootID, RefID: oldRootID,
				})
			case parentID != 0:
				// Old root had no DOM node (e.g. the scope was showing an
				// empty fragment), so there is no sibling to anchor against.
				// Append to the parent — correct for a trailing/only child.
				muts = append(muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: parentID, ChildID: newRootID,
				})
			default:
				muts = append(muts, core.Mutation{
					Type: core.MutAppendChild, NodeID: 0, ChildID: newRootID,
				})
			}
		}
		if s.Prev != nil {
			r.emitRemoveTree(s.Prev, &muts)
		}
	case oldIsFrag && newIsFrag:
		// Multi-root list content (e.g. For): reconcile the children directly
		// against the scope's real parent so keyed reorders, inserts, and
		// removes land in the right place — this is what makes keyed lists
		// (of elements or components) work when they aren't wrapped in an
		// element of their own.
		r.diffChildren(parentID, oldFrag.Children, newFrag.Children, &muts)
	default:
		r.diffNode(s.Prev, newTree, &muts)
	}

	r.parentStack = r.parentStack[:len(r.parentStack)-1]

	if len(muts) > 0 {
		r.Scheduler.Enqueue(muts...)
	}

	s.Prev = newTree
}

func (r *DOMRenderer) diffNode(oldNode, newNode core.Node, muts *[]core.Mutation) int {
	if oldNode == nil && newNode == nil {
		return 0
	}
	if oldNode == nil {
		return r.renderNode(newNode, muts)
	}
	if newNode == nil {
		r.emitRemoveTree(oldNode, muts)
		return 0
	}

	switch old := oldNode.(type) {
	case *core.ElementNode:
		new, ok := newNode.(*core.ElementNode)
		if !ok || old.Tag != new.Tag {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		new.ID = old.ID

		oldAttrs := map[string]string{}
		for _, a := range old.Attrs {
			oldAttrs[a.Name] = a.Value
		}
		for _, a := range new.Attrs {
			if ov, ok := oldAttrs[a.Name]; !ok || ov != a.Value {
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetAttribute, NodeID: old.ID, Key: a.Name, Value: a.Value,
				})
			}
			delete(oldAttrs, a.Name)
		}
		for name := range oldAttrs {
			*muts = append(*muts, core.Mutation{
				Type: core.MutRemoveAttribute, NodeID: old.ID, Key: name,
			})
		}

		oldProps := map[string]any{}
		for _, p := range old.Props {
			oldProps[p.Name] = p.Value
		}
		for _, p := range new.Props {
			if ov, ok := oldProps[p.Name]; !ok || ov != p.Value {
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: p.Name, Value: p.Value,
				})
			}
			delete(oldProps, p.Name)
		}
		for name, ov := range oldProps {
			var zero any = ""
			if _, isBool := ov.(bool); isBool {
				zero = false
			}
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: name, Value: zero,
			})
		}

		r.Bindings.Unbind(old.ID)
		for _, b := range new.Binds {
			r.Bindings.Bind(old.ID, b)
			*muts = append(*muts, runtime.MutationForBind(old.ID, b))
		}

		newEvents := map[string]bool{}
		for _, hd := range new.Handlers {
			r.Registry.RegisterHandler(old.ID, hd.Event, hd.Fn, hd.Options)
			newEvents[hd.Event] = true
		}
		for _, hd := range old.Handlers {
			if !newEvents[hd.Event] {
				r.Registry.RemoveHandler(old.ID, hd.Event)
			}
		}

		r.diffChildren(old.ID, old.Children, new.Children, muts)
		return old.ID

	case *core.TextNode:
		new, ok := newNode.(*core.TextNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		new.ID = old.ID

		oldStr, oldIsStr := old.Value.(string)
		newStr, newIsStr := new.Value.(string)
		if oldIsStr && newIsStr && oldStr != newStr {
			*muts = append(*muts, core.Mutation{
				Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
			})
			return old.ID
		}

		if oldIsStr != newIsStr || (!oldIsStr && !newIsStr) {
			r.Bindings.Unbind(old.ID)
			if newSig, ok := new.Value.(core.SignalAccessor); ok {
				r.Bindings.Bind(old.ID, core.Bind{
					Target: core.BindToProp, Name: "textContent", Signal: newSig,
				})
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent",
					Value: newSig.Value(),
				})
			} else if newStr, ok := new.Value.(string); ok {
				*muts = append(*muts, core.Mutation{
					Type: core.MutSetProperty, NodeID: old.ID, Key: "textContent", Value: newStr,
				})
			}
		}
		return old.ID

	case *core.FragmentNode:
		new, ok := newNode.(*core.FragmentNode)
		if !ok {
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		r.diffChildren(0, old.Children, new.Children, muts)
		return 0

	case *core.ComponentNode:
		new, ok := newNode.(*core.ComponentNode)
		if !ok || old.Name != new.Name {
			// Different component (or no longer a component): unmount + mount.
			r.emitRemoveTree(old, muts)
			return r.renderNode(newNode, muts)
		}
		// Same component: preserve it. Setup ran once at mount and the output
		// is a stable, self-updating subtree (bindings and inner scopes react
		// on their own), so we keep the frame and DOM untouched — this is what
		// gives components stable identity and state across scope re-renders.
		new.Frame = old.Frame
		new.Prev = old.Prev
		return rootIDFromTree(old.Prev)

	case *core.ScopeNode:
		if old == newNode {
			return rootIDFromTree(old.Prev)
		}
		if old.Prev != nil {
			r.emitRemoveTree(old.Prev, muts)
		}
		for _, unsub := range old.Unsubs {
			if unsub != nil {
				unsub()
			}
		}
		old.Unsubs = nil
		newScope, ok := newNode.(*core.ScopeNode)
		if !ok {
			return 0
		}
		return r.renderNode(newScope, muts)

	case *core.MetadataNode:
		// Like portals: tear down old head content and render new.
		for _, c := range old.Children {
			r.emitRemoveTree(c, muts)
		}
		if nm, ok := newNode.(*core.MetadataNode); ok {
			for _, child := range nm.Children {
				childID := r.renderNode(child, muts)
				if childID != 0 {
					*muts = append(*muts, core.Mutation{
						Type: core.MutPortalAppend, NodeID: 0, ChildID: childID, Value: "head",
					})
				}
			}
		}
		return 0

	case *core.PortalNode:
		// The target is a selector, not a node we can reconcile against, so
		// tear down the old portal content and render the new (ADR-017). Keep
		// portal state in signals outside the portal.
		for _, c := range old.Children {
			r.emitRemoveTree(c, muts)
		}
		if np, ok := newNode.(*core.PortalNode); ok {
			r.renderPortal(np, muts)
		}
		return 0

	case *core.ErrorBoundaryNode:
		nb, ok := newNode.(*core.ErrorBoundaryNode)
		if !ok {
			if old.Prev != nil {
				r.emitRemoveTree(old.Prev, muts)
			}
			return r.renderNode(newNode, muts)
		}
		return r.diffBoundary(old, nb, muts)
	}
	return 0
}

// diffBoundary updates a boundary on re-render. It diffs the previously rendered
// subtree against the new child (preserving child state); if that panics, it
// keeps the previous DOM and logs (update-time panics are contained, not
// swapped to the fallback). When the previous render was the fallback, a
// successful diff to the new child restores it.
func (r *DOMRenderer) diffBoundary(old, nb *core.ErrorBoundaryNode, muts *[]core.Mutation) int {
	baseStack := len(r.parentStack)
	frameDepth := runtime.SaveFrameStack()
	savedNS, savedDyn := r.currentNS, r.hydrateDynamic

	var childMuts []core.Mutation
	id, rec := r.tryDiffNode(old.Prev, nb.Child, &childMuts)
	if rec != nil {
		r.parentStack = r.parentStack[:baseStack]
		runtime.RestoreFrameStack(frameDepth)
		r.currentNS, r.hydrateDynamic = savedNS, savedDyn
		core.Log(core.LogRecoverErrorBoundary, "panic during boundary update; subtree kept previous state", map[string]any{
			"phase": "update",
			"panic": rec,
		})
		nb.Prev = old.Prev
		return nodeID(old.Prev)
	}
	*muts = append(*muts, childMuts...)
	nb.Prev = nb.Child
	return id
}

func (r *DOMRenderer) tryDiffNode(old, new core.Node, muts *[]core.Mutation) (id int, rec any) {
	defer func() { rec = recover() }()
	id = r.diffNode(old, new, muts)
	return
}

// renderPortal renders a portal's children into their target container. The
// content is client-side: rendered fresh (never claimed) even during
// hydration, since the server does not render portals.
func (r *DOMRenderer) renderPortal(p *core.PortalNode, muts *[]core.Mutation) {
	prevHydrating, prevNS := r.hydrating, r.currentNS
	r.hydrating, r.currentNS = false, ""
	for _, child := range p.Children {
		childID := r.renderNode(child, muts)
		if childID != 0 {
			*muts = append(*muts, core.Mutation{
				Type: core.MutPortalAppend, NodeID: 0, ChildID: childID, Value: p.Target,
			})
		}
	}
	r.hydrating, r.currentNS = prevHydrating, prevNS
}

func (r *DOMRenderer) diffChildren(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	hasKeys := hasAnyKey(old) || hasAnyKey(new)
	if hasKeys {
		r.diffChildrenKeyed(parentID, old, new, muts)
		return
	}
	r.diffChildrenPositional(parentID, old, new, muts)
}

// keyOf returns a node's reconciliation key, if it carries one (elements and
// components). Keyed matching therefore works uniformly whether list rows are
// elements or components.
func keyOf(n core.Node) (any, bool) {
	switch v := n.(type) {
	case *core.ElementNode:
		if v != nil && v.Key != nil {
			return v.Key, true
		}
	case *core.ComponentNode:
		if v != nil && v.Key != nil {
			return v.Key, true
		}
	}
	return nil, false
}

func hasAnyKey(nodes []core.Node) bool {
	for _, n := range nodes {
		if _, ok := keyOf(n); ok {
			return true
		}
	}
	return false
}

func (r *DOMRenderer) diffChildrenPositional(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	var created []bool
	var ids []int

	maxLen := len(old)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	// Pass 1: pair positionally. Surplus old children (i >= len(new)) are
	// paired with nil, which makes diffNode remove them — so no separate
	// removal pass is needed. A node is "created" when diffNode had to
	// render it fresh (old was nil, or a type/tag mismatch forced a swap).
	for i := 0; i < maxLen; i++ {
		var oldChild, newChild core.Node
		if i < len(old) {
			oldChild = old[i]
		}
		if i < len(new) {
			newChild = new[i]
		}

		oldID := nodeID(oldChild)
		childID := r.diffNode(oldChild, newChild, muts)
		created = append(created, newChild != nil && childID != oldID)
		ids = append(ids, childID)
	}

	// Pass 2: place created children. Reused children keep their position
	// under positional matching, so they never move.
	refID := 0
	for i := len(new) - 1; i >= 0; i-- {
		if created[i] && ids[i] != 0 {
			*muts = append(*muts, core.Mutation{
				Type: core.MutInsertBefore, NodeID: parentID,
				ChildID: ids[i], RefID: refID,
			})
		}
		if ids[i] != 0 {
			refID = ids[i]
		}
	}
}

func (r *DOMRenderer) diffChildrenKeyed(parentID int, old, new []core.Node, muts *[]core.Mutation) {
	oldByKey := map[any]int{}
	oldUnkeyed := []int{}
	for i, n := range old {
		if k, ok := keyOf(n); ok {
			if _, dup := oldByKey[k]; dup {
				core.Log(core.LogWarn, "duplicate key in keyed children; treating extra as unkeyed", map[string]any{
					"key": k,
				})
				continue
			}
			oldByKey[k] = i
		} else {
			oldUnkeyed = append(oldUnkeyed, i)
		}
	}

	paired := make([]bool, len(old))
	oldIndexForNew := make([]int, len(new))
	newIDs := make([]int, len(new))

	unkeyedIdx := 0
	for i, n := range new {
		oldIndexForNew[i] = -1
		if k, ok := keyOf(n); ok {
			if oldIdx, ok := oldByKey[k]; ok {
				if paired[oldIdx] {
					core.Log(core.LogWarn, "duplicate key in keyed children; treating extra as unkeyed", map[string]any{
					"key": k,
				})
					continue
				}
				paired[oldIdx] = true
				oldIndexForNew[i] = oldIdx
			}
		} else {
			for unkeyedIdx < len(oldUnkeyed) {
				oi := oldUnkeyed[unkeyedIdx]
				unkeyedIdx++
				if !paired[oi] && typeCompatible(old[oi], n) {
					paired[oi] = true
					oldIndexForNew[i] = oi
					break
				}
			}
		}
	}

	for i := len(new) - 1; i >= 0; i-- {
		var oldChild core.Node
		if oldIndexForNew[i] >= 0 {
			oldChild = old[oldIndexForNew[i]]
		}
		newIDs[i] = r.diffNode(oldChild, new[i], muts)
	}

	for i, n := range old {
		if !paired[i] {
			r.emitRemoveTree(n, muts)
		}
	}

	// LIS optimisation: compute which existing elements already appear in the
	// correct relative order and skip DOM moves for them. Only elements NOT in
	// the LIS (plus new elements) need insertBefore mutations.
	needsMove := computeNeedsMove(oldIndexForNew)

	refID := 0
	for i := len(new) - 1; i >= 0; i-- {
		if newIDs[i] != 0 {
			if needsMove[i] {
				*muts = append(*muts, core.Mutation{
					Type: core.MutInsertBefore, NodeID: parentID,
					ChildID: newIDs[i], RefID: refID,
				})
			}
			refID = newIDs[i]
		}
	}
}

// computeNeedsMove returns a boolean slice parallel to oldIndexForNew where
// true means the element at that new position needs a DOM mutation. New
// elements (oldIdx == -1) always need a move; existing elements whose old
// index is NOT part of the longest increasing subsequence (by old index,
// ordered by new position) need a move; the rest stay in place.
func computeNeedsMove(oldIndexForNew []int) []bool {
	n := len(oldIndexForNew)
	needsMove := make([]bool, n)

	// Extract kept (existing) old indices in new-list order.
	kept := make([]int, 0, n)
	keptPos := make([]int, 0, n)
	for i, oldIdx := range oldIndexForNew {
		if oldIdx >= 0 {
			kept = append(kept, oldIdx)
			keptPos = append(keptPos, i)
		} else {
			needsMove[i] = true // new element, always needs a move
		}
	}
	if len(kept) == 0 {
		return needsMove
	}

	// LIS over kept old indices: which are already in the correct order?
	inLIS := lis(kept)

	for i, in := range inLIS {
		if !in {
			needsMove[keptPos[i]] = true
		}
	}
	return needsMove
}

// lis computes the longest increasing subsequence on arr (O(n²) DP).
// Returns a boolean slice where true means the element at that index is
// part of one longest increasing subsequence.
func lis(arr []int) []bool {
	n := len(arr)
	if n == 0 {
		return nil
	}

	dp := make([]int, n)
	maxLen := 0
	for i := 0; i < n; i++ {
		dp[i] = 1
		for j := 0; j < i; j++ {
			if arr[j] < arr[i] && dp[j]+1 > dp[i] {
				dp[i] = dp[j] + 1
			}
		}
		if dp[i] > maxLen {
			maxLen = dp[i]
		}
	}

	inLIS := make([]bool, n)
	if maxLen == 0 {
		return inLIS
	}

	// Reconstruct from the right: pick the rightmost element with dp = target
	// and value < previously picked value (greedy backwards walk).
	target := maxLen
	prev := int(^uint(0) >> 1) // max int
	for i := n - 1; i >= 0; i-- {
		if dp[i] == target && arr[i] < prev {
			inLIS[i] = true
			target--
			prev = arr[i]
		}
	}
	return inLIS
}

func typeCompatible(a, b core.Node) bool {
	if a == nil || b == nil {
		return false
	}
	switch va := a.(type) {
	case *core.ElementNode:
		vb, ok := b.(*core.ElementNode)
		return ok && va.Tag == vb.Tag
	case *core.TextNode:
		_, ok := b.(*core.TextNode)
		return ok
	case *core.FragmentNode:
		_, ok := b.(*core.FragmentNode)
		return ok
	case *core.ScopeNode:
		_, ok := b.(*core.ScopeNode)
		return ok
	case *core.ComponentNode:
		vb, ok := b.(*core.ComponentNode)
		return ok && va.Name == vb.Name
	}
	return false
}

func (r *DOMRenderer) emitRemoveTree(n core.Node, muts *[]core.Mutation) {
	r.disposeReactive(n)
	ids := runtime.CollectIDs(n)
	for _, id := range ids {
		r.Bindings.Unbind(id)
		r.Registry.Remove(id)
		*muts = append(*muts, core.Mutation{
			Type: core.MutRemoveNode, NodeID: id,
		})
	}
}

// disposeReactive tears down the reactive resources of a subtree being
// removed: each component frame (its effects, and the Computed/Watch
// subscriptions registered on it) and each scope's dep-subscriptions. It
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
		for _, u := range v.Unsubs {
			if u != nil {
				u()
			}
		}
		v.Unsubs = nil
		// Drop any pending re-render: this scope's subtree is being removed.
		r.Scheduler.CancelDirty(v)
		if v.Prev != nil {
			r.disposeReactive(v.Prev)
		}
	}
}
