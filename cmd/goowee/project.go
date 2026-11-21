package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yogisalomo/goowee/web"
)

// config is goowee.json, a project's build settings that `goowee new`
// records so dev and build don't need flags every time.
type config struct {
	TinyGo bool `json:"tinygo,omitempty"`
}

const configFile = "goowee.json"

func writeConfig(dir string, c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, configFile), append(data, '\n'), 0o644)
}

// project is the layout dev and build work with: the module root, the
// browser entry point (the root package), web/ for static files, and
// server/ when the project renders on the server.
type project struct {
	dir    string
	web    string // static files, "" if there is no web/
	server bool   // server/ exists: an SSR project
	cfg    config
}

func loadProject(dir string) (*project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return nil, errors.New("no go.mod in " + abs + ": run goowee in a project's root (goowee new creates one)")
	}
	p := &project{dir: abs}
	if fi, err := os.Stat(filepath.Join(abs, "web")); err == nil && fi.IsDir() {
		p.web = filepath.Join(abs, "web")
	}
	if fi, err := os.Stat(filepath.Join(abs, "server")); err == nil && fi.IsDir() {
		p.server = true
	}
	if data, err := os.ReadFile(filepath.Join(abs, configFile)); err == nil {
		if err := json.Unmarshal(data, &p.cfg); err != nil {
			return nil, errors.New(configFile + ": " + err.Error())
		}
	}
	return p, nil
}

// wasmMode is how buildWasm builds.
type wasmMode int

const (
	wasmDev   wasmMode = iota // as go build does by default
	wasmProd                  // symbols stripped, dev tools left out (goowee_nodev)
	wasmNames                 // wasmProd with the name section, for the size report
)

// buildWasm builds the browser entry point to out.
func (p *project) buildWasm(out string, mode wasmMode) ([]byte, error) {
	var cmd *exec.Cmd
	if p.cfg.TinyGo {
		args := []string{"build", "-target", "wasm", "-opt", "z", "-stack-size=1MB", "-o", out}
		if mode != wasmNames {
			args = append(args, "-no-debug")
		}
		if mode != wasmDev {
			args = append(args, "-tags", "goowee_nodev")
		}
		cmd = exec.Command("tinygo", append(args, ".")...)
	} else {
		args := []string{"build", "-o", out}
		switch mode {
		case wasmProd:
			args = append(args, "-trimpath", "-ldflags=-s -w", "-tags", "goowee_nodev")
		case wasmNames:
			args = append(args, "-trimpath", "-tags", "goowee_nodev")
		}
		cmd = exec.Command("go", append(args, ".")...)
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	}
	cmd.Dir = p.dir
	return cmd.CombinedOutput()
}

// buildServer builds server/ for the host.
func (p *project) buildServer(out string, prod bool) ([]byte, error) {
	args := []string{"build", "-o", out}
	if prod {
		args = append(args, "-trimpath")
	}
	cmd := exec.Command("go", append(args, "./server")...)
	cmd.Dir = p.dir
	return cmd.CombinedOutput()
}

// loader is the wasm_exec.js that matches the toolchain buildWasm uses.
func (p *project) loader() ([]byte, error) {
	if !p.cfg.TinyGo {
		return web.WasmExecJS()
	}
	out, err := exec.Command("tinygo", "env", "TINYGOROOT").Output()
	if err != nil {
		return nil, errors.New("tinygo env TINYGOROOT: " + err.Error())
	}
	return os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "targets", "wasm_exec.js"))
}

// copyDir copies the files under src into dst, keeping their paths.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// execCommand is exec.Command, a variable for tests.
var execCommand = exec.Command
