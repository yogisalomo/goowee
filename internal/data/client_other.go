//go:build !(js && wasm)

package data

import "encoding/json"

// loadPage: outside the browser the client store has no page to read.
func loadPage() map[string]json.RawMessage { return nil }
