// Command render writes test/e2e/islands' page: plain server HTML, the kind
// an html/template site serves, with two goowee islands in it. It doesn't
// boot by itself: the test looks at the server markup first, then calls
// window.bootNow().
package main

import (
	"fmt"
	"log"

	"github.com/yogisalomo/goowee/ssr"
	"github.com/yogisalomo/goowee/test/e2e/islands/app"
)

func main() {
	a, err := ssr.Island("counter", app.Counter(app.CounterProps{Label: "apples", Start: 3}), app.CounterProps{Label: "apples", Start: 3})
	if err != nil {
		log.Fatal(err)
	}
	b, err := ssr.Island("counter", app.Counter(app.CounterProps{Label: "pears", Start: 10}), app.CounterProps{Label: "pears", Start: 10})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Shop</title>
<script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head>
<body>
<header><h1>A server-rendered shop</h1><nav><a href="/about">About</a></nav></header>
<main>
<section id="a"><p>Server text before.</p>%s</section>
<p id="between">Server text between the islands.</p>
<section id="b">%s</section>
</main>
<footer>Footer from the server template</footer>
<script>window.bootNow = function () { return goowee.boot(); };</script>
</body></html>
`, a, b)
}
