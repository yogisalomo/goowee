package app

import (
	"fmt"
	"time"
	. "goowee/h"
	"goowee/core"
	"goowee/hooks"
	"goowee/router"
)

func App(r *router.Router) core.Node {
	return core.Component("App", func() core.Node {
		return appLayout(r,
			r.Route(map[string]func() core.Node{
				"/":          homePage,
				"/counter":   counterPage,
				"/about":     aboutPage,
				"/form":      formPage,
				"/todos":     todosPage,
				"/stopwatch": stopwatchPage,
				"/dashboard": dashboardPage,
			}),
		)
	})
}

func appLayout(r *router.Router, children ...core.Node) core.Node {
	return Div(Class("app"),
		appHeader(r),
		Main(childrenToItems(children)...),
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
	)
}

func homePage() core.Node {
	return H1(Text("Welcome to Goowee"))
}

func counterPage() core.Node {
	return core.Component("CounterPage", func() core.Node {
		count, setCount := hooks.UseState(0)
		show, setShow := hooks.UseState(true)

		greeting := hooks.UseScope(func() core.Node {
			if show.Get() {
				return P(Class("greeting"), Text("Hello!"))
			}
			return P(Class("hidden"), Text("(hidden)"))
		}, show)

		return Div(
			Button(
				OnClick(func() { setCount(count.Get() + 1) }),
				Textf("Count: %d", count),
			),
			Button(
				OnClick(func() { setShow(!show.Get()) }),
				Text("Toggle"),
			),
			greeting,
		)
	})
}

type entry struct {
	name, email string
	agreed      bool
}

func formPage() core.Node {
	return core.Component("FormPage", func() core.Node {
		name, setName := hooks.UseState("")
		email, setEmail := hooks.UseState("")
		agreed, setAgreed := hooks.UseState(false)
		entries, setEntries := hooks.UseState([]entry{})

		preview := hooks.UseScope(func() core.Node {
			return P(Text("Name: " + name.Get() + ", Email: " + email.Get()))
		}, name, email)

		subList := hooks.UseScope(func() core.Node {
			return submissionsList(entries.Get())
		}, entries)

		return Div(
			H2(Text("Form Demo")),
			Form(
				OnSubmit(func(vals map[string]string) {
					en := entry{
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
			preview,
			H3(Text("Submissions")),
			subList,
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

func submissionsList(entries []entry) core.Node {
	if len(entries) == 0 {
		return P(Text("No submissions yet."))
	}
	var items []core.Node
	for _, en := range entries {
		agreed := "no"
		if en.agreed {
			agreed = "yes"
		}
		items = append(items, Li(
			Text(en.name+" — "+en.email+" (subscribed: "+agreed+")"),
		))
	}
	return Ul(Group(childrenToItems(items)...))
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
						for i := range cur {
							if cur[i].id == t.id {
								cur[i].completed = !cur[i].completed
								break
							}
						}
						setTodos(cur)
					}),
				),
				Span(Text(t.text)),
				Button(
					OnClick(func() {
						cur := todos.Get()
						for i := range cur {
							if cur[i].id == t.id {
								setTodos(append(cur[:i], cur[i+1:]...))
								break
							}
						}
					}),
					Text("✕"),
				),
			)
		}, VirtualListHeight(300))

		counter := hooks.UseScope(func() core.Node {
			items := todos.Get()
			done := 0
			for _, t := range items {
				if t.completed {
					done++
				}
			}
			return P(Text(fmt.Sprintf("%d/%d completed", done, len(items))))
		}, todos)

		return Div(
			H2(Text("Todo List")),
			Form(
				OnSubmit(func(vals map[string]string) {
					text := vals["todo"]
					if text == "" {
						return
					}
					setTodos(append([]todo{{
						id: len(todos.Get()) + 1, text: text,
					}}, todos.Get()...))
				}),
				Input(
					Type("text"), Name("todo"), Placeholder("What needs to be done?"),
				),
				Button(Text("Add")),
			),
			counter,
			todoList,
		)
	})
}

func stopwatchPage() core.Node {
	return core.Component("StopwatchPage", func() core.Node {
		elapsed, setElapsed := hooks.UseState(0)
		running, setRunning := hooks.UseState(false)

		display := hooks.UseScope(func() core.Node {
			e := elapsed.Get()
			tenths := e % 10
			whole := e / 10
			secs := whole % 60
			mins := whole / 60
			return P(
				Style("font-size:2rem;font-family:monospace;"),
				Text(fmt.Sprintf("%02d:%02d.%d", mins, secs, tenths)),
			)
		}, elapsed)

		go func() {
			for {
				time.Sleep(100 * time.Millisecond)
				if running.Get() {
					setElapsed(elapsed.Get() + 1)
				}
			}
		}()

		return Div(
			H2(Text("Stopwatch")),
			display,
			Button(
				OnClick(func() { setRunning(!running.Get()) }),
				Text(func() string {
					if running.Get() {
						return "Pause"
					}
					return "Start"
				}()),
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

		selectedLabel := hooks.UseScope(func() core.Node {
			idx := selected.Get()
			if idx < 0 || idx >= len(data.Get()) {
				return P(Text("No row selected."))
			}
			row := data.Get()[idx]
			return P(Text(fmt.Sprintf("Selected: %s = %d%s", row.Label, row.Value, row.Unit)))
		}, selected, data)

		total := hooks.UseScope(func() core.Node {
			sum := 0
			for _, r := range data.Get() {
				sum += r.Value
			}
			return P(Text(fmt.Sprintf("Total: %d", sum)))
		}, data)

		return Div(
			H2(Text("Dashboard")),
			total,
			Table(
				Style("border-collapse:collapse;width:100%;max-width:500px;"),
				Thead(
					Tr(
						Th(Style("text-align:left;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Label")),
						Th(Style("text-align:right;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Value")),
						Th(Style("text-align:left;padding:4px 8px;border-bottom:2px solid #ccc;"), Text("Unit")),
					),
				),
				Tbody(childrenToItems(dashboardRows(data, selected, setSelected))...),
			),
			selectedLabel,
			Button(
				OnClick(func() {
					cur := data.Get()
					for i := range cur {
						cur[i].Value = (cur[i].Value*7 + 13) % 100
					}
					setData(cur)
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

func childrenToItems(children []core.Node) []core.Item {
	items := make([]core.Item, len(children))
	for i, c := range children {
		items[i] = nodeItem{c}
	}
	return items
}

type nodeItem struct {
	node core.Node
}

func (n nodeItem) Apply(el *core.ElementNode) {
	if n.node == nil {
		return
	}
	el.Children = append(el.Children, n.node)
}
