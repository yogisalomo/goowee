package router

import (
	"strings"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// navKind says what kind of history change led to a navigation effect. The
// browser-side implementation uses it to choose between scrolling to the top,
// scrolling to a fragment and restoring a saved position.
type navKind uint8

const (
	navPush navKind = iota
	navReplace
	navPop
)

// linkPath is the one derived signal every NavLink watches. Building one
// Computed per link would give each link its own subscription; this way all
// active states update from a single path change.
func (r *Router) linkPath() *core.Signal[string] {
	if r.linkPathSig == nil {
		r.linkPathSig = core.Computed([]core.SignalAccessor{r.Path}, func() string {
			return r.Path.Get()
		})
	}
	return r.linkPathSig
}

func navTargetPath(to string) string {
	rest, _, _ := strings.Cut(to, "#")
	path, _, _ := strings.Cut(rest, "?")
	return path
}

// pathActive implements segment-prefix matching: "/docs" is active on
// "/docs" and "/docs/install", but not "/docstring". "/" is active only on
// "/" so the home link doesn't stay lit on every page.
func pathActive(current, to string) bool {
	if to == "" || to == "/" {
		return current == "" || current == "/"
	}
	if current == to {
		return true
	}
	return strings.HasPrefix(current, strings.TrimSuffix(to, "/")+"/")
}

// NavLink is a Link that also follows the current route: it gets class
// "active" and aria-current="page" while its target matches, and an empty
// class plus aria-current="false" otherwise. Both attributes are bound, so a
// route change updates every NavLink from the router's shared path signal.
//
// NavLink owns the "class" and "aria-current" attributes — pass other
// attributes and children (ID, Title, an icon, …) in items.
func (r *Router) NavLink(to, text string, items ...core.Item) *core.ElementNode {
	return r.NavLinkTo(to, append([]core.Item{h.Text(text)}, items...)...)
}

// NavLinkTo is NavLink with arbitrary content.
func (r *Router) NavLinkTo(to string, items ...core.Item) *core.ElementNode {
	target := navTargetPath(to)
	path := r.linkPath()
	active := core.Computed([]core.SignalAccessor{path}, func() bool {
		return pathActive(path.Get(), target)
	})
	class := core.Computed([]core.SignalAccessor{active}, func() string {
		if active.Get() {
			return "active"
		}
		return ""
	})
	current := core.Computed([]core.SignalAccessor{active}, func() string {
		if active.Get() {
			return "page"
		}
		return "false"
	})
	all := []core.Item{
		h.BindAttr("class", class),
		h.BindAttr("aria-current", current),
	}
	return r.LinkTo(to, append(all, items...)...)
}

// BeforeLeave registers check for the calling component. If it returns false,
// Navigate, NavigateReplace, r.Link/r.NavLink clicks and r.Back/r.Forward are
// cancelled; the component is still mounted when check runs, so it can read
// its own state (with Peek — a guard is a snapshot decision, not a render).
// The guard is removed when the component unmounts, e.g. after it allows a
// route change away from itself.
//
// Call it from a component body, like a hook:
//
//	r.BeforeLeave(func(to string) bool { return !dirty.Peek() })
//
// The browser's back button and a tab close also ask the guards through
// beforeunload; a same-document popstate cannot be vetoed once it has happened.
func (r *Router) BeforeLeave(check func(to string) bool) {
	if check == nil {
		return
	}
	id := r.nextGuard
	r.nextGuard++
	hooks.OnMount(func() func() {
		if r.leaveGuards == nil {
			r.leaveGuards = make(map[int]func(string) bool)
		}
		r.leaveGuards[id] = check
		return func() { delete(r.leaveGuards, id) }
	})
}

func (r *Router) canLeave(to string) bool {
	for _, check := range r.leaveGuards {
		if !check(to) {
			return false
		}
	}
	return true
}
