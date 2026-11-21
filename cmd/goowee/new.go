package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/yogisalomo/goowee/web"
)

//go:embed all:templates
var templates embed.FS

// newProject is `goowee new <dir>`: a working project from the embedded
// templates (templates/app, plus templates/ssr for --ssr).
func newProject(args []string) error {
	fset := flag.NewFlagSet("new", flag.ContinueOnError)
	ssr := fset.Bool("ssr", false, "render pages on the server too (adds server/)")
	tinygo := fset.Bool("tinygo", false, "build with TinyGo by default (records it in goowee.json)")
	module := fset.String("module", "", "the Go module path (default: the directory's name)")
	replace := fset.String("goowee", "", "use the goowee module at this local path (a replace directive), for working on goowee itself")
	if err := fset.Parse(reorder(args)); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errors.New("usage: goowee new <dir> [--ssr] [--tinygo] [--module path]")
	}
	dir := fset.Arg(0)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s exists and isn't empty", dir)
	}
	name := filepath.Base(filepath.Clean(dir))
	if *module == "" {
		*module = name
	}
	if *replace != "" {
		abs, err := filepath.Abs(*replace)
		if err != nil {
			return err
		}
		*replace = filepath.ToSlash(abs)
	}
	data := map[string]any{
		"Name": name, "Module": *module, "Version": web.Version,
		"Replace": *replace, "SSR": *ssr, "TinyGo": *tinygo,
	}
	sets := []string{"templates/app"}
	if *ssr {
		sets = append(sets, "templates/ssr")
	}
	for _, set := range sets {
		if err := writeTemplates(set, dir, data); err != nil {
			return err
		}
	}
	if err := writeConfig(dir, config{TinyGo: *tinygo}); err != nil {
		return err
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "goowee: `go mod tidy` failed in %s; run it once you're online:\n%s\n", dir, out)
	}
	fmt.Fprintf(os.Stderr, "goowee: created %s\n\n  cd %s\n  goowee dev\n\n", dir, dir)
	return nil
}

// writeTemplates renders every file under set into dir: "x.tmpl" becomes x,
// and "gitignore" becomes .gitignore (a dotfile can't be embedded by name).
func writeTemplates(set, dir string, data map[string]any) error {
	return fs.WalkDir(templates, set, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(p, set+"/"), ".tmpl")
		if path.Base(rel) == "gitignore" {
			rel = path.Join(path.Dir(rel), ".gitignore")
		}
		src, err := templates.ReadFile(p)
		if err != nil {
			return err
		}
		t, err := template.New(rel).Parse(string(src))
		if err != nil {
			return fmt.Errorf("template %s: %w", p, err)
		}
		out := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		if err := t.Execute(f, data); err != nil {
			f.Close()
			return fmt.Errorf("template %s: %w", p, err)
		}
		return f.Close()
	})
}

// reorder moves flags before positional arguments, so `goowee new demo
// --ssr` works like `goowee new --ssr demo` (the flag package stops at the
// first non-flag).
func reorder(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			// A flag with a separate value: --module example.com/x.
			if !strings.Contains(a, "=") && i+1 < len(args) && takesValue(a) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

func takesValue(flag string) bool {
	switch strings.TrimLeft(flag, "-") {
	case "module", "goowee", "addr", "out", "proxy":
		return true
	}
	return false
}
