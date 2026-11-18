// Command render writes test/e2e/early's page, server-rendered, to stdout.
// The page doesn't boot by itself: the test calls window.bootNow() once it has
// typed and clicked, to stand in for a slow WASM download.
package main

import (
	"fmt"

	"github.com/yogisalomo/goowee/ssr"
	"github.com/yogisalomo/goowee/test/e2e/early/app"
)

func main() {
	body, head := ssr.New().Render(app.App())
	fmt.Printf(`<!DOCTYPE html>
<html><head><meta charset="utf-8">%s
<script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head>
<body><div id="root">%s</div>
<script>
window.bootNow = function () { return goowee.boot(); };
window.bootBroken = function () { return goowee.boot({wasm: "missing.wasm"}).catch(function () {}); };
</script>
</body></html>
`, head, body)
}
