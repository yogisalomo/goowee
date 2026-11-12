package hooks

import (
	"encoding/json"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/storage"
)

// PersistOptions configures UsePersist.
type PersistOptions struct {
	// Store is where the value is kept: storage.Local (the default) survives
	// reloads and is shared by the origin's tabs; storage.Session lasts as
	// long as the tab.
	Store *storage.Store
}

// UsePersist keeps sig in browser storage under key, so it survives a reload.
// Once the component has mounted it loads the stored value into sig, then
// saves sig, as JSON, each time it changes:
//
//	hooks.UsePersist(store.Cart, "cart")
//
// Loading waits for the mount so that a server-rendered page hydrates with the
// value the server rendered (the server has no browser storage); the stored
// value replaces it right after, so a page can show the default for a frame.
// A stored value that no longer decodes, say after the type changed, is
// logged and left alone: sig keeps its value until it next changes and is
// saved over it. Name a new key ("cart.v2") to start fresh instead.
//
// Persisting lasts as long as the component, so call it where the signal
// lives: the root component for an app-wide store. On the server it does
// nothing.
func UsePersist[T any](sig *core.Signal[T], key string, opts ...PersistOptions) {
	area := storage.Local
	if len(opts) > 0 && opts[0].Store != nil {
		area = opts[0].Store
	}
	OnMount(func() func() {
		if raw, ok := area.Get(key); ok {
			var v T
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				core.Log(core.LogWarn, "a stored value doesn't decode; keeping the signal's value (UsePersist)", map[string]any{
					"key":   key,
					"error": err.Error(),
				})
			} else {
				sig.Set(v)
			}
		}
		return sig.Subscribe(func() {
			data, err := json.Marshal(sig.Peek())
			if err == nil {
				err = area.Set(key, string(data))
			}
			if err != nil {
				core.Log(core.LogWarn, "couldn't save a persisted signal (UsePersist)", map[string]any{
					"key":   key,
					"error": err.Error(),
				})
			}
		})
	})
}
