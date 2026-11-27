//go:build !(js && wasm)

package app

import "github.com/yogisalomo/goowee/core"

func chartUnavailable() bool { return false }

// drawBars is browser-only (see chart_js.go); SSR and host tests skip it.
func drawBars(ref *core.Ref, values []int) {}
