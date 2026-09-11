package app

import (
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
)

// lessonLayout wraps an example in a consistent tutorial page: the concept, a
// live demo card, the code that produces it, and "how it works" notes.
func lessonLayout(r *router.Router, title, intro string, demo core.Node, filename, code string, notes ...core.Node) core.Node {
	body := []core.Node{
		A(Class("backlink"), Href("/tutorial"),
			OnClickE(func(core.EventData) { r.Navigate("/tutorial") }, PreventDefault()),
			Text("← Back to tutorial")),
		H1(Text(title)),
		P(Class("lead"), Text(intro)),
		Div(Class("demo-card"),
			Div(Class("demo-card-bar"), Span(Class("dot")), Span(Class("dot")), Span(Class("dot")),
				Span(Class("demo-card-tag"), Text("live"))),
			Div(Class("demo-card-body"), demo),
		),
	}
	if code != "" {
		body = append(body, codeBlock(filename, code))
	}
	body = append(body, notes...)
	return Div(Class("page lesson"), Group(Nodes(body)...))
}

// codeBlock is a dark, labeled code panel (same look as the landing).
func codeBlock(filename, code string) core.Node {
	return Div(Class("panel"),
		Div(Class("panel-bar"),
			Span(Class("dot")), Span(Class("dot")), Span(Class("dot")),
			Span(Style("margin-left:auto"), Text(filename)),
		),
		Pre(Class("code"), Text(code)),
	)
}

// howItWorks renders a titled bullet list of explanation/steps.
func howItWorks(items ...string) core.Node {
	lis := make([]core.Node, len(items))
	for i, it := range items {
		lis[i] = Li(Text(it))
	}
	return Div(H3(Text("How it works")), Ul(Class("notes"), Group(Nodes(lis)...)))
}

// ---- code snippets shown on the lesson pages (essence, not the full source) ----

const counterCode = `count, setCount := hooks.UseState(0)     // a signal + its setter
show, setShow := hooks.UseState(true)

return Div(
    Button(OnClick(func() { setCount(count.Get() + 1) }),
        Textf("Count: %d", count)),          // reactive: only this text updates
    Button(OnClick(func() { setShow(!show.Get()) }), Text("Toggle")),
    ShowElse(show,
        func() core.Node { return P(Text("Hello!")) },
        func() core.Node { return P(Text("(hidden)")) }),
)`

const formCode = `name, setName := hooks.UseState("")
entries, setEntries := hooks.UseState([]entry{})

return Form(
    OnSubmit(func(vals map[string]string) {          // vals = named field values
        setEntries(append(entries.Get(), entry{name: vals["name"]}))
        setName("")                                   // clear the bound input
    }),
    Input(Type("text"), Name("name"), RefTo(nameRef), BindValue(name)), // two-way binding
    Input(Type("file"), OnChangeE(func(e core.EventData) {  // e.Files(): name, size, type
        f := e.Files()[0]
        go func() {                                       // bytes are read off-loop
            data, err := f.Bytes()
            core.Schedule(func() { setPicked(f.Name, data, err) })
        }()
    })),
    Button(Text("Submit")),
    For(entries, func(e entry) int { return e.id },     // keyed list
        func(e entry) core.Node { return Li(Text(e.name)) }),
)
// Elsewhere: nameRef.Focus() to focus the field, or read a value back —
// nameRef.Get("offsetWidth", func(v any) { w, _ := v.(float64); setWidth(int(w)) })`

const todosCode = `todos, setTodos := hooks.UseState(initial)   // 100 rows

// VirtualList renders only the rows currently on screen, so a long
// list stays fast. Each row is keyed by index for stable reuse.
VirtualList(todos, 48 /* row height px */, func(i int, t todo) core.Node {
    return Li(
        Input(Type("checkbox"), Checked(t.completed), OnChange(toggle(t))),
        Span(Text(t.text)),
        Button(OnClick(remove(t)), Text("✕")),
    )
}, VirtualListHeight(300))`

const dashboardCode = `data, setData := hooks.UseState(rows)
selected, setSelected := hooks.UseState(-1)

// Computed derives a value from signals and recomputes only when a
// dependency changes — never on unrelated updates.
total := core.Computed([]core.SignalAccessor{data}, func() string {
    sum := 0
    for _, r := range data.Get() { sum += r.Value }
    return fmt.Sprintf("Total: %d", sum)
})
// A row's OnClick(func(){ setSelected(i) }) makes the "selected" text recompute.`

const stopwatchCode = `elapsed, setElapsed := hooks.UseState(0)

hooks.OnMount(func() func() {                 // runs once, on the client
    ticker := time.NewTicker(100 * time.Millisecond)
    stop := make(chan struct{})
    go func() {
        for {
            select {
            case <-stop:
                return
            case <-ticker.C:
                // The ticker runs off the render loop, so hand the
                // update to the scheduler instead of setting directly.
                core.Schedule(func() { setElapsed(elapsed.Get() + 1) })
            }
        }
    }()
    return func() { close(stop); ticker.Stop() } // cleanup on unmount
})`

const asyncCode = `// UseResource runs fetch in a goroutine and exposes its state as
// signals: Data, Loading, Err (+ Refetch). Applied safely on the loop.
res := hooks.UseResource(nil, func() (string, error) {
    return loadFromServer() // your blocking call, in a goroutine
})

return ShowElse(res.Loading,
    func() core.Node { return P(Text("Loading…")) },
    func() core.Node { return P(TextS(res.Data)) })
// Pass deps — UseResource([]core.SignalAccessor{id}, …) — to reload on change.`

const errorCode = `// ErrorBoundary renders the fallback if rendering its child panics,
// so one broken subtree shows a message instead of blanking the page.
ErrorBoundary(
    func(err any) core.Node {
        return P(Textf("Recovered: %v", err))
    },
    brokenBox(), // a component that panics while rendering
)`

const greetCode = `// Route "/greet/:name" captures :name. Read it *reactively* with
// ParamSignal so the same (preserved) component updates in place when
// only the param changes — no remount.
return P(Textf("Hello, %s!", r.ParamSignal("name")))`

// aiRules is the drop-in guidance shown on the AI-assisted-coding page.
const aiRules = `# Building with goowee — rules for a coding agent

goowee is a Go→WebAssembly reactive UI framework. It is NOT React.

THE ONE RULE: a component's function body runs ONCE, on mount. It is
setup, not render. State lives in signals; changing a signal updates
only the exact DOM bound to it (no re-render, no virtual DOM).

- Static vs reactive text: Text("x") never updates. Use TextS(sig) or
  Textf("Count: %d", count) (signal args are reactive). Attributes have
  reactive -S variants: ClassS, StyleS, ValueS, DisabledS.
- State: count, setCount := hooks.UseState(0). Read count.Get(); write
  setCount(v). In markup pass the SIGNAL (count), not count.Get().
- Derive with core.Computed(deps, fn); react with hooks.UseEffect(deps, fn),
  hooks.Watch(deps, fn), or hooks.OnMount(fn) — deps are explicit.
- Off the render loop (timers, goroutines, fetch callbacks) NEVER call a
  setter directly — wrap it: core.Schedule(func() { setCount(n) }).
- Load data with hooks.UseResource(deps, fetch) → Data/Loading/Err + Refetch.
- Conditionals: Show / ShowElse / Switch (not a plain if in the body).
  Lists: For(sig, keyFn, render) with a stable key.
- SSR must be deterministic; wrap non-deterministic content in h.Dynamic().
- Imperative DOM: h.Ref()+h.RefTo(ref), then ref.Focus(). Overlays: h.Portal.
- Wrap risky subtrees in h.ErrorBoundary(fallback, child).

Full guide: https://github.com/yogisalomo/goowee/blob/main/AGENTS.md`

func aiGuidePage(r *router.Router) core.Node {
	return core.Component("AIGuidePage", func() core.Node {
		return Div(Class("page lesson"),
			A(Class("backlink"), Href("/tutorial"),
				OnClickE(func(core.EventData) { r.Navigate("/tutorial") }, PreventDefault()),
				Text("← Back to tutorial")),
			H1(Text("Coding goowee with an AI agent")),
			P(Class("lead"), Text("Agents (Claude Code, Cursor, and friends) usually trip on goowee's run-once model. Give yours these rules and it writes correct code the first time.")),
			P(Text("Save it as AGENTS.md or CLAUDE.md at your repo root, or paste it into your agent's rules. Click the box to select all, then copy.")),
			Textarea(Class("copybox"), ReadOnly(true), Rows(24),
				OnFocus(func() {}, SelectOnFocus()),
				Text(aiRules)),
			P(Text("The complete guide, mental model, full API cheat sheet, copy-paste patterns, and anti-patterns, lives in "),
				A(Href(repoURL+"/blob/main/AGENTS.md"), Target("_blank"), Rel("noopener"), Text("AGENTS.md")),
				Text(".")),
			tutorialStepNav(r, "/ai"),
		)
	})
}
