package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/yogisalomo/goowee/examples/counter/app"
	"github.com/yogisalomo/goowee/router"
	"github.com/yogisalomo/goowee/ssr"
)

func main() {
	staticDir := "examples/counter"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = filepath.Join("..", "..", "examples", "counter")
	}
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Fatalf("static directory not found at examples/counter/")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Route known paths through SSR
		switch r.URL.Path {
		case "/", "/tutorial", "/counter", "/about", "/form", "/todos", "/stopwatch", "/dashboard", "/async":
			rtr := router.New(r.URL.Path)
			renderer := ssr.New()
			body := renderer.Render(app.App(rtr))

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>goowee — reactive Go UIs in WebAssembly</title>
    <link rel="stylesheet" href="site.css">
    <script src="wasm_exec.js"></script>
    <script src="goowee.js"></script>
    <script src="counter.js"></script>
</head>
<body>
    <div id="root">%s</div>
    <script>
        const go = new Go();
        WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject)
            .then(result => go.run(result.instance));
    </script>
</body>
</html>`, body)
			return
		}

		// Static files: serve from examples/counter if they exist
		staticPath := filepath.Join(staticDir, r.URL.Path)
		if fi, err := os.Stat(staticPath); err == nil && !fi.IsDir() {
			http.ServeFile(w, r, staticPath)
			return
		}

		// Unknown paths: serve index.html so the WASM app can handle routing client-side
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := ":" + port
	log.Printf("SSR server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
