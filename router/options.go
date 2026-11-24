package router

// ScrollOptions controls the browser work a router does after a navigation's
// frame is applied. The zero value is the browser-like default: scroll to the
// top on a push, to the fragment's element when the URL has one, and to the
// entry's saved position on back/forward; then move focus to the new page's
// heading and announce the document title.
type ScrollOptions struct {
	// Disabled skips all of the above for navigations made through this
	// router. Query-parameter updates never scroll or focus in any case.
	Disabled bool

	// Container is a CSS selector for the scrollable element instead of the
	// window (e.g. "#app-scroll"). The element must be in the document by the
	// time the navigation's frame is applied.
	Container string

	// FocusDisabled leaves focus where it is. Use it when the route swap is
	// small enough that moving focus would be surprising.
	FocusDisabled bool

	// AnnounceDisabled skips updating the aria-live route status region.
	AnnounceDisabled bool
}

// SetNavigationOptions replaces the browser-behaviour options for subsequent
// navigations. Call it once at startup; options read at navigation time, so
// changing them later affects only later navigations.
func (r *Router) SetNavigationOptions(opt ScrollOptions) { r.navOptions = opt }

// NavigationOptions returns the current navigation options.
func (r *Router) NavigationOptions() ScrollOptions { return r.navOptions }
