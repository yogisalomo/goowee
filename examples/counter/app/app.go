package app

import (
	"fmt"
	"goowee/core"
	"goowee/html"
	"goowee/hooks"
	"goowee/router"
)

func App(r *router.Router) core.Node {
	return core.Component("App", func() core.Node {
		return Layout(r,
			r.Route(map[string]func() core.Node{
				"/":        homePage,
				"/counter": counterPage,
				"/about":   aboutPage,
				"/form":    formPage,
				"/todos":   todosPage,
			}),
		)
	})
}

func Layout(r *router.Router, children ...core.Node) core.Node {
	return html.Div(html.Props{"class": "app"},
		Header(r),
		html.Main(nil, children...),
	)
}

func Header(r *router.Router) core.Node {
	return html.Nav(html.Props{"class": "nav"},
		r.Link("/", "Home"),
		html.Text(" | "),
		r.Link("/counter", "Counter"),
		html.Text(" | "),
		r.Link("/about", "About"),
		html.Text(" | "),
		r.Link("/form", "Form"),
		html.Text(" | "),
		r.Link("/todos", "Todos"),
	)
}

func homePage() core.Node {
	return &core.ElementNode{
		Tag:   "h1",
		Props: map[string]any{"textContent": "Welcome to Goowee"},
	}
}

func counterPage() core.Node {
	return core.Component("CounterPage", func() core.Node {
		count, setCount := hooks.UseState(0)
		show, setShow := hooks.UseState(true)

		greeting := hooks.UseScope(func() core.Node {
			if show.Get() {
				return &core.ElementNode{
					Tag:   "p",
					Props: map[string]any{"textContent": "Hello!", "class": "greeting"},
				}
			}
			return &core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": "(hidden)", "class": "hidden"},
			}
		}, show)

		return &core.ElementNode{
			Tag: "div",
			Children: []core.Node{
				&core.ElementNode{
					Tag: "button",
					Props: map[string]any{
						"textContent": count,
						"onclick": func(ed core.EventData) {
							setCount(count.Get() + 1)
						},
					},
				},
				&core.ElementNode{
					Tag: "button",
					Props: map[string]any{
						"textContent": "Toggle",
						"onclick": func(ed core.EventData) {
							setShow(!show.Get())
						},
					},
				},
				greeting,
			},
		}
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
			return &core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": "Name: " + name.Get() + ", Email: " + email.Get()},
			}
		}, name, email)

		subList := hooks.UseScope(func() core.Node {
			return submissionsList(entries.Get())
		}, entries)

		return &core.ElementNode{
			Tag: "div",
			Children: []core.Node{
				&core.ElementNode{
					Tag:   "h2",
					Props: map[string]any{"textContent": "Form Demo"},
				},
				&core.ElementNode{
					Tag: "form",
					Props: map[string]any{
						"onsubmit": func(ed core.EventData) {
							raw, ok := ed.Data["values"]
							if !ok {
								return
							}
							vals, ok := raw.(map[string]any)
							if !ok {
								return
							}
							en := entry{
								name:   stringFrom(vals, "name"),
								email:  stringFrom(vals, "email"),
								agreed: vals["agreed"] == "on",
							}
							setEntries(append(entries.Get(), en))
							setName("")
							setEmail("")
							setAgreed(false)
						},
					},
					Children: []core.Node{
						label("Name", &core.ElementNode{
							Tag: "input",
							Props: map[string]any{
								"type":  "text",
								"name":  "name",
								"value": name,
								"oninput": func(ed core.EventData) {
									if v, ok := ed.Data["value"].(string); ok {
										setName(v)
									}
								},
							},
						}),
						label("Email", &core.ElementNode{
							Tag: "input",
							Props: map[string]any{
								"type":  "email",
								"name":  "email",
								"value": email,
								"oninput": func(ed core.EventData) {
									if v, ok := ed.Data["value"].(string); ok {
										setEmail(v)
									}
								},
							},
						}),
						&core.ElementNode{
							Tag: "label",
							Children: []core.Node{
								&core.ElementNode{
									Tag: "input",
									Props: map[string]any{
										"type":    "checkbox",
										"name":    "agreed",
										"checked": agreed,
										"oninput": func(ed core.EventData) {
											if v, ok := ed.Data["checked"].(bool); ok {
												setAgreed(v)
											}
										},
									},
								},
								&core.TextNode{Value: " Subscribe to newsletter"},
							},
						},
						&core.ElementNode{
							Tag: "button",
							Props: map[string]any{
								"textContent": "Submit",
							},
						},
					},
				},
				&core.ElementNode{
					Tag:   "h3",
					Props: map[string]any{"textContent": "Preview"},
				},
				preview,
				&core.ElementNode{
					Tag:   "h3",
					Props: map[string]any{"textContent": "Submissions"},
				},
				subList,
			},
		}
	})
}

func label(text string, input core.Node) core.Node {
	return &core.ElementNode{
		Tag: "label",
		Children: []core.Node{
			&core.TextNode{Value: text + " "},
			input,
			&core.TextNode{Value: "\n"},
		},
	}
}

func submissionsList(entries []entry) core.Node {
	if len(entries) == 0 {
		return &core.ElementNode{
			Tag:   "p",
			Props: map[string]any{"textContent": "No submissions yet."},
		}
	}
	var items []core.Node
	for _, en := range entries {
		agreed := "no"
		if en.agreed {
			agreed = "yes"
		}
		items = append(items, &core.ElementNode{
			Tag: "li",
			Children: []core.Node{
				&core.TextNode{Value: en.name + " — " + en.email + " (subscribed: " + agreed + ")"},
			},
		})
	}
	return &core.ElementNode{
		Tag:      "ul",
		Children: items,
	}
}

func stringFrom(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

type todo struct {
	id        int
	text      string
	completed bool
}

func todosPage() core.Node {
	return core.Component("TodosPage", func() core.Node {
		input, setInput := hooks.UseState("")
		initial := make([]todo, 100)
		for i := range initial {
			initial[i] = todo{id: i + 1, text: fmt.Sprintf("Item %d", i+1)}
		}
		todos, setTodos := hooks.UseState(initial)
		nextID, setNextID := hooks.UseState(101)

		todoList := html.VirtualList(todos, 48, func(i int, t todo) core.Node {
			return html.Li(html.Props{
				"class": func() string {
					if t.completed {
						return "completed"
					}
					return ""
				}(),
				"style": "display:flex;align-items:center;gap:8px;padding:4px 8px;height:48px;box-sizing:border-box;",
			},
				html.Input(html.Props{
					"type": "checkbox", "checked": t.completed,
					"oninput": func(ed core.EventData) {
						cur := todos.Get()
						for i := range cur {
							if cur[i].id == t.id {
								cur[i].completed = !cur[i].completed
								break
							}
						}
						setTodos(cur)
					},
				}),
				html.Span(html.Props{"textContent": t.text}),
				html.Button(html.Props{
					"textContent": "✕",
					"onclick": func(ed core.EventData) {
						cur := todos.Get()
						for i := range cur {
							if cur[i].id == t.id {
								setTodos(append(cur[:i], cur[i+1:]...))
								break
							}
						}
					},
				}),
			)
		}, html.VirtualListHeight(300))

		counter := hooks.UseScope(func() core.Node {
			items := todos.Get()
			done := 0
			for _, t := range items {
				if t.completed {
					done++
				}
			}
			return html.P(html.Props{
				"textContent": fmt.Sprintf("%d/%d completed", done, len(items)),
			})
		}, todos)

		return html.Div(nil,
			html.H2(html.Props{"textContent": "Todo List"}),
			html.Form(html.Props{
				"onsubmit": func(ed core.EventData) {
					raw, ok := ed.Data["values"]
					if !ok {
						return
					}
					vals, ok := raw.(map[string]any)
					if !ok {
						return
					}
					text, ok := vals["todo"].(string)
					if !ok || text == "" {
						return
					}
					setTodos(append([]todo{{id: nextID.Get(), text: text}}, todos.Get()...))
					setNextID(nextID.Get() + 1)
					setInput("")
				},
			},
				html.Input(html.Props{
					"type": "text", "name": "todo", "placeholder": "What needs to be done?",
					"value": input,
					"oninput": func(ed core.EventData) {
						if v, ok := ed.Data["value"].(string); ok {
							setInput(v)
						}
					},
				}),
				html.Button(html.Props{"textContent": "Add"}),
			),
			counter,
			todoList,
		)
	})
}

func aboutPage() core.Node {
	return &core.ElementNode{
		Tag: "div",
		Children: []core.Node{
			&core.ElementNode{
				Tag:   "h2",
				Props: map[string]any{"textContent": "About"},
			},
			&core.ElementNode{
				Tag:   "p",
				Props: map[string]any{"textContent": "A minimal Go WASM signal-based UI framework."},
			},
		},
	}
}
