// Package api is the notes example's server functions (goowee/rpc): one
// definition each, imported by the server and by the WASM client.
package api

import (
	"strings"

	"github.com/yogisalomo/goowee/form"
	"github.com/yogisalomo/goowee/rpc"
)

// Note is a note as both sides see it.
type Note struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// ListReq asks for the notes; Query filters titles (empty for all).
type ListReq struct {
	Query string `json:"query"`
}

// GetReq asks for one note.
type GetReq struct {
	ID int `json:"id"`
}

// AddReq is the new-note form.
type AddReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Validate checks a new note. The form runs it in the browser as the user
// types, and the server runs it again on what it receives (goowee/form, #150).
func (r AddReq) Validate() form.Errors {
	var errs form.Errors
	if strings.TrimSpace(r.Title) == "" {
		errs = errs.Add("title", "Give the note a title.")
	} else if len(r.Title) > 80 {
		errs = errs.Add("title", "Keep the title under 80 characters.")
	}
	return errs
}

var (
	ListNotes = rpc.Define[ListReq, []Note]("notes.list")
	GetNote   = rpc.Define[GetReq, Note]("notes.get")
	AddNote   = rpc.Define[AddReq, Note]("notes.add")
)
