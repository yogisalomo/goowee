// Package app is the notes example (#151): a list, a detail page and a form,
// talking to its server through typed server functions (goowee/rpc).
package app

import (
	"context"
	"strconv"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/examples/notes/api"
	"github.com/yogisalomo/goowee/form"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/router"
	"github.com/yogisalomo/goowee/rpc"
)

// App is the whole app; the server renders it too (ssr.RoutedPage).
func App(r *router.Router) core.Node {
	return Div(Class("notes-app"),
		Metadata(Title("Notes")),
		Header(r.Link("/", "Notes"), Text(" · "), r.Link("/new", "New note", ID("new-link"))),
		Main(r.Route(map[string]func() core.Node{
			"/":          func() core.Node { return listPage(r) },
			"/notes/:id": func() core.Node { return notePage(r) },
			"/new":       func() core.Node { return newPage(r) },
		})),
	)
}

func listPage(r *router.Router) core.Node {
	return core.Component("NoteList", func() core.Node {
		query := core.NewSignal(api.ListReq{})
		notes := rpc.UseQuery(api.ListNotes, query)
		return Section(
			H1(Text("Notes")),
			Input(ID("search"), Placeholder("Filter titles"), OnInput(func(v string) { query.Set(api.ListReq{Query: v}) })),
			ShowResource(notes,
				func() core.Node { return P(Class("loading"), Text("Loading…")) },
				func(err error) core.Node { return P(Class("error"), Text(err.Error())) },
				func(list *core.Signal[[]api.Note]) core.Node {
					return Ul(ID("notes"), For(list, func(n api.Note) any { return n.ID }, func(n api.Note) core.Node {
						return Li(r.Link("/notes/"+strconv.Itoa(n.ID), n.Title))
					}))
				},
			),
		)
	})
}

func notePage(r *router.Router) core.Node {
	return core.Component("NoteDetail", func() core.Node {
		id := r.ParamSignal("id")
		req := core.Computed([]core.SignalAccessor{id}, func() api.GetReq {
			n, _ := strconv.Atoi(id.Get())
			return api.GetReq{ID: n}
		})
		note := rpc.UseQuery(api.GetNote, req)
		return Section(
			ShowResource(note,
				func() core.Node { return P(Class("loading"), Text("Loading…")) },
				func(err error) core.Node { return P(Class("error"), ID("note-error"), Text(err.Error())) },
				func(n *core.Signal[api.Note]) core.Node {
					return Article(ID("note"),
						H1(TextS(core.Computed([]core.SignalAccessor{n}, func() string { return n.Get().Title }))),
						P(TextS(core.Computed([]core.SignalAccessor{n}, func() string { return n.Get().Body }))),
					)
				},
			),
		)
	})
}

func newPage(r *router.Router) core.Node {
	return core.Component("NewNote", func() core.Node {
		// The form checks api.AddReq.Validate as the user types; the server
		// checks it again, and its field errors show here too.
		f := form.Use(api.AddReq{})
		return Section(
			H1(Text("New note")),
			Form(ID("new-note"),
				f.OnSubmit(func(ctx context.Context, req api.AddReq) error {
					note, err := api.AddNote.Call(ctx, req)
					if err == nil {
						core.Schedule(func() { r.Navigate("/notes/" + strconv.Itoa(note.ID)) })
					}
					return err
				}),
				Label(Text("Title "), Input(ID("title"), form.Text(f, "title", func(n *api.AddReq) *string { return &n.Title }))),
				P(Class("field-error"), ID("title-error"), TextS(f.Error("title"))),
				Label(Text("Body "), Textarea(ID("body"), form.Text(f, "body", func(n *api.AddReq) *string { return &n.Body }))),
				P(Class("field-error"), ID("form-error"), TextS(f.Error(""))),
				Button(Type("submit"), DisabledS(f.Submitting), Text("Save")),
			),
		)
	})
}
