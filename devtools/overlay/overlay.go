// Package overlay is dev mode's in-page panel (#148): what goowee reports,
// which otherwise reaches only the console, where a developer sees it
// without opening the console.
//
//   - A badge in the corner counts errors (contained panics, the signal
//     cycle guard) and warnings.
//   - A new error opens the panel on its entry: the panic value, the
//     component, and the Go stack. It stays until dismissed.
//   - Warnings list their message and the fix each carries.
//   - The inspector shows the component tree, the signals with their values
//     and subscriber counts, and the change log, live while it is open,
//     filterable by signal name.
//
// The bridge mounts it as a second root in dev mode (?goowee-dev); a
// goowee_nodev build leaves it out with the rest of dev mode. It is written
// in goowee, and its own state is kept out of the change log
// (devtools.Silence).
package overlay

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// isError reports a log entry the overlay treats as an error.
func isError(e core.LogEntry) bool {
	return strings.HasPrefix(string(e.Kind), "recover.") || e.Kind == core.LogSignalCycle
}

func counts() (errs, warns int) {
	for _, e := range devtools.Logs() {
		if isError(e) {
			errs++
		} else if e.Kind == core.LogWarn {
			warns++
		}
	}
	return
}

func state[T any](v T) (*core.Signal[T], func(T)) {
	sig, set := hooks.UseState(v)
	devtools.Silence(sig)
	return sig, set
}

// View is the overlay.
func View() core.Node {
	return core.Component("GooweeDevOverlay", func() core.Node {
		open, setOpen := state(false)
		tab, setTab := state("errors")
		filter, setFilter := state("")
		tick, setTick := state(0)
		seen, setSeen := state(0) // errors the developer has dismissed
		logs := devtools.LogsVersion()

		// A new error opens the panel on it.
		hooks.Watch([]core.SignalAccessor{logs}, func() {
			if errs, _ := counts(); errs > seen.Peek() {
				setTab("errors")
				setOpen(true)
			}
		})
		// The inspector follows the app while it is open.
		hooks.UseEffect([]core.SignalAccessor{open, tab}, func() func() {
			if !open.Get() || tab.Get() != "inspector" {
				return nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				t := time.NewTicker(500 * time.Millisecond)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						core.Schedule(func() { setTick(tick.Peek() + 1) })
					}
				}
			}()
			return cancel
		})

		badge := core.Computed([]core.SignalAccessor{logs}, func() string {
			errs, warns := counts()
			return "goowee  ✕ " + strconv.Itoa(errs) + "  ⚠ " + strconv.Itoa(warns)
		})
		badgeStyle := core.Computed([]core.SignalAccessor{logs}, func() string {
			bg := "#1f2937"
			if errs, _ := counts(); errs > 0 {
				bg = "#b91c1c"
			}
			return badgeBase + "background:" + bg + ";"
		})
		dismiss := func() {
			errs, _ := counts()
			setSeen(errs)
			setOpen(false)
		}
		tabButton := func(name, label string) core.Node {
			style := core.Computed([]core.SignalAccessor{tab}, func() string {
				if tab.Get() == name {
					return tabBase + "background:#374151;color:#fff;"
				}
				return tabBase
			})
			return Button(Type("button"), Class("goowee-dev-tab"), Attr("data-tab", name), StyleS(style),
				OnClick(func() { setTab(name) }), Text(label))
		}
		return Div(ID("goowee-dev-overlay"), Style(rootStyle),
			Show(open, func() core.Node {
				return Div(ID("goowee-dev-panel"), Style(panelStyle),
					Div(Style("display:flex;gap:6px;align-items:center;margin-bottom:8px;"),
						tabButton("errors", "Errors"), tabButton("warnings", "Warnings"), tabButton("inspector", "Inspector"),
						Span(Style("flex:1")),
						Button(Type("button"), ID("goowee-dev-clear"), Style(tabBase), OnClick(func() { devtools.ClearLogs(); setSeen(0) }), Text("Clear")),
						Button(Type("button"), ID("goowee-dev-dismiss"), Style(tabBase), OnClick(dismiss), Text("Dismiss")),
					),
					hooks.UseScope(func() core.Node {
						switch tab.Get() {
						case "warnings":
							return entries(false)
						case "inspector":
							return inspector(filter, setFilter)
						}
						return entries(true)
					}, tab, logs, tick, filter),
				)
			}),
			Button(Type("button"), ID("goowee-dev-badge"), StyleS(badgeStyle), OnClick(func() { setOpen(!open.Peek()) }), TextS(badge)),
		)
	})
}

// entries lists the errors, or the warnings, newest first.
func entries(errors bool) core.Node {
	var items []core.Item
	all := devtools.Logs()
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if isError(e) != errors || (!errors && e.Kind != core.LogWarn) {
			continue
		}
		items = append(items, entry(e))
	}
	if len(items) == 0 {
		if errors {
			return P(Class("goowee-dev-empty"), Style(mutedStyle), Text("No errors."))
		}
		return P(Class("goowee-dev-empty"), Style(mutedStyle), Text("No warnings."))
	}
	return Div(items...)
}

func entry(e core.LogEntry) core.Node {
	items := []core.Item{Class("goowee-dev-entry"), Attr("data-kind", string(e.Kind)), Style(entryStyle),
		Div(Strong(Text(string(e.Kind))), Text(" "), Text(e.Message))}
	if p, ok := e.Fields["panic"]; ok {
		items = append(items, Pre(Class("goowee-dev-panic"), Style(preStyle), Text(core.FormatValue(p))))
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		if k != "panic" && k != "stack" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		style := mutedStyle
		if k == "fix" {
			style = "color:#a7f3d0;"
		}
		items = append(items, Div(Style(style), Text(k+": "+core.FormatValue(e.Fields[k]))))
	}
	if st, ok := e.Fields["stack"].(string); ok && st != "" {
		items = append(items, Details(Summary(Text("Go stack")), Pre(Style(preStyle), Text(st))))
	}
	return Div(items...)
}

// inspector shows the component tree, the signals and the change log.
func inspector(filter *core.Signal[string], setFilter func(string)) core.Node {
	snap := devtools.Snapshot()
	var tree []core.Item
	components(snap["tree"], 0, &tree)
	var sigs []core.Item
	if list, ok := snap["signals"].([]map[string]any); ok {
		sort.Slice(list, func(i, j int) bool { return toInt(list[i]["id"]) < toInt(list[j]["id"]) })
		for _, s := range list {
			name, _ := s["name"].(string)
			if name == "" {
				name = "#" + strconv.Itoa(toInt(s["id"]))
			}
			line := name + " = " + core.FormatValue(s["value"])
			if n, ok := s["subscribers"]; ok {
				line += "  (" + core.FormatValue(n) + " subscribers)"
			}
			sigs = append(sigs, Li(Class("goowee-dev-signal"), Text(line)))
		}
	}
	f := strings.ToLower(filter.Peek())
	var changes []core.Item
	all := devtools.Changes()
	for i := len(all) - 1; i >= 0 && len(changes) < 50; i-- {
		c := all[i]
		name := c.Name
		if name == "" {
			name = "#" + strconv.Itoa(c.Signal)
		}
		if f != "" && !strings.Contains(strings.ToLower(name), f) {
			continue
		}
		changes = append(changes, Li(Class("goowee-dev-change"), Text(name+": "+c.From+" → "+c.To+"  "+c.At)))
	}
	return Div(Class("goowee-dev-inspector"),
		H4(Style(headStyle), Text("Components")), Ul(append([]core.Item{Style(listStyle)}, tree...)...),
		H4(Style(headStyle), Text("Signals")), Ul(append([]core.Item{Style(listStyle)}, sigs...)...),
		H4(Style(headStyle), Text("Changes")),
		Input(ID("goowee-dev-filter"), Placeholder("filter by signal name"), Attr("value", filter.Peek()), Style(inputStyle),
			OnInputE(func(e core.EventData) { setFilter(e.Value()) })),
		Ul(append([]core.Item{Style(listStyle)}, changes...)...),
	)
}

// components collects the tree's components, indented by depth.
func components(n any, depth int, out *[]core.Item) {
	m, ok := n.(map[string]any)
	if !ok {
		return
	}
	next := depth
	if m["type"] == "component" {
		name, _ := m["name"].(string)
		*out = append(*out, Li(Class("goowee-dev-component"), Style("padding-left:"+strconv.Itoa(depth*12)+"px;"), Text(name)))
		next = depth + 1
	}
	if kids, ok := m["children"].([]any); ok {
		for _, k := range kids {
			components(k, next, out)
		}
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

const (
	rootStyle  = "position:fixed;right:12px;bottom:12px;z-index:2147483646;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;color:#e5e7eb;display:flex;flex-direction:column;align-items:flex-end;gap:8px;"
	badgeBase  = "border:0;border-radius:6px;padding:6px 10px;color:#fff;font:inherit;cursor:pointer;box-shadow:0 2px 8px rgba(0,0,0,.3);"
	panelStyle = "width:min(640px,calc(100vw - 24px));max-height:min(70vh,560px);overflow:auto;background:#111827;border:1px solid #374151;border-radius:8px;padding:10px;box-shadow:0 8px 24px rgba(0,0,0,.4);"
	tabBase    = "border:1px solid #374151;border-radius:4px;background:transparent;color:#d1d5db;font:inherit;padding:2px 8px;cursor:pointer;"
	entryStyle = "border-top:1px solid #374151;padding:6px 0;"
	preStyle   = "white-space:pre-wrap;margin:4px 0;color:#fecaca;max-height:240px;overflow:auto;"
	mutedStyle = "color:#9ca3af;"
	headStyle  = "margin:8px 0 4px;font:inherit;color:#9ca3af;"
	listStyle  = "list-style:none;margin:0;padding:0;"
	inputStyle = "font:inherit;width:100%;margin:2px 0 4px;background:#1f2937;color:#e5e7eb;border:1px solid #374151;border-radius:4px;padding:2px 6px;"
)
