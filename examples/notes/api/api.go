// Package api is the notes example's server functions (goowee/rpc): one
// definition each, imported by the server and by the WASM client.
package api

import "github.com/yogisalomo/goowee/rpc"

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

var (
	ListNotes = rpc.Define[ListReq, []Note]("notes.list")
	GetNote   = rpc.Define[GetReq, Note]("notes.get")
	AddNote   = rpc.Define[AddReq, Note]("notes.add")
)
