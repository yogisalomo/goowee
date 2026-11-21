package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewDevBuild is #145's "goowee new demo && goowee dev" in CI: a new
// project's tests pass, dev builds and serves it, rebuilds and reloads on an
// edit, reports a broken build to the page, and build writes hashed,
// precompressed files. It builds WASM several times, so -short skips it.
func TestNewDevBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds WASM; skipped with -short")
	}
	for _, ssr := range []bool{false, true} {
		name := "spa"
		if ssr {
			name = "ssr"
		}
		t.Run(name, func(t *testing.T) { checkFlow(t, ssr) })
	}
}

func checkFlow(t *testing.T, ssr bool) {
	root, _ := filepath.Abs("../..")
	dir := filepath.Join(t.TempDir(), "demo")
	args := []string{dir, "--goowee", root}
	if ssr {
		args = append(args, "--ssr")
	}
	if err := newProject(args); err != nil {
		t.Fatal(err)
	}
	t.Log("created")
	run(t, dir, "go", "vet", "./...")
	run(t, dir, "go", "test", "./...")

	t.Log("vetted and tested")
	p, err := loadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.server != ssr {
		t.Fatalf("server/ detected: %v", p.server)
	}
	d := newDevServer(p, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go d.watch(ctx, 100*time.Millisecond)
	srv := httptest.NewServer(d)
	defer func() {
		cancel() // ends the event stream, which srv.Close waits for
		srv.Close()
		d.stopServer()
	}()

	waitFor(t, "the first build", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.builds > 0
	})
	if d.failed != "" {
		t.Fatalf("first build failed:\n%s", d.failed)
	}

	t.Log("first build done")
	page := get(t, srv.URL+"/about", "text/html")
	if !strings.Contains(page, devScript) || !strings.Contains(page, `id="root"`) {
		t.Fatalf("/about isn't the shell with the dev script:\n%s", page)
	}
	if ssr && !strings.Contains(page, "Built with goowee.") {
		t.Fatalf("the server didn't render /about:\n%s", page)
	}
	if wasm := get(t, srv.URL+"/main.wasm", "application/wasm"); !strings.HasPrefix(wasm, "\x00asm") {
		t.Fatal("/main.wasm isn't WASM")
	}
	if js := get(t, srv.URL+"/goowee.js", "text/javascript"); !strings.Contains(js, "goowee.boot") {
		t.Fatal("/goowee.js isn't the runtime")
	}

	t.Log("served")
	events := listen(t, ctx, srv.URL+"/_goowee/events")
	app := filepath.Join(dir, "app", "app.go")
	src, _ := os.ReadFile(app)
	write(t, app, strings.Replace(string(src), "Built with goowee.", "Edited.", 1))
	expect(t, events, "reload")
	if ssr {
		if page := get(t, srv.URL+"/about", "text/html"); !strings.Contains(page, "Edited.") {
			t.Fatal("the restarted server doesn't render the edit")
		}
	}
	t.Log("reloaded after the edit")
	write(t, app, string(src)+"\nfunc broken( {\n")
	if data := expect(t, events, "error"); !strings.Contains(data, "app.go") {
		t.Fatalf("the error event doesn't name the file: %q", data)
	}
	write(t, app, string(src))
	expect(t, events, "reload")

	t.Log("error and fix seen")
	out := filepath.Join(dir, "dist")
	res, err := build(p, out, false)
	if err != nil {
		t.Fatal(err)
	}
	static := out
	if ssr {
		static = filepath.Join(out, "static")
		if _, err := os.Stat(res.server); err != nil {
			t.Fatalf("no server binary: %v", err)
		}
	}
	index, err := os.ReadFile(filepath.Join(static, "index.html"))
	if err != nil {
		if !ssr {
			t.Fatal(err)
		}
	}
	for _, asset := range []string{"main.wasm", "goowee.js", "wasm_exec.js"} {
		hashed := res.names[asset]
		if hashed == asset || !fileExists(filepath.Join(static, hashed)) || !fileExists(filepath.Join(static, hashed+".gz")) {
			t.Fatalf("%s: hashed file %q or its .gz missing", asset, hashed)
		}
		if !ssr && !strings.Contains(string(index), "/"+hashed) {
			t.Fatalf("index.html doesn't refer to %s", hashed)
		}
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := execCommand(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func get(t *testing.T, url, ctype string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), ctype) {
		t.Fatalf("GET %s: %d %s, want 200 %s\n%s", url, resp.StatusCode, resp.Header.Get("Content-Type"), ctype, body)
	}
	return string(body)
}

type event struct{ name, data string }

// listen streams the dev server's events.
func listen(t *testing.T, ctx context.Context, url string) <-chan event {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan event, 16)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var ev event
		var data []string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = append(data, strings.TrimPrefix(line, "data: "))
			case line == "" && ev.name != "":
				ev.data = strings.Join(data, "\n")
				ch <- ev
				ev, data = event{}, nil
			}
		}
	}()
	return ch
}

func expect(t *testing.T, events <-chan event, name string) string {
	t.Helper()
	for {
		select {
		case ev := <-events:
			if ev.name == name {
				return ev.data
			}
		case <-time.After(3 * time.Minute):
			t.Fatalf("no %q event", name)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Some filesystems keep modification times to the second.
	future := time.Now().Add(time.Second)
	os.Chtimes(path, future, future)
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
