//go:build !goowee_nodev

package dom

import "github.com/yogisalomo/goowee/devtools"

// devMode reports whether dev-only diagnostics are on (?goowee-dev); a
// variable so tests can switch it. A goowee_nodev build has none
// (devmode_nodev.go), and doesn't link goowee/devtools.
var devMode = devtools.Enabled
