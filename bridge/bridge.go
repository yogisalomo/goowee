//go:build !(js && wasm)

package bridge

import "github.com/yogisalomo/goowee/core"

// Run is the client entry point and only functions in a js/wasm build; this
// stub exists so the package compiles for host tooling (go vet, tests, the SSR
// server) on non-wasm platforms. Rendering on the server goes through the ssr
// package, not bridge.
func Run(app core.Node) {
	panic("bridge.Run is only available in a GOOS=js GOARCH=wasm build")
}
