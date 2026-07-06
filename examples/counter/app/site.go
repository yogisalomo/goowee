package app

import (
	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/router"
)

const repoURL = "https://github.com/yogisalomo/goowee"

// heroCode is shown verbatim on the landing page — the same shape as the live
// demo beside it, so "built with goowee" is literal.
const heroCode = `func Counter() core.Node {
    return core.Component("Counter", func() core.Node {
        count, setCount := hooks.UseState(0)
        return Div(
            P(Textf("Count: %d", count)),
            Button(
                OnClick(func() { setCount(count.Get() + 1) }),
                Text("increment"),
            ),
        )
    })
}`

func landingPage(r *router.Router) core.Node {
	return core.Component("Landing", func() core.Node {
		return Div(
			hero(r),
			features(),
			codeSection(),
			feedback(),
		)
	})
}

func hero(r *router.Router) core.Node {
	return Div(Class("hero"),
		Div(Class("hero-copy"),
			H1(Text("Reactive web UIs, written in Go.")),
			P(Class("lead"), Text("goowee compiles to WebAssembly and renders on the server, then hydrates on the client. Signals drive fine-grained updates — no virtual DOM, no build step.")),
			Div(Class("cta-row"),
				A(Class("btn btn-primary"), Href("/tutorial"),
					OnClickE(func(core.EventData) { r.Navigate("/tutorial") }, PreventDefault()),
					Text("Get started")),
				A(Class("btn"), Href(repoURL), Target("_blank"), Rel("noopener"), Text("★ GitHub")),
			),
		),
		heroDemo(),
	)
}

// heroDemo is a real, running goowee component — the page's proof of itself.
func heroDemo() core.Node {
	return core.Component("HeroDemo", func() core.Node {
		count, setCount := hooks.UseState(0)
		show, setShow := hooks.UseState(true)
		return Div(Class("panel"),
			Div(Class("panel-bar"),
				Span(Class("dot")), Span(Class("dot")), Span(Class("dot")),
				Span(Class("live"), Text("live · built with goowee")),
			),
			Div(Class("panel-body"),
				Div(Class("demo"),
					Div(Class("count"), Textf("%d", count)),
					Div(Class("row"),
						demoButton("increment", func() { setCount(count.Get() + 1) }),
						demoButton("reset", func() { setCount(0) }),
						demoButton("toggle", func() { setShow(!show.Get()) }),
					),
					ShowElse(show,
						func() core.Node { return Span(Class("greeting"), Text("▸ this whole page is a goowee app")) },
						func() core.Node { return Span(Class("hidden"), Text("▸ (hidden)")) },
					),
				),
			),
		)
	})
}

func demoButton(label string, onClick func()) core.Node {
	return Button(OnClick(onClick), Text(label))
}

func features() core.Node {
	card := func(tag, title, body string) core.Node {
		return Div(Class("feature"),
			P(Class("tag"), Text(tag)),
			H3(Text(title)),
			P(Text(body)),
		)
	}
	return Div(Class("section-soft"),
		Div(Class("section"),
			P(Class("eyebrow"), Text("Why goowee")),
			H2(Text("A small, honest reactive model.")),
			Div(Class("features"),
				card("signals", "Fine-grained updates", "State is signals. A change updates only the nodes that read it — no re-render pass, no virtual DOM."),
				card("ssr", "SSR + hydration", "Render real HTML on the server, then hydrate: the client reuses that DOM instead of rebuilding it."),
				card("go", "One language, both sides", "Types, validation, and logic are plain Go — shared between server and browser, checked by the compiler."),
			),
		),
	)
}

func codeSection() core.Node {
	return Div(Class("section"),
		P(Class("eyebrow"), Text("The whole idea")),
		H2(Text("Components are functions that run once.")),
		P(Class("lead"), Style("max-width:52ch"), Text("Set up state and return a tree. Signals keep the DOM in sync from there — the demo above is exactly this.")),
		Div(Class("panel"),
			Div(Class("panel-bar"),
				Span(Class("dot")), Span(Class("dot")), Span(Class("dot")),
				Span(Style("margin-left:auto"), Text("counter.go")),
			),
			Pre(Class("code"), Text(heroCode)),
		),
	)
}

func feedback() core.Node {
	return Div(Class("section-soft"),
		Div(Class("section feedback"),
			P(Class("eyebrow"), Text("Feedback")),
			H2(Text("Tell us what's missing.")),
			P(Class("lead"), Text("goowee is v0 and the API will change. What would make it useful for what you're building?")),
			// A plain GET form to GitHub's new-issue page — no JS, prefilled title/body.
			Form(Action(repoURL+"/issues/new"), Method("get"), Target("_blank"),
				Label(Text("Summary")),
				Input(Type("text"), Name("title"), Placeholder("Short summary")),
				Label(Text("Details")),
				Textarea(Name("body"), Placeholder("What are you building? What's rough or missing?")),
				Button(Class("btn btn-primary"), Type("submit"), Text("Open an issue on GitHub")),
			),
		),
	)
}

func footer() core.Node {
	return Footer(Class("footer"),
		Div(Class("footer-inner"),
			Span(Class("install"), Text("go get "+repoURL[len("https://"):])),
			A(Href(repoURL), Target("_blank"), Rel("noopener"), Text("GitHub")),
			A(Href(repoURL+"/blob/main/LICENSE"), Target("_blank"), Rel("noopener"), Text("MIT")),
			Span(Class("sp"), Text("v0 · experimental")),
		),
	)
}

func tutorialIndex(r *router.Router) core.Node {
	link := func(n, path, title, desc string) core.Node {
		return Li(
			A(Href(path), OnClickE(func(core.EventData) { r.Navigate(path) }, PreventDefault()),
				Span(Class("n"), Text(n)),
				Span(Text(title)),
				Span(Class("d"), Text("— "+desc)),
			),
		)
	}
	return core.Component("Tutorial", func() core.Node {
		return Div(Class("page"),
			P(Class("eyebrow"), Text("Tutorial")),
			H1(Text("Learn goowee by example")),
			P(Class("lead"), Style("max-width:52ch"), Text("Each example is a live goowee app. Read the page, click around, then open the source under examples/counter/app.")),
			Ul(Class("steps"),
				link("01", "/counter", "Counter", "state, signals, reactive text"),
				link("02", "/form", "Form", "two-way binding and submit"),
				link("03", "/todos", "Todos", "keyed lists and virtualization"),
				link("04", "/dashboard", "Dashboard", "computed values and selection"),
				link("05", "/stopwatch", "Stopwatch", "OnMount and an off-loop timer"),
				link("06", "/greet/alice", "Greeting", "URL params, read reactively"),
			),
		)
	})
}
