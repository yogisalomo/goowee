package app

import (
	"fmt"
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
	"time"
)

func App(r *router.Router) core.Node {
	return core.Component("App", func() core.Node {
		return appLayout(r,
			r.Route(map[string]func() core.Node{
				"/":                func() core.Node { return landingPage(r) },
				"/tutorial":        func() core.Node { return tutorialIndex(r) },
				"/getting-started": func() core.Node { return gettingStartedPage(r) },
				"/counter":         func() core.Node { return counterPage(r) },
				"/about":           aboutPage,
				"/form":            func() core.Node { return formPage(r) },
				"/todos":           func() core.Node { return todosPage(r) },
				"/stopwatch":       func() core.Node { return stopwatchPage(r) },
				"/dashboard":       func() core.Node { return dashboardPage(r) },
				"/async":           func() core.Node { return asyncPage(r) },
				"/error":           func() core.Node { return errorPage(r) },
				"/ai":              func() core.Node { return aiGuidePage(r) },
				"/greet/:name":     func() core.Node { return greetPage(r) },
			}),
		)
	})
}

func appLayout(r *router.Router, children ...core.Node) core.Node {
	return Div(Class("app"),
		appHeader(r),
		Main(Nodes(children)...),
		footer(),
	)
}

// logo is a small inline SVG mark (a terminal prompt) — also exercises the
// framework's namespaced-element support (h.Svg → createElementNS).
func logo() core.Node {
	return Svg(Class("logo"), Attr("width", "20"), Attr("height", "20"),
		Attr("viewBox", "0 0 20 20"), Attr("fill", "none"), Attr("aria-hidden", "true"),
		Rect(Attr("x", "1"), Attr("y", "1"), Attr("width", "18"), Attr("height", "18"),
			Attr("rx", "5"), Attr("fill", "#00add8")),
		Path(Attr("d", "M6 7l3 3-3 3"), Attr("stroke", "#fff"), Attr("stroke-width", "1.8"),
			Attr("stroke-linecap", "round"), Attr("stroke-linejoin", "round")),
		Line(Attr("x1", "11"), Attr("y1", "13.5"), Attr("x2", "14.5"), Attr("y2", "13.5"),
			Attr("stroke", "#fff"), Attr("stroke-width", "1.8"), Attr("stroke-linecap", "round")),
	)
}

func appHeader(r *router.Router) core.Node {
	return Nav(Class("nav"),
		A(Class("brand"), Href("/"),
			OnClickE(func(core.EventData) { r.Navigate("/") }, PreventDefault()),
			logo(), Text("goowee")),
		A(Href("/tutorial"),
			OnClickE(func(core.EventData) { r.Navigate("/tutorial") }, PreventDefault()),
			Text("Tutorial")),
		A(Href(repoURL), Target("_blank"), Rel("noopener"), Text("GitHub")),
	)
}

// greetPage demonstrates a URL param route. The component mounts once and is
// preserved across /greet/:name changes; it reads the name reactively via
// ParamSignal, so navigating between names updates the text in place.
func greetPage(r *router.Router) core.Node {
	return core.Component("GreetPage", func() core.Node {
		demo := Div(
			P(Class("preview"), Textf("Hello, %s!", r.ParamSignal("name"))),
			Div(Class("demo-row"),
				r.Link("/greet/alice", "Alice"),
				r.Link("/greet/bob", "Bob"),
				r.Link("/greet/carol", "Carol"),
			),
		)
		return lessonLayout(r, "URL params",
			"The route /greet/:name captures a param. This one component is preserved across name changes and reads the param reactively, so switching names updates the text in place, no remount.",
			demo, "greet.go", greetCode,
			howItWorks(
				"Register a param route: r.Route(map[string]...{ \"/greet/:name\": ... }).",
				"r.ParamSignal(\"name\") is a derived signal; binding it (via Textf) updates the text when only the param changes.",
				"Use r.Param(\"name\") for a one-shot read inside an event handler instead.",
			),
			tutorialStepNav(r, "/greet/alice"),
		)
	})
}

func homePage() core.Node {
	return H1(Text("Welcome to Goowee"))
}

func counterPage(r *router.Router) core.Node {
	return core.Component("CounterPage", func() core.Node {
		count, setCount := hooks.UseState(0)
		show, setShow := hooks.UseState(true)

		demo := Div(Class("demo-row"),
			Button(OnClick(func() { setCount(count.Get() + 1) }), Textf("Count: %d", count)),
			Button(OnClick(func() { setShow(!show.Get()) }), Text("Toggle")),
			ShowElse(show,
				func() core.Node { return P(Class("greeting"), Text("Hello!")) },
				func() core.Node { return P(Class("hidden"), Text("(hidden)")) },
			),
		)
		return lessonLayout(r, "Counter",
			"State is a signal. Reading it inside Textf binds that text to the signal, so clicking updates only the number, nothing else re-renders.",
			demo, "counter.go", counterCode,
			howItWorks(
				"hooks.UseState(0) returns a signal and a setter; read with count.Get(), write with setCount(v).",
				"Textf(\"Count: %d\", count) binds the signal, so the setter updates just that text node, not the component.",
				"ShowElse swaps between two views based on a boolean signal.",
			),
			tutorialStepNav(r, "/counter"),
		)
	})
}

type entry struct {
	id          int
	name, email string
	agreed      bool
}

func formPage(r *router.Router) core.Node {
	return core.Component("FormPage", func() core.Node {
		name, setName := hooks.UseState("")
		email, setEmail := hooks.UseState("")
		agreed, setAgreed := hooks.UseState(false)
		entries, setEntries := hooks.UseState([]entry{})
		nameRef := Ref() // imperative focus (h.Ref -> ref.Focus)

		demo := Div(
			Button(Type("button"), OnClick(func() { nameRef.Focus() }), Text("Focus name")),
			Form(
				OnSubmit(func(vals map[string]string) {
					en := entry{
						id:     len(entries.Get()) + 1, // append-only, so unique + stable
						name:   vals["name"],
						email:  vals["email"],
						agreed: vals["agreed"] == "on",
					}
					setEntries(append(entries.Get(), en))
					setName("")
					setEmail("")
					setAgreed(false)
				}),
				label("Name", Input(Type("text"), Name("name"), RefTo(nameRef), BindValue(name))),
				label("Email", Input(Type("email"), Name("email"), BindValue(email))),
				Label(
					Input(Type("checkbox"), Name("agreed"), BindChecked(agreed)),
					Text(" Subscribe to newsletter"),
				),
				Button(Text("Submit")),
			),
			P(Class("preview"), Textf("Preview, Name: %s, Email: %s", name, email)),
			H3(Text("Submissions")),
			Ul(Class("submission-list"),
				For(entries, func(e entry) int { return e.id }, func(e entry) core.Node {
					agreed := "no"
					if e.agreed {
						agreed = "yes"
					}
					return Li(
						Span(Class("sub-name"), Text(e.name)),
						Span(Class("sub-email"), Text(e.email)),
						Span(Class("sub-agreed"), Text("subscribed: "+agreed)),
					)
				}),
			),
		)
		return lessonLayout(r, "Forms & inputs",
			"BindValue keeps an input and a signal in sync both ways. OnSubmit hands you the named field values, and a ref lets you focus a field imperatively.",
			demo, "form.go", formCode,
			howItWorks(
				"BindValue(name) fills the input from the signal and updates the signal on every keystroke, two-way.",
				"OnSubmit(func(vals map[string]string){...}) receives values keyed by each input's Name; preventDefault is handled for you.",
				"ref := Ref(); attach with RefTo(ref); ref.Focus() from a handler focuses the node.",
				"For(entries, key, render) renders the keyed, reactive list of submissions.",
			),
			tutorialStepNav(r, "/form"),
		)
	})
}

func label(text string, input core.Node) core.Node {
	return Label(
		Text(text+" "),
		input,
		Text("\n"),
	)
}

type todo struct {
	id        int
	text      string
	completed bool
}

func todosPage(r *router.Router) core.Node {
	return core.Component("TodosPage", func() core.Node {
		initial := make([]todo, 100)
		for i := range initial {
			initial[i] = todo{id: i + 1, text: fmt.Sprintf("Item %d", i+1)}
		}
		todos, setTodos := hooks.UseState(initial)
		// Monotonic id source so ids stay unique even after removals
		// (len+1 would collide once anything is deleted).
		nextID, setNextID := hooks.UseState(len(initial) + 1)

		doneCount := core.Computed([]core.SignalAccessor{todos}, func() string {
			items := todos.Get()
			done := 0
			for _, t := range items {
				if t.completed {
					done++
				}
			}
			return fmt.Sprintf("%d/%d completed", done, len(items))
		})

		todoList := VirtualList(todos, 48, func(i int, t todo) core.Node {
			return Li(
				Class(func() string {
					if t.completed {
						return "completed"
					}
					return ""
				}()),
				Style("display:flex;align-items:center;gap:8px;padding:4px 8px;height:48px;box-sizing:border-box;"),
				Input(
					Type("checkbox"), Checked(t.completed),
					OnChange(func(value string) {
						cur := todos.Get()
						next := make([]todo, len(cur))
						copy(next, cur)
						for i := range next {
							if next[i].id == t.id {
								next[i].completed = !next[i].completed
								break
							}
						}
						setTodos(next)
					}),
				),
				Span(Text(t.text)),
				Button(
					OnClick(func() {
						cur := todos.Get()
						next := make([]todo, 0, len(cur))
						for _, td := range cur {
							if td.id != t.id {
								next = append(next, td)
							}
						}
						setTodos(next)
					}),
					Text("✕"),
				),
			)
		}, VirtualListHeight(300))

		demo := Div(
			Form(
				OnSubmit(func(vals map[string]string) {
					text := vals["todo"]
					if text == "" {
						return
					}
					id := nextID.Get()
					setNextID(id + 1)
					setTodos(append([]todo{{id: id, text: text}}, todos.Get()...))
				}),
				Input(Type("text"), Name("todo"), Placeholder("What needs to be done?")),
				Button(Text("Add")),
			),
			P(Class("preview"), TextS(doneCount)),
			todoList,
		)
		return lessonLayout(r, "Keyed lists & virtualization",
			"This list holds 100 items but only renders the rows on screen. Each row is keyed, so toggling or removing one touches just that row.",
			demo, "todos.go", todosCode,
			howItWorks(
				"VirtualList(sig, rowHeight, render, VirtualListHeight(h)) renders only the visible window of a large list.",
				"Each row updates its slice immutably (copy, then setTodos) so the signal notices the change.",
				"doneCount is a Computed over todos; it recomputes only when the list changes.",
			),
			tutorialStepNav(r, "/todos"),
		)
	})
}

func errorPage(r *router.Router) core.Node {
	return core.Component("ErrorPage", func() core.Node {
		demo := Div(
			ErrorBoundary(
				func(err any) core.Node {
					return P(Style("color:#c0392b;font-family:var(--mono)"), Textf("Recovered: %v", err))
				},
				brokenBox(),
			),
			P(Text("This line still renders below the boundary.")),
		)
		return lessonLayout(r, "Error boundaries",
			"The box below panics on purpose. The boundary catches the failure and renders a fallback, so the rest of the page keeps working instead of going blank.",
			demo, "error.go", errorCode,
			howItWorks(
				"h.ErrorBoundary(fallback, child) renders fallback(err) if rendering child panics.",
				"It is for render-time failures (bad data at mount); update-time panics are contained (the subtree keeps its previous state) and logged.",
				"Everything outside the boundary renders normally, so the failure is scoped.",
			),
			tutorialStepNav(r, "/error"),
		)
	})
}

func brokenBox() core.Node {
	return core.Component("BrokenBox", func() core.Node {
		panic("intentional failure to demo the error boundary")
	})
}

func asyncPage(r *router.Router) core.Node {
	return core.Component("AsyncPage", func() core.Node {
		// A goroutine does the "loading" off the render loop; UseResource applies
		// the result back safely via core.Schedule. Server renders the loading
		// state; the client loads after hydration.
		res := hooks.UseResource(nil, func() (string, error) {
			time.Sleep(500 * time.Millisecond)
			return "Loaded at " + time.Now().Format("15:04:05.000"), nil
		})
		demo := Div(
			ShowElse(res.Loading,
				func() core.Node { return P(Text("Loading\u2026")) },
				func() core.Node { return P(Class("preview"), TextS(res.Data)) },
			),
			Button(Type("button"), OnClick(func() { res.Refetch() }), Text("Reload")),
		)
		return lessonLayout(r, "Async data",
			"Real work runs in a goroutine, off the render loop, and UseResource applies the result back safely. The view just binds the loading and data signals.",
			demo, "async.go", asyncCode,
			howItWorks(
				"hooks.UseResource(deps, fetch) runs fetch in a goroutine; its result lands on the render loop via core.Schedule.",
				"It exposes Data/Loading/Err signals and a Refetch() method; a generation guard drops stale results.",
				"Fetching is client-side, so SSR renders the loading state and the client fills it in after hydration.",
			),
			tutorialStepNav(r, "/async"),
		)
	})
}

func stopwatchPage(r *router.Router) core.Node {
	return core.Component("StopwatchPage", func() core.Node {
		elapsed, setElapsed := hooks.UseState(0)
		running, setRunning := hooks.UseState(false)

		display := core.Computed([]core.SignalAccessor{elapsed}, func() string {
			e := elapsed.Get()
			tenths := e % 10
			whole := e / 10
			secs := whole % 60
			mins := whole / 60
			return fmt.Sprintf("%02d:%02d.%d", mins, secs, tenths)
		})

		runLabel := core.Computed([]core.SignalAccessor{running}, func() string {
			if running.Get() {
				return "Pause"
			}
			return "Start"
		})

		hooks.OnMount(func() func() {
			stop := make(chan struct{})
			go func() {
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						// The ticker runs off the render loop, so hand the state
						// change to the scheduler rather than touching signals
						// here (see core.Schedule / ADR-015).
						core.Schedule(func() {
							if running.Get() {
								setElapsed(elapsed.Get() + 1)
							}
						})
					}
				}
			}()
			return func() { close(stop) }
		})

		demo := Div(
			P(Style("font-size:2rem;font-family:var(--mono);margin:0 0 12px"), TextS(display)),
			Div(Class("demo-row"),
				Button(OnClick(func() { setRunning(!running.Get()) }), TextS(runLabel)),
				Button(OnClick(func() { setElapsed(0); setRunning(false) }), Text("Reset")),
			),
		)
		return lessonLayout(r, "Lifecycle & off-loop timers",
			"OnMount starts a ticker goroutine when the component mounts and stops it on unmount. Because the ticker runs off the render loop, it updates state through core.Schedule.",
			demo, "stopwatch.go", stopwatchCode,
			howItWorks(
				"hooks.OnMount(fn) runs once on mount (client only) and returns a cleanup that runs on unmount, so no goroutine leaks.",
				"The ticker is off the render loop, so it wraps the update in core.Schedule(func(){...}) instead of calling the setter directly.",
				"display is a Computed that formats elapsed tenths as mm:ss.d.",
			),
			tutorialStepNav(r, "/stopwatch"),
		)
	})
}

func dashboardPage(r *router.Router) core.Node {
	return core.Component("DashboardPage", func() core.Node {
		data, setData := hooks.UseState([]dashboardRow{
			{Label: "Alpha", Value: 42, Unit: "km"},
			{Label: "Beta", Value: 17, Unit: "%"},
			{Label: "Gamma", Value: 88, Unit: "°C"},
			{Label: "Delta", Value: 5, Unit: "L"},
			{Label: "Epsilon", Value: 63, Unit: "kg"},
		})

		selected, setSelected := hooks.UseState(-1)

		totalStr := core.Computed([]core.SignalAccessor{data}, func() string {
			sum := 0
			for _, r := range data.Get() {
				sum += r.Value
			}
			return fmt.Sprintf("Total: %d", sum)
		})

		selectedText := core.Computed([]core.SignalAccessor{selected, data}, func() string {
			idx := selected.Get()
			if idx < 0 || idx >= len(data.Get()) {
				return "No row selected."
			}
			row := data.Get()[idx]
			return fmt.Sprintf("Selected: %s = %d%s", row.Label, row.Value, row.Unit)
		})

		demo := Div(
			P(Class("preview"), TextS(totalStr)),
			Table(
				Style("border-collapse:collapse;width:100%;max-width:500px;"),
				Thead(
					Tr(
						Th(Style("text-align:left;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Label")),
						Th(Style("text-align:right;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Value")),
						Th(Style("text-align:left;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Unit")),
					),
				),
				Tbody(Nodes(dashboardRows(data, selected, setSelected))...),
			),
			P(TextS(selectedText)),
			Button(OnClick(func() {
				cur := data.Get()
				next := make([]dashboardRow, len(cur))
				copy(next, cur)
				for i := range next {
					next[i].Value = (next[i].Value*7 + 13) % 100
				}
				setData(next)
			}), Text("Randomize Values")),
		)
		return lessonLayout(r, "Computed values & selection",
			"Two Computed values derive from signals: a running total, and a description of the selected row. Each recomputes only when its inputs change. Click a row or randomize to see it.",
			demo, "dashboard.go", dashboardCode,
			howItWorks(
				"core.Computed(deps, fn) is a read-only signal derived from others; it recomputes only when a listed dep changes.",
				"selectedText depends on both selected and data, so it updates when either changes.",
				"Clicking a row calls setSelected(i); Randomize replaces data immutably so dependents recompute.",
			),
			tutorialStepNav(r, "/dashboard"),
		)
	})
}

type dashboardRow struct {
	Label string
	Value int
	Unit  string
}

func dashboardRows(data *core.Signal[[]dashboardRow], selected *core.Signal[int], setSelected func(int)) []core.Node {
	rows := data.Get()
	nodes := make([]core.Node, len(rows))
	for i, r := range rows {
		i := i
		r := r
		highlight := ""
		if i == selected.Get() {
			highlight = "background:#eef;"
		}
		nodes[i] = Tr(
			Style(highlight+"cursor:pointer;"),
			OnClick(func() { setSelected(i) }),
			Td(Style("padding:4px 8px;border-bottom:1px solid #ddd;"), Text(r.Label)),
			Td(Style("text-align:right;padding:4px 8px;border-bottom:1px solid #ddd;"), Text(fmt.Sprintf("%d", r.Value))),
			Td(Style("padding:4px 8px;border-bottom:1px solid #ddd;"), Text(r.Unit)),
		)
	}
	return nodes
}

type tutorialStep struct {
	n, path, title string
}

var tutorialSteps = []tutorialStep{
	{"00", "/getting-started", "Getting Started"},
	{"01", "/counter", "Counter"},
	{"02", "/form", "Form"},
	{"03", "/todos", "Todos"},
	{"04", "/dashboard", "Dashboard"},
	{"05", "/stopwatch", "Stopwatch"},
	{"06", "/async", "Async"},
	{"07", "/error", "Error Boundary"},
	{"08", "/greet/alice", "Greeting"},
	{"09", "/ai", "Coding with AI"},
}

func tutorialStepNav(r *router.Router, current string) core.Node {
	var prev, next *tutorialStep
	for i := range tutorialSteps {
		s := &tutorialSteps[i]
		if s.path == current {
			if i > 0 {
				prev = &tutorialSteps[i-1]
			}
			if i < len(tutorialSteps)-1 {
				next = &tutorialSteps[i+1]
			}
			break
		}
	}
	var left, right core.Node
	if prev != nil {
		left = A(Class("btn"), Href(prev.path),
			OnClickE(func(core.EventData) { r.Navigate(prev.path) }, PreventDefault()),
			Text("\u2190 "+prev.n+" "+prev.title),
		)
	}
	if next != nil {
		right = A(Class("btn"), Href(next.path),
			OnClickE(func(core.EventData) { r.Navigate(next.path) }, PreventDefault()),
			Text(next.n+" "+next.title+" \u2192"),
		)
	}
	return Nav(Class("tutorial-nav"),
		left,
		Span(Style("flex:1")),
		right,
	)
}

func gettingStartedPage(r *router.Router) core.Node {
	return core.Component("GettingStarted", func() core.Node {
		return Div(Class("page lesson"),
			A(Class("backlink"), Href("/tutorial"),
				OnClickE(func(core.EventData) { r.Navigate("/tutorial") }, PreventDefault()),
				Text("← Back to tutorial")),
			H1(Text("Getting started")),
			H3(Text("Install goowee")),
			P(Text("Add the module to your project:")),
			Pre(Class("code"), Text("go get github.com/yogisalomo/goowee")),
			H3(Text("App structure")),
			P(Text("A goowee project has two parts: a WASM binary and an optional SSR server. The minimal layout:")),
			Pre(Class("code"), Text(`myapp/
  cmd/
    app/
      main.go    WASM entry point
    server/
      main.go    SSR server (optional)
  app/
    app.go       component tree
  web/
    index.html   page shell`)),
			P(Text("The WASM entry point (cmd/app/main.go):")),
			Pre(Class("code"), Text(`//go:build js && wasm

package main

import (
    "github.com/yogisalomo/goowee/bridge"
    "github.com/yogisalomo/goowee/router"
    "myapp/app"
)

func main() {
    r := router.New(router.CurrentPath())
    r.BindHistory()
    // bridge.Run mounts the app, hydrates if the page was
    // server-rendered, and drives the render loop. It never returns.
    bridge.Run(app.App(r))
}`)),
			H3(Text("Hello World")),
			P(Text("A component is a function that returns a node tree:")),
			Pre(Class("code"), Text(`func Hello() core.Node {
    return core.Component("Hello", func() core.Node {
        name, setName := hooks.UseState("World")
        return Div(
            P(Textf("Hello, %s!", name)),
            Input(BindValue(name)),
        )
    })
}`)),
			H3(Text("Build & serve")),
			P(Text("Compile the WASM binary:")),
			Pre(Class("code"), Text("GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app")),
			P(Text("Copy the runtime files and serve the web directory:")),
			Pre(Class("code"), Text(`cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
cp "$(go env GOMODCACHE)"/github.com/yogisalomo/goowee@*/runtime/goowee.js web/
cd web && python3 -m http.server 8080`)),
			P(Text("Open "), Text("http://localhost:8080"), Text(" in your browser.")),
			tutorialStepNav(r, "/getting-started"),
		)
	})
}

func aboutPage() core.Node {
	return Div(
		H2(Text("About")),
		P(Text("A minimal Go WASM signal-based UI framework.")),
	)
}
