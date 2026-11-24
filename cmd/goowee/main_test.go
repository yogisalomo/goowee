package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yogisalomo/goowee/web"
)

func TestAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lib", "wasm"), 0o755); err != nil {
		t.Fatal(err)
	}
	loader := []byte("// wasm loader\n")
	if err := os.WriteFile(filepath.Join(root, "lib", "wasm", "wasm_exec.js"), loader, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOROOT", root)

	out := filepath.Join(t.TempDir(), "web")
	if err := assets([]string{out}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"goowee.js":    string(web.RuntimeJS()),
		"wasm_exec.js": string(loader),
	} {
		got, err := os.ReadFile(filepath.Join(out, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s: got %d bytes, want %d", path, len(got), len(want))
		}
	}
}

func TestAssetsNeedsDir(t *testing.T) {
	if err := assets(nil); err == nil {
		t.Fatal("assets with no directory succeeded")
	}
}
