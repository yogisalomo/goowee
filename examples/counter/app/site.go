package app

import (
	"strings"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

const repoURL = "https://github.com/yogisalomo/goowee"

// heroCode is the source shown in the hero, line for line the component
// running beside it (heroDemo), so "built with goowee" is literal.
const heroCode = `func Counter() core.Node {
    return core.Component("Counter", func() core.Node {
        count, setCount := hooks.UseState(0)
        return Div(
            P(Textf("%d", count)),
            Button(Text("increment"),
                OnClick(func() { setCount(count.Get() + 1) })),
            Button(Text("reset"),
                OnClick(func() { setCount(0) })),
        )
    })
}`

func landingPage(r *router.Router) core.Node {
	return core.Component("Landing", func() core.Node {
		return Div(
			hero(r),
			principles(),
			getStarted(r),
			feedback(),
		)
	})
}

func hero(r *router.Router) core.Node {
	return Section(Class("hero"),
		Div(Class("hero-inner"),
			H1(Text("Reactive web UIs, written in Go.")),
			P(Class("lead"), rich("goowee compiles to WebAssembly, renders on the server and hydrates in the browser. "+
				"When a signal changes, only the nodes that read it update: no virtual DOM, and no JavaScript to write.")),
			heroDemo(),
			Div(Class("cta-row"),
				r.Link("/tutorial", "Start the tutorial", Class("btn btn-primary")),
				A(Class("btn"), Href(repoURL), Target("_blank"), Rel("noopener"), Text("Source on GitHub")),
			),
		),
	)
}

// heroDemo is a running goowee component next to its own source. Each click
// highlights the binding that re-ran — the Textf line — and the text node it
// changed, the one moment of motion on the page.
func heroDemo() core.Node {
	return core.Component("HeroDemo", func() core.Node {
		count, setCount := hooks.UseState(0)
		count.Named("hero.count") // in ?goowee-dev's inspector and change log
		updates, setUpdates := hooks.UseState(0)
		// Alternating between two identical animations restarts the flash on
		// every update.
		flash := func(base string) *core.Signal[string] {
			return core.Computed([]core.SignalAccessor{updates}, func() string {
				switch n := updates.Get(); {
				case n == 0:
					return base
				case n%2 == 1:
					return base + " hit-a"
				default:
					return base + " hit-b"
				}
			})
		}
		lineClass, countClass := flash("hl"), flash("count")
		set := func(v int) {
			if v != count.Get() {
				setUpdates(updates.Get() + 1)
			}
			setCount(v)
		}

		var lines []core.Item
		for i, line := range strings.Split(heroCode, "\n") {
			if i > 0 {
				lines = append(lines, Text("\n"))
			}
			if strings.Contains(line, "Textf(") {
				code := strings.TrimLeft(line, " ")
				lines = append(lines, Text(line[:len(line)-len(code)]), Span(ClassS(lineClass), Text(code)))
			} else {
				lines = append(lines, Text(line))
			}
		}
		return Div(Class("play"),
			Div(Class("play-source"),
				P(Class("file"), Text("counter.go")),
				Pre(Class("code"), Code(lines...)),
			),
			Div(Class("play-output"),
				P(Class("file"), Text("running in this page")),
				P(ClassS(countClass), Textf("%d", count)),
				Div(Class("row"),
					Button(OnClick(func() { set(count.Get() + 1) }), Text("increment")),
					Button(OnClick(func() { set(0) }), Text("reset")),
				),
				P(Class("play-note"), Text("The component ran once. A click re-runs only the highlighted binding, which changes one text node.")),
			),
		)
	})
}

func principles() core.Node {
	row := func(term, body string) core.Node {
		return Div(Class("principle"),
			Dt(Text(term)),
			Dd(rich(body)),
		)
	}
	return Section(Class("section"),
		H2(Text("How it works")),
		Dl(Class("principles"),
			row("Components run once.",
				"A component sets up its state and returns a tree. It isn't called again when that state changes."),
			row("Signals update what reads them.",
				"`Textf`, `BindValue` and the other bindings subscribe one node each. A write changes those nodes and nothing else; there is no diffing pass."),
			row("The server renders first.",
				"`ssr.Handler` sends real HTML. In the browser, goowee claims those nodes instead of building them again."),
			row("It's Go on both sides.",
				"Types, validation and logic are shared by server and browser and checked by the compiler. `gooweetest` tests components without a browser."),
		),
	)
}

func getStarted(r *router.Router) core.Node {
	return Section(Class("section start"),
		H2(Text("Get started")),
		P(rich("Add the module, write a component, and build it for the browser with `GOOS=js GOARCH=wasm`.")),
		Pre(Class("code"), Code(Text(`go get github.com/yogisalomo/goowee@latest
GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app`))),
		P(Class("links"),
			r.Link("/getting-started", "Read the setup guide"),
			r.Link("/tutorial", "Work through the examples"),
		),
	)
}

func feedback() core.Node {
	return Section(Class("section feedback"),
		H2(Text("Tell us what's missing.")),
		P(Text("goowee is v0, and the API will change. What would make it useful for what you're building?")),
		// A plain GET form to GitHub's new-issue page: no JS, prefilled title/body.
		Form(Action(repoURL+"/issues/new"), Method("get"), Target("_blank"),
			Label(HtmlFor("fb-title"), Text("Summary")),
			Input(ID("fb-title"), Type("text"), Name("title"), Placeholder("A short summary")),
			Label(HtmlFor("fb-body"), Text("Details")),
			Textarea(ID("fb-body"), Name("body"), Placeholder("What are you building? What's rough or missing?")),
			Button(Class("btn btn-primary"), Type("submit"), Text("Open an issue on GitHub")),
		),
	)
}

func footer() core.Node {
	return Footer(Class("footer"),
		Div(Class("footer-inner"),
			P(Text("goowee is v0: the API may still change.")),
			P(Class("footer-links"),
				A(Href(repoURL), Target("_blank"), Rel("noopener"), Text("GitHub")),
				A(Href(repoURL+"/blob/main/LICENSE"), Target("_blank"), Rel("noopener"), Text("MIT license")),
				A(Href("https://go.dev/blog/go-fonts"), Target("_blank"), Rel("noopener"), Text("Set in the Go fonts")),
			),
		),
	)
}

func tutorialIndex(r *router.Router) core.Node {
	desc := map[string]string{
		"/getting-started": "Install, project layout, hello world.",
		"/counter":         "State, signals and reactive text.",
		"/form":            "Two-way binding with `BindValue`, submit, refs, file inputs.",
		"/todos":           "Keyed lists and `VirtualList`.",
		"/dashboard":       "`Computed` values and selection.",
		"/stopwatch":       "`OnMount` and a timer off the render loop.",
		"/async":           "`UseResource`: load data in a goroutine.",
		"/error":           "Recover from a render panic and show a fallback.",
		"/greet/alice":     "URL params, read reactively.",
		"/events":          "Bubbling, non-bubbling events, `PreventDefault`.",
		"/canvas":          "Draw thousands of marks from Go with `goowee/canvas`.",
		"/ai":              "Rules to paste into Claude Code, Cursor and other agents.",
	}
	return core.Component("Tutorial", func() core.Node {
		items := make([]core.Node, len(tutorialSteps))
		for i, st := range tutorialSteps {
			items[i] = Li(
				r.LinkTo(st.path,
					Span(Class("n"), Text(st.n)),
					Span(Class("t"), Text(st.title)),
				),
				P(Class("d"), rich(desc[st.path])),
			)
		}
		return Div(Class("page"),
			H1(Text("Learn goowee by example")),
			P(Class("lead"), rich("Each lesson is a small goowee app running in this page. Try it, read the code under it, "+
				"then find the full source in `examples/counter/app`.")),
			Ol(Class("steps"), Group(Nodes(items)...)),
		)
	})
}
