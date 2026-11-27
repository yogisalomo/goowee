package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package inside a module builds (make pages builds ./examples/counter),
// --web replaces web/ (prerendered pages), and --no-compress writes no
// .gz/.br siblings.
func TestBuildPackageInModule(t *testing.T) {
	web := t.TempDir()
	page := `<link rel="preload" href="main.wasm"><script src="wasm_exec.js"></script>` +
		`<script src="goowee.js"></script><script>goowee.boot({wasm: "main.wasm"});</script>`
	if err := os.MkdirAll(filepath.Join(web, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "docs/index.html"} {
		if err := os.WriteFile(filepath.Join(web, f), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	if err := buildCmd([]string{"testdata/inmodule", "--web", web, "--out", out, "--no-report", "--no-compress"}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "docs/index.html"} {
		got, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		for _, asset := range []string{"main.wasm", "goowee.js", "wasm_exec.js"} {
			if strings.Contains(string(got), `"`+asset+`"`) {
				t.Errorf("%s still refers to %s: %s", f, asset, got)
			}
		}
	}
	matches, _ := filepath.Glob(filepath.Join(out, "main.*.wasm"))
	if len(matches) != 1 {
		t.Fatalf("hashed wasm: %v", matches)
	}
	if gz, _ := filepath.Glob(filepath.Join(out, "*.gz")); len(gz) != 0 {
		t.Errorf("--no-compress wrote %v", gz)
	}
}

func TestBuildNeedsModule(t *testing.T) {
	if _, err := loadProject(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no go.mod") {
		t.Fatalf("err = %v, want no go.mod", err)
	}
}
