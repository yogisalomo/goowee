// Command goowee builds the browser assets an app needs to match its Go build.
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
	fmt.Fprintln(os.Stderr, "usage: goowee assets <dir>")
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
