// Package data carries keyed resource data from a server render to the
// client (#152). A server render provides a per-request Store through
// Context; hooks.Load requests its key there in a first pass, the server runs
// the fetches, and a second pass renders with the results, which are written
// into the page as JSON. In the browser, Context's default store reads that
// JSON, so each Load starts with its data instead of fetching.
package data

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yogisalomo/goowee/core"
)

// ScriptID is the id of the <script type="application/json"> holding a
// page's data.
const ScriptID = "goowee-data"

// Store holds the data of one server render, or the data a page came with on
// the client.
type Store struct {
	mu      sync.Mutex
	server  bool
	pending map[string]func(ctx context.Context) (any, error)
	results map[string]json.RawMessage
	load    func() map[string]json.RawMessage // client: reads the page's data once
	loaded  bool
}

// Context gives hooks.Load the store of the render it is in. With no
// provider (the browser, a plain ssr.Renderer) it is the client store.
var Context = core.NewContext(client)

// client is the browser's store: the data the page came with.
var client = &Store{}

// SetPageData replaces the client store's data, as if the page had come with
// it: for tests of components that use hooks.Load. It returns a function
// that restores the previous state.
func SetPageData(entries map[string]json.RawMessage) (restore func()) {
	client.mu.Lock()
	prevResults, prevLoaded := client.results, client.loaded
	client.results, client.loaded = entries, true
	client.mu.Unlock()
	return func() {
		client.mu.Lock()
		client.results, client.loaded = prevResults, prevLoaded
		client.mu.Unlock()
	}
}

// Provided is s as the value of Context, for a renderer to provide to the
// whole tree (walker.Provided) without a provider node.
func Provided(s *Store) *core.ContextValue {
	return &core.ContextValue{Key: Context, Value: core.NewSignal(s)}
}

// NewServer returns the store for one server render.
func NewServer() *Store {
	return &Store{server: true, pending: map[string]func(context.Context) (any, error){}, results: map[string]json.RawMessage{}}
}

// Server reports whether this is a server render's store.
func (s *Store) Server() bool { return s.server }

// Request records a keyed fetch for the server to run before the next pass.
// A key already requested or fetched keeps its first fetch.
func (s *Store) Request(key string, fetch func(ctx context.Context) (any, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, done := s.results[key]; done {
		return
	}
	if _, ok := s.pending[key]; !ok {
		s.pending[key] = fetch
	}
}

// Pending reports whether fetches wait to run.
func (s *Store) Pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending) > 0
}

// Run runs the pending fetches concurrently, each with ctx and the deadline,
// and keeps the JSON of those that succeed. A failed or late fetch is left
// out: the page renders its loading state, and the client fetches.
func (s *Store) Run(ctx context.Context, timeout time.Duration) {
	s.mu.Lock()
	pending := s.pending
	s.pending = map[string]func(context.Context) (any, error){}
	s.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var wg sync.WaitGroup
	for key, fetch := range pending {
		wg.Add(1)
		go func(key string, fetch func(context.Context) (any, error)) {
			defer wg.Done()
			defer func() { _ = recover() }() // a panicking fetch is a failed one
			v, err := fetch(ctx)
			if err != nil || ctx.Err() != nil {
				return
			}
			raw, err := json.Marshal(v)
			if err != nil {
				return
			}
			s.mu.Lock()
			s.results[key] = raw
			s.mu.Unlock()
		}(key, fetch)
	}
	wg.Wait()
}

// Result returns the server's result for key.
func (s *Store) Result(key string) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.results[key]
	return raw, ok
}

// Take returns the data the page came with for key, once: a later Load of
// the same key (after navigating away and back) fetches afresh.
func (s *Store) Take(key string) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		s.loaded = true
		if s.load != nil {
			s.results = s.load()
		}
	}
	raw, ok := s.results[key]
	if ok {
		delete(s.results, key)
	}
	return raw, ok
}

// Script is the page's data as a <script type="application/json">, "" when
// there is none. "</" is escaped so the JSON can't close the element.
func (s *Store) Script() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return ""
	}
	data, err := json.Marshal(s.results)
	if err != nil {
		return ""
	}
	return `<script type="application/json" id="` + ScriptID + `">` +
		strings.ReplaceAll(string(data), "</", `<\/`) + `</script>`
}
