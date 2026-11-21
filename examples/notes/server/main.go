// Command server serves the notes example: server-rendered pages
// (ssr.Handler), the API (goowee/rpc) and the client's static files.
//
//	GOOS=js GOARCH=wasm go build -o examples/notes/static/main.wasm ./examples/notes
//	go run ./examples/notes/server -static examples/notes/static
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yogisalomo/goowee/examples/notes/api"
	"github.com/yogisalomo/goowee/examples/notes/app"
	"github.com/yogisalomo/goowee/rpc"
	"github.com/yogisalomo/goowee/ssr"
)

// store is the in-memory notes database.
type store struct {
	mu    sync.Mutex
	notes map[int]api.Note
	next  int
}

func newStore() *store {
	s := &store{notes: map[int]api.Note{}, next: 1}
	for _, n := range []api.Note{
		{Title: "Groceries", Body: "Milk, eggs, bread."},
		{Title: "Ideas", Body: "A goowee island on the docs site."},
	} {
		s.add(n)
	}
	return s
}

func (s *store) add(n api.Note) api.Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	n.ID = s.next
	s.next++
	s.notes[n.ID] = n
	return n
}

// Routes registers the API on mux.
func (s *store) Routes(mux *http.ServeMux) {
	rpc.Handle(mux, api.ListNotes, func(ctx context.Context, req api.ListReq) ([]api.Note, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []api.Note{}
		for _, n := range s.notes {
			if strings.Contains(strings.ToLower(n.Title), strings.ToLower(req.Query)) {
				out = append(out, n)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	rpc.Handle(mux, api.GetNote, func(ctx context.Context, req api.GetReq) (api.Note, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		n, ok := s.notes[req.ID]
		if !ok {
			return api.Note{}, rpc.Errorf(rpc.CodeNotFound, "no note %d", req.ID)
		}
		return n, nil
	})
	rpc.Handle(mux, api.AddNote, func(ctx context.Context, req api.AddReq) (api.Note, error) {
		if strings.TrimSpace(req.Title) == "" {
			return api.Note{}, rpc.Invalid("the note needs a title", map[string]string{"title": "Give the note a title."})
		}
		return s.add(api.Note{Title: req.Title, Body: req.Body}), nil
	})
}

func document(w io.Writer, head, body string) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
%s
<script src="/wasm_exec.js"></script><script src="/goowee.js"></script>
</head><body><div id="root">%s</div><script>goowee.boot({wasm: "/main.wasm"});</script></body></html>
`, head, body)
}

func main() {
	addr := flag.String("addr", "localhost:8090", "listen address")
	static := flag.String("static", "examples/notes/static", "directory with main.wasm, wasm_exec.js and goowee.js")
	flag.Parse()

	mux := http.NewServeMux()
	newStore().Routes(mux)
	files := http.FileServer(http.Dir(*static))
	pages := ssr.Handler(ssr.HandlerOptions{Page: ssr.RoutedPage(app.App), Document: document})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if fi, err := os.Stat(filepath.Join(*static, filepath.Clean("/"+r.URL.Path))); err == nil && !fi.IsDir() {
			if strings.HasSuffix(r.URL.Path, ".wasm") {
				w.Header().Set("Content-Type", "application/wasm")
			}
			files.ServeHTTP(w, r)
			return
		}
		pages.ServeHTTP(w, r)
	})
	log.Printf("notes on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
