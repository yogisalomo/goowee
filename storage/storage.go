// Package storage is the browser's localStorage and sessionStorage, usable
// outside the browser too: in a native build each is an in-memory store, so a
// component that remembers a draft or a preference stays testable with
// gooweetest (which clears both when a test ends).
//
//	hooks.OnMount(func() func() {
//	    if draft, ok := storage.Local.Get("draft"); ok {
//	        setDraft(draft)
//	    }
//	    return nil
//	})
//	hooks.Watch([]core.SignalAccessor{draft}, func() { storage.Local.Set("draft", draft.Get()) })
//
// Storage is client state: during a server render reads find nothing and
// writes do nothing (the server has no browser storage, and a process-wide
// store would leak between requests), so read it in OnMount, not in setup,
// or the server and client markup differ. hooks.UsePersist(sig, key) does all
// of the above for a signal.
package storage

import (
	"errors"
	"sort"
	"sync"

	"github.com/yogisalomo/goowee/internal/runtime"
)

// ErrUnavailable is returned by Set when the browser refuses storage (a
// sandboxed frame, storage disabled by the user).
var ErrUnavailable = errors.New("goowee/storage: storage is unavailable in this browser context")

// Store is one storage area. Its methods are safe to call from any goroutine.
type Store struct {
	area string // "localStorage" or "sessionStorage"

	mu  sync.Mutex
	mem map[string]string // the native build's store
}

var (
	// Local persists across reloads and tabs of the same origin.
	Local = &Store{area: "localStorage"}
	// Session lasts as long as the tab.
	Session = &Store{area: "sessionStorage"}
)

// Get returns the value stored under key and whether there is one.
func (s *Store) Get(key string) (string, bool) {
	if serverRender() {
		return "", false
	}
	return s.get(key)
}

// Set stores value under key. It fails when the browser refuses (the quota is
// full, or storage is disabled).
func (s *Store) Set(key, value string) error {
	if serverRender() {
		return nil
	}
	return s.set(key, value)
}

// Remove deletes key.
func (s *Store) Remove(key string) {
	if !serverRender() {
		s.remove(key)
	}
}

// Clear deletes every key in the area.
func (s *Store) Clear() {
	if !serverRender() {
		s.clear()
	}
}

// Keys returns the stored keys, sorted.
func (s *Store) Keys() []string {
	if serverRender() {
		return nil
	}
	keys := s.keys()
	sort.Strings(keys)
	return keys
}

func serverRender() bool { return runtime.CurrentEnv() == runtime.EnvServer }
