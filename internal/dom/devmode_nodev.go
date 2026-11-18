//go:build goowee_nodev

package dom

// devMode is always false in a goowee_nodev build (#127).
var devMode = func() bool { return false }
