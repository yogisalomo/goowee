// Command goowee creates, runs and builds goowee apps:
//
//	goowee new <dir> [--ssr] [--tinygo]   a working project
//	goowee dev [--addr host:port] [--proxy /api=URL]
//	                                      build, serve, rebuild and reload on every change
//	goowee build [--out dist] [--tinygo]  a production build with a size report
//	goowee assets <dir>                   copy goowee.js and wasm_exec.js into dir
//
// Run it as go run github.com/yogisalomo/goowee/cmd/goowee, or go install it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yogisalomo/goowee/web"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "assets":
		err = assets(os.Args[2:])
	case "new":
		err = newProject(os.Args[2:])
	case "dev":
		err = devCmd(os.Args[2:])
	case "build":
		err = buildCmd(os.Args[2:])
	case "version":
		fmt.Println("goowee", web.Version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "goowee:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  goowee new <dir> [--ssr] [--tinygo] [--module path]
  goowee dev [dir] [--addr localhost:8080] [--proxy /api=http://localhost:9000]
  goowee build [dir] [--out dist] [--tinygo]
  goowee assets <dir>
  goowee version`)
}

func assets(args []string) error {
	fs := flag.NewFlagSet("assets", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: goowee assets <dir>")
	}
	dir := fs.Arg(0)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	loader, err := web.WasmExecJS()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "goowee.js"), web.RuntimeJS(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), loader, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "goowee: copied goowee.js (%s) and wasm_exec.js into %s\n", web.Version, dir)
	return nil
}
