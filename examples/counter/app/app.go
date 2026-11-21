package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/form"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
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
				"/events":          func() core.Node { return eventsPage(r) },
				"/canvas":          func() core.Node { return canvasPage(r) },
				"/graph":           func() core.Node { return graphPage(r) },
			}),
		)
	})
}

func appLayout(r *router.Router, children ...core.Node) core.Node {
	return Div(Class("app"),
		Metadata(
			Page(PageMeta{
				Title:       "goowee — Reactive web UIs in Go",
				Description: "goowee is a golang frontend framework that compiles to WebAssembly — signals drive fine-grained DOM updates, no virtual DOM, no build step.",
				Keywords:    []string{"golang", "wasm", "reactive", "web", "ui", "framework", "frontend"},
				Charset:     "utf-8",
			}),
			StyleEl(Text("body{margin:0;font-family:system-ui,sans-serif;background:#fafafa;color:#1a1a2e}")),
			Link(Rel("icon"), Attr("href", "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 20 20'><rect x='1' y='1' width='18' height='18' rx='5' fill='%2300add8'/></svg>")),
		),
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
		r.LinkTo("/", Class("brand"), logo(), Text("goowee")),
		r.Link("/tutorial", "Tutorial"),
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
			"The route `/greet/:name` captures a param. This one component is preserved across name changes and reads the param reactively, so switching names updates the text in place, no remount.",
			demo, "greet.go", greetCode,
			howItWorks(
				"Register a param route: `r.Route(map[string]func() core.Node{\"/greet/:name\": …})`.",
				"`r.ParamSignal(\"name\")` is a derived signal; binding it (via `Textf`) updates the text when only the param changes.",
				"Use `r.Param(\"name\")` for a one-shot read inside an event handler instead.",
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
			"State is a signal. Reading it inside `Textf` binds that text to the signal, so clicking updates only the number, nothing else re-renders.",
			demo, "counter.go", counterCode,
			howItWorks(
				"`hooks.UseState(0)` returns a signal and a setter; read with `count.Get()`, write with `setCount(v)`.",
				"`Textf(\"Count: %d\", count)` binds the signal, so the setter updates just that text node, not the component.",
				"`ShowElse` swaps between two views based on a boolean signal.",
			),
			tutorialStepNav(r, "/counter"),
		)
	})
}

type entry struct {
	id          int
	name, email string
	age         int
	agreed      bool
}

// signup is the form lesson's value. Validate is plain Go: goowee/form runs
// it as the user types and on submit, and a server would run the same method
// on what it receives (examples/notes does).
type signup struct {
	Name   string
	Email  string
	Agreed bool
}

func (s signup) Validate() form.Errors {
	var errs form.Errors
	if strings.TrimSpace(s.Name) == "" {
		errs = errs.Add("name", "Enter your name.")
	}
	if !strings.Contains(s.Email, "@") {
		errs = errs.Add("email", "Enter an email address.")
	}
	return errs
}

func formPage(r *router.Router) core.Node {
	return core.Component("FormPage", func() core.Node {
		f := form.Use(signup{})
		age, setAge := hooks.UseState(0)
		entries, setEntries := hooks.UseState([]entry{})
		nameRef := Ref() // imperative focus (h.Ref -> ref.Focus), measuring (ref.Get)
		nameWidth, setNameWidth := hooks.UseState(0)
		attachment, setAttachment := hooks.UseState("no file picked")

		// A file input's change event carries the picked files' metadata; the
		// bytes are read off-loop with File.Bytes and applied via core.Schedule.
		pickFile := func(e core.EventData) {
			files := e.Files()
			if len(files) == 0 {
				setAttachment("no file picked")
				return
			}
			f := files[0]
			setAttachment(fmt.Sprintf("picked %s (%d bytes), reading…", f.Name, f.Size))
			go func() {
				data, err := f.Bytes()
				core.Schedule(func() {
					if err != nil {
						setAttachment(fmt.Sprintf("%s: %v", f.Name, err))
						return
					}
					setAttachment(fmt.Sprintf("%s: read %d bytes: %q", f.Name, len(data), preview(data)))
				})
			}()
		}

		hasName := hooks.UseComputed([]core.SignalAccessor{f.Value}, func() bool { return f.Value.Get().Name != "" })

		demo := Div(
			Button(Type("button"), OnClick(func() { nameRef.Focus() }), Text("Focus name")),
			Button(Type("button"), OnClick(func() {
				nameRef.Get("offsetWidth", func(v any) {
					w, _ := v.(float64)
					setNameWidth(int(w))
				})
			}), Text("Measure name")),
			ShowWhen(nameWidth, func(w int) bool { return w > 0 },
				func() core.Node { return P(Class("measure"), Textf("Name input is %dpx wide", nameWidth)) }),
			Form(
				// This lesson has no server: the submit function succeeds at
				// once, and the second function, run after it, records the
				// entry and starts the form over.
				f.OnSubmit(func(ctx context.Context, v signup) error { return nil }, func(v signup) {
					en := entry{
						id:     len(entries.Get()) + 1, // append-only, so unique + stable
						name:   v.Name,
						email:  v.Email,
						age:    age.Get(),
						agreed: v.Agreed,
					}
					setEntries(append(entries.Get(), en))
					f.Reset(signup{})
					setAge(0)
				}),
				label("Name", Input(Type("text"), RefTo(nameRef), form.Text(f, "name", func(s *signup) *string { return &s.Name }))),
				P(Class("field-error"), TextS(f.Error("name"))),
				label("Email", Input(Type("email"), form.Text(f, "email", func(s *signup) *string { return &s.Email }))),
				P(Class("field-error"), TextS(f.Error("email"))),
				label("Age", Input(Type("number"), Name("age"), BindNumber(age))),
				Label(
					Input(form.Bool(f, "agreed", func(s *signup) *bool { return &s.Agreed })),
					Text(" Subscribe to newsletter"),
				),
				label("Attachment", Input(Type("file"), Name("attachment"), OnChangeE(pickFile))),
				P(Class("attachment"), TextS(attachment)),
				Button(Type("submit"), DisabledS(f.Submitting), Text("Submit")),
			),
			P(Class("preview"), ClassIf("has-name", hasName), TextS(core.Computed([]core.SignalAccessor{f.Value, age}, func() string {
				v := f.Value.Get()
				return fmt.Sprintf("Preview, Name: %s, Email: %s, Age: %d", v.Name, v.Email, age.Get())
			}))),
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
						Span(Class("sub-age"), Textf("age: %d", e.age)),
						Span(Class("sub-agreed"), Text("subscribed: "+agreed)),
					)
				}),
			),
		)
		return lessonLayout(r, "Forms & inputs",
			"goowee/form keeps a form's fields in one typed value, checked by a plain Go `Validate` method that a server can run too. `BindNumber` keeps a number input and an `int` signal in sync, `ClassIf` toggles one class from a bool signal, and a ref lets you focus a field imperatively.",
			demo, "form.go", formCode,
			howItWorks(
				"`f := form.Use(signup{})` holds the value, its errors, and which fields were left or submitted.",
				"`form.Text(f, \"name\", func(s *signup) *string { return &s.Name })` binds an input to a field both ways, and sets its `name` and `aria-invalid`.",
				"`signup.Validate()` runs as you type once a field was left, and on submit; `f.Error(\"name\")` is that field's message while it should show.",
				"`f.OnSubmit(send, then)` runs `send` in a goroutine with `f.Submitting` set, shows a server's field errors on their fields, and runs `then` after a success.",
				"`BindNumber(age)` uses the browser's `valueAsNumber`, so the signal holds an `int` rather than a string, and a half-typed number leaves it alone.",
				"`ClassIf(\"has-name\", hasName)` toggles one class from a bool signal without recomputing the whole class list.",
				"`ref := Ref()`, attached with `RefTo(ref)`; `ref.Focus()` in a handler focuses the node.",
				"`ref.Get(\"offsetWidth\", func(v any) {…})` reads a value back from the node on the next frame; the callback runs on the render loop.",
				"`OnChangeE` on a file input gives `e.Files()`: name, size, type. `f.Bytes()` reads the contents in a goroutine, and `core.Schedule` applies the result.",
				"`For(entries, key, render)` renders the keyed, reactive list of submissions.",
			),
			tutorialStepNav(r, "/form"),
		)
	})
}

// preview trims file contents to a short, single-line snippet for display.
func preview(data []byte) string {
	const max = 40
	s := string(data)
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
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

		doneCount := core.Derive(todos, func(items []todo) string {
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
				Transition("todo"),
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
			"This list holds 100 items but only renders the rows on screen. Each row is keyed, so toggling or removing one touches just that row. Try adding or removing one to see the enter/leave transition.",
			demo, "todos.go", todosCode,
			howItWorks(
				"`VirtualList(sig, rowHeight, render, VirtualListHeight(h))` renders only the visible window of a large list.",
				"`Transition(\"todo\")` marks a row for animation: the runtime adds `.todo-enter` on insert and holds a removed row with `.todo-leave` until its CSS transition ends. Reduced-motion users skip it.",
				"Each row updates its slice immutably (copy, then `setTodos`) so the signal notices the change.",
				"`doneCount` is a `Computed` over `todos`; it recomputes only when the list changes.",
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
				"`h.ErrorBoundary(fallback, child)` renders `fallback(err)` if rendering `child` panics.",
				"A panic while the child renders — at mount, or when a re-render reaches the boundary — shows the fallback; whatever the failed render had set up is cleaned up, and a later successful render brings the child back.",
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
		// A goroutine does the "loading" off the render loop; the result comes
		// back safely via core.Schedule. hooks.Load also runs it on the server:
		// the page arrives with the data, and the client doesn't fetch again.
		demo := hooks.Load("async-demo", nil, func(ctx context.Context) (string, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return "Loaded at " + time.Now().Format("15:04:05.000"), nil
			case <-ctx.Done(): // reloaded or navigated away: stop early
				return "", ctx.Err()
			}
		}, func(res *hooks.Resource[string]) core.Node {
			return Div(
				ShowElse(res.Loading,
					func() core.Node { return P(Text("Loading\u2026")) },
					func() core.Node { return P(Class("preview"), TextS(res.Data)) },
				),
				Button(Type("button"), OnClick(func() { res.Refetch() }), Text("Reload")),
			)
		})
		return lessonLayout(r, "Async data",
			"Real work runs in a goroutine, off the render loop, and `UseResource` applies the result back safely. The view just binds the loading and data signals.",
			demo, "async.go", asyncCode,
			howItWorks(
				"`hooks.UseResource(deps, fetch)` runs `fetch` in a goroutine; its result lands on the render loop through `core.Schedule`.",
				"`fetch` gets a context that is cancelled when a newer load starts or the page unmounts — pass it to your request.",
				"It exposes `Data`, `Loading` and `Err` signals and a `Refetch()` method; a generation guard drops stale results.",
				"hooks.Load(key, deps, fetch, render) is UseResource whose data the server loads too: the page arrives with it, embedded as JSON, and the client doesn't fetch it again.",
			),
			tutorialStepNav(r, "/async"),
		)
	})
}

func stopwatchPage(r *router.Router) core.Node {
	return core.Component("StopwatchPage", func() core.Node {
		elapsed, setElapsed := hooks.UseState(0)
		running, setRunning := hooks.UseState(false)

		display := core.Derive(elapsed, func(e int) string {
			tenths := e % 10
			whole := e / 10
			secs := whole % 60
			mins := whole / 60
			return fmt.Sprintf("%02d:%02d.%d", mins, secs, tenths)
		})

		runLabel := core.Derive(running, func(on bool) string {
			if on {
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
			"`OnMount` starts a ticker goroutine when the component mounts and stops it on unmount. Because the ticker runs off the render loop, it updates state through `core.Schedule`.",
			demo, "stopwatch.go", stopwatchCode,
			howItWorks(
				"`hooks.OnMount(fn)` runs once on mount (client only) and returns a cleanup that runs on unmount, so no goroutine leaks.",
				"The ticker is off the render loop, so it wraps the update in `core.Schedule(func() {…})` instead of calling the setter directly.",
				"`display` is a `Computed` that formats elapsed tenths as mm:ss.d.",
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

		totalStr := core.Derive(data, func(rows []dashboardRow) string {
			sum := 0
			for _, r := range rows {
				sum += r.Value
			}
			return fmt.Sprintf("Total: %d", sum)
		})

		selectedText := core.Derive2(selected, data, func(idx int, rows []dashboardRow) string {
			if idx < 0 || idx >= len(rows) {
				return "No row selected."
			}
			row := rows[idx]
			return fmt.Sprintf("Selected: %s = %d%s", row.Label, row.Value, row.Unit)
		})

		// A <canvas> drawn from Go through bridge.Element: OnMount runs once
		// the element is in the document; Watch redraws on every change.
		chart := Ref()
		values := func() []int {
			var v []int
			for _, row := range data.Get() {
				v = append(v, row.Value)
			}
			return v
		}
		hooks.OnMount(func() func() { drawBars(chart, values()); return nil })
		hooks.Watch([]core.SignalAccessor{data}, func() { drawBars(chart, values()) })

		demo := Div(
			P(Class("preview"), TextS(totalStr)),
			Canvas(Class("bars"), RefTo(chart), Width("300"), Height("80"), AriaLabel("Values as bars")),
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
			"Two derived values come from signals: a running total, and a description of the selected row. Each recomputes only when its inputs change. Click a row or randomize to see it.",
			demo, "dashboard.go", dashboardCode,
			howItWorks(
				"`core.Derive(dep, fn)` is a read-only signal derived from one dependency; `Derive2` takes two. Each recomputes only when its input changes.",
				"`selectedText` uses `Derive2(selected, data, …)`, so it updates when either changes. Use `Computed(core.Deps(a, b, c), …)` for more.",
				"Clicking a row calls `setSelected(i)`; Randomize replaces `data` immutably so dependents recompute.",
				"The bars are drawn on a `<canvas>` from `OnMount` via `bridge.Element(ref)` — how you hand an element to a JavaScript library (a chart, a map, an editor).",
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
	{"09", "/events", "Events"},
	{"10", "/canvas", "Canvas"},
	{"11", "/ai", "Coding with AI"},
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
		left = r.Link(prev.path, "\u2190 "+prev.n+" "+prev.title, Class("btn"))
	}
	if next != nil {
		right = r.Link(next.path, next.n+" "+next.title+" \u2192", Class("btn"))
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
			r.Link("/tutorial", "← Back to tutorial", Class("backlink")),
			H1(Text("Getting started")),
			H3(Text("Install goowee")),
			P(Text("Add the module to your project (Go 1.25+):")),
			snippet("sh", "go get github.com/yogisalomo/goowee@latest"),
			H3(Text("App structure")),
			P(Text("A goowee project has two parts: a WASM binary and an optional SSR server. The minimal layout:")),
			snippet("", `myapp/
  cmd/
    app/
      main.go    WASM entry point
    server/
      main.go    SSR server (optional)
  app/
    app.go       component tree
  web/
    index.html   page shell`),
			P(rich("The WASM entry point (`cmd/app/main.go`):")),
			snippet("go", `//go:build js && wasm

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
}`),
			H3(Text("Hello World")),
			P(Text("A component is a function that returns a node tree:")),
			snippet("go", `func Hello() core.Node {
    return core.Component("Hello", func() core.Node {
        name, setName := hooks.UseState("World")
        return Div(
            P(Textf("Hello, %s!", name)),
            Input(BindValue(name)),
        )
    })
}`),
			H3(Text("Build & serve")),
			P(Text("Compile the WASM binary:")),
			snippet("sh", "GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app"),
			P(rich("The page shell (`web/index.html`) loads the Go runtime glue and goowee's runtime, then boots `main.wasm` into `#root`:")),
			snippet("html", `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <script src="wasm_exec.js"></script>
  <script src="goowee.js"></script>
</head>
<body>
  <div id="root">Loading…</div>
  <script>goowee.boot();</script>
</body>
</html>`),
			P(rich("Whatever `#root` holds until the app mounts (here a loading message) is replaced by its first frame.")),
			P(rich("Copy the matching runtime files and serve the `web` directory:")),
			snippet("sh", `go run github.com/yogisalomo/goowee/cmd/goowee assets ./web
cd web && python3 -m http.server 8080`),
			P(rich("Open `http://localhost:8080` in your browser.")),
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
