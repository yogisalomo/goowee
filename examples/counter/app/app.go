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
				"/":            homePage,
				"/counter":     counterPage,
				"/about":       aboutPage,
				"/form":        formPage,
				"/todos":       todosPage,
				"/stopwatch":   stopwatchPage,
				"/dashboard":   dashboardPage,
				"/greet/:name": func() core.Node { return greetPage(r) },
			}),
		)
	})
}

func appLayout(r *router.Router, children ...core.Node) core.Node {
	return Div(Class("app"),
		appHeader(r),
		Main(Nodes(children)...),
	)
}

func appHeader(r *router.Router) core.Node {
	return Nav(Class("nav"),
		r.Link("/", "Home"),
		Text(" | "),
		r.Link("/counter", "Counter"),
		Text(" | "),
		r.Link("/about", "About"),
		Text(" | "),
		r.Link("/form", "Form"),
		Text(" | "),
		r.Link("/todos", "Todos"),
		Text(" | "),
		r.Link("/stopwatch", "Stopwatch"),
		Text(" | "),
		r.Link("/dashboard", "Dashboard"),
		Text(" | "),
		r.Link("/greet/alice", "Greet"),
	)
}

// greetPage demonstrates a URL param route. The component mounts once and is
// preserved across /greet/:name changes; it reads the name reactively via
// ParamSignal, so navigating between names updates the text in place.
func greetPage(r *router.Router) core.Node {
	return core.Component("GreetPage", func() core.Node {
		return Div(
			H2(Text("Greeting")),
			P(Textf("Hello, %s!", r.ParamSignal("name"))),
			Nav(Class("nav"),
				r.Link("/greet/alice", "Alice"),
				Text(" | "),
				r.Link("/greet/bob", "Bob"),
				Text(" | "),
				r.Link("/greet/carol", "Carol"),
			),
		)
	})
}

func homePage() core.Node {
	return H1(Text("Welcome to Goowee"))
}

func counterPage() core.Node {
	return core.Component("CounterPage", func() core.Node {
		count, setCount := hooks.UseState(0)
		show, setShow := hooks.UseState(true)

		return Div(
			Button(
				OnClick(func() { setCount(count.Get() + 1) }),
				Textf("Count: %d", count),
			),
			Button(
				OnClick(func() { setShow(!show.Get()) }),
				Text("Toggle"),
			),
			ShowElse(show,
				func() core.Node { return P(Class("greeting"), Text("Hello!")) },
				func() core.Node { return P(Class("hidden"), Text("(hidden)")) },
			),
		)
	})
}

type entry struct {
	id          int
	name, email string
	agreed      bool
}

func formPage() core.Node {
	return core.Component("FormPage", func() core.Node {
		name, setName := hooks.UseState("")
		email, setEmail := hooks.UseState("")
		agreed, setAgreed := hooks.UseState(false)
		entries, setEntries := hooks.UseState([]entry{})

		return Div(
			H2(Text("Form Demo")),
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
				label("Name", Input(
					Type("text"), Name("name"), BindValue(name),
				)),
				label("Email", Input(
					Type("email"), Name("email"), BindValue(email),
				)),
				Label(
					Input(
						Type("checkbox"), Name("agreed"), BindChecked(agreed),
					),
					Text(" Subscribe to newsletter"),
				),
				Button(Text("Submit")),
			),
			H3(Text("Preview")),
			P(Textf("Name: %s, Email: %s", name, email)),
			H3(Text("Submissions")),
			For(entries, func(e entry) int { return e.id }, func(e entry) core.Node {
				agreed := "no"
				if e.agreed {
					agreed = "yes"
				}
				return Li(Text(e.name + " — " + e.email + " (subscribed: " + agreed + ")"))
			}),
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

func todosPage() core.Node {
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

		return Div(
			H2(Text("Todo List")),
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
				Input(
					Type("text"), Name("todo"), Placeholder("What needs to be done?"),
				),
				Button(Text("Add")),
			),
			P(TextS(doneCount)),
			todoList,
		)
	})
}

func stopwatchPage() core.Node {
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

		return Div(
			H2(Text("Stopwatch")),
			P(Style("font-size:2rem;font-family:monospace;"), TextS(display)),
			Button(
				OnClick(func() { setRunning(!running.Get()) }),
				TextS(runLabel),
			),
			Button(
				OnClick(func() { setElapsed(0); setRunning(false) }),
				Text("Reset"),
			),
		)
	})
}

func dashboardPage() core.Node {
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

		return Div(
			H2(Text("Dashboard")),
			P(TextS(totalStr)),
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
			Button(
				OnClick(func() {
					cur := data.Get()
					next := make([]dashboardRow, len(cur))
					copy(next, cur)
					for i := range next {
						next[i].Value = (next[i].Value*7 + 13) % 100
					}
					setData(next)
				}),
				Text("Randomize Values"),
			),
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

func aboutPage() core.Node {
	return Div(
		H2(Text("About")),
		P(Text("A minimal Go WASM signal-based UI framework.")),
	)
}
