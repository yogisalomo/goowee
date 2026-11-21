// Package runtime embeds the browser-side goowee script so a host can serve
// the exact runtime that matches this build. Package web is the public face.
package runtime

import _ "embed"

//go:embed goowee.js
var js []byte

// JS returns a copy of the embedded goowee.js.
func JS() []byte {
	return append([]byte(nil), js...)
}
