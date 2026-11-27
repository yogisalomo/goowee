package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yogisalomo/goowee/web"
)

// buildCmd is `goowee build`: a production output directory.
func buildCmd(args []string) error {
	fset := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fset.String("out", "dist", "output directory (replaced)")
	tinygo := fset.Bool("tinygo", false, "build with TinyGo (also set by goowee.json)")
	noReport := fset.Bool("no-report", false, "skip the code-by-package table (it links a second copy with names)")
	webDir := fset.String("web", "", "static files to copy in, instead of the project's web/ (e.g. prerendered pages)")
	noCompress := fset.Bool("no-compress", false, "skip the .gz/.br siblings, for hosts that compress on their own (GitHub Pages)")
	if err := fset.Parse(reorder(args)); err != nil {
		return err
	}
	dir := "."
	if fset.NArg() > 0 {
		dir = fset.Arg(0)
	}
	p, err := loadProject(dir)
	if err != nil {
		return err
	}
	if *tinygo {
		p.cfg.TinyGo = true
	}
	if *webDir != "" {
		if p.web, err = filepath.Abs(*webDir); err != nil {
			return err
		}
		if fi, err := os.Stat(p.web); err != nil || !fi.IsDir() {
			return errors.New("--web " + *webDir + ": not a directory")
		}
	}
	outDir := *out
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(p.dir, outDir)
	}
	res, err := build(p, outDir, buildOptions{report: !*noReport, compress: !*noCompress})
	if err != nil {
		return err
	}
	res.report(os.Stdout)
	return nil
}

type buildResult struct {
	static string            // the static files' directory
	server string            // the server binary, "" for a client-only app
	names  map[string]string // runtime asset -> hashed name
	wasm   []byte
	gz, br int
	brotli bool
	// compressed is false for --no-compress: no .gz/.br were written.
	compressed bool
	packages   []pkgSize
}

// buildOptions are the optional parts of a build.
type buildOptions struct {
	report   bool // the code-by-package table
	compress bool // .gz/.br siblings
}

// build writes the production files: in out for a client-only app, in
// out/static with the server at out/server for an SSR one.
func build(p *project, out string, opt buildOptions) (*buildResult, error) {
	if err := os.RemoveAll(out); err != nil {
		return nil, err
	}
	res := &buildResult{static: out, names: map[string]string{}}
	if p.server {
		res.static = filepath.Join(out, "static")
	}
	if err := os.MkdirAll(res.static, 0o755); err != nil {
		return nil, err
	}
	if p.web != "" {
		if err := copyDir(p.web, res.static); err != nil {
			return nil, err
		}
	}
	tmp := filepath.Join(out, "main.wasm.tmp")
	if outp, err := p.buildWasm(tmp, wasmProd); err != nil {
		return nil, fmt.Errorf("building the WASM failed:\n%s", outp)
	}
	wasm, err := os.ReadFile(tmp)
	os.Remove(tmp)
	if err != nil {
		return nil, err
	}
	res.wasm = wasm
	loader, err := p.loader()
	if err != nil {
		return nil, err
	}
	for name, data := range map[string][]byte{"main.wasm": wasm, "goowee.js": web.RuntimeJS(), "wasm_exec.js": loader} {
		hashed := hashedName(name, data)
		res.names[name] = hashed
		if err := os.WriteFile(filepath.Join(res.static, hashed), data, 0o644); err != nil {
			return nil, err
		}
	}
	// Point the pages at the hashed names.
	err = filepath.Walk(res.static, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}
		page, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, rewriteNames(page, res.names), 0o644)
	})
	if err != nil {
		return nil, err
	}
	if p.server {
		res.server = filepath.Join(out, "server")
		if runtime.GOOS == "windows" {
			res.server += ".exe"
		}
		if outp, err := p.buildServer(res.server, true); err != nil {
			return nil, fmt.Errorf("building server/ failed:\n%s", outp)
		}
		fmt.Fprintln(os.Stderr, "goowee: the server renders pages with the asset names in its own document; rewrite it to read them from", filepath.Join(res.static, "index.html"), "or serve static files as built (#166)")
	}
	// Precompressed siblings: .gz always, .br when a brotli binary is around.
	_, berr := exec.LookPath("brotli")
	res.brotli = berr == nil
	res.compressed = opt.compress
	err = filepath.Walk(res.static, func(path string, info os.FileInfo, err error) error {
		if err != nil || !opt.compress || info.IsDir() || !compressible(contentType(path)) {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(data)
		zw.Close()
		if err := os.WriteFile(path+".gz", buf.Bytes(), 0o644); err != nil {
			return err
		}
		if filepath.Base(path) == res.names["main.wasm"] {
			res.gz = buf.Len()
		}
		if res.brotli {
			if outp, err := exec.Command("brotli", "-q", "11", "-f", "-o", path+".br", path).CombinedOutput(); err != nil {
				return errors.New("brotli: " + string(outp))
			}
			if filepath.Base(path) == res.names["main.wasm"] {
				if fi, err := os.Stat(path + ".br"); err == nil {
					res.br = int(fi.Size())
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if opt.report {
		// The production build has no name section: link a copy with one.
		named := filepath.Join(os.TempDir(), "goowee-named-"+hashedName("main.wasm", wasm))
		if _, err := p.buildWasm(named, wasmNames); err == nil {
			if data, err := os.ReadFile(named); err == nil {
				res.packages, _ = wasmPackages(data)
			}
			os.Remove(named)
		}
	}
	return res, nil
}

// hashedName is name with the first 10 hex digits of data's SHA-256 before
// its extension: main.wasm -> main.3f2a9c0d1e.wasm.
func hashedName(name string, data []byte) string {
	sum := sha256.Sum256(data)
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + "." + hex.EncodeToString(sum[:5]) + ext
}

// rewriteNames replaces quoted references to the runtime assets ("main.wasm",
// "/main.wasm", 'goowee.js', …) with their hashed names.
func rewriteNames(page []byte, names map[string]string) []byte {
	s := string(page)
	for name, hashed := range names {
		for _, q := range []string{`"`, `'`} {
			s = strings.ReplaceAll(s, q+name+q, q+hashed+q)
			s = strings.ReplaceAll(s, q+"/"+name+q, q+"/"+hashed+q)
			s = strings.ReplaceAll(s, q+"./"+name+q, q+"./"+hashed+q)
		}
	}
	return []byte(s)
}

func (res *buildResult) report(w io.Writer) {
	fmt.Fprintf(w, "goowee build: %s\n", res.static)
	if res.server != "" {
		fmt.Fprintf(w, "  server: %s (run it with -static %s)\n", res.server, res.static)
	}
	if !res.compressed {
		fmt.Fprintf(w, "  %s: %s raw (no .gz/.br written)\n", res.names["main.wasm"], kb(len(res.wasm)))
	} else {
		br := "brotli: install `brotli` to write .br files"
		if res.brotli {
			br = fmt.Sprintf("%s brotli", kb(res.br))
		}
		fmt.Fprintf(w, "  %s: %s raw, %s gzip, %s\n", res.names["main.wasm"], kb(len(res.wasm)), kb(res.gz), br)
	}
	if len(res.packages) > 0 {
		total := 0
		for _, p := range res.packages {
			total += p.bytes
		}
		fmt.Fprintf(w, "  code by package (of %s):\n", kb(total))
		for i, p := range res.packages {
			if i == 10 {
				break
			}
			fmt.Fprintf(w, "    %9s  %4.1f%%  %s\n", kb(p.bytes), 100*float64(p.bytes)/float64(total), p.pkg)
		}
	}
	if res.compressed {
		fmt.Fprintln(w, "  Serve the hashed files with Cache-Control: immutable, the .gz/.br siblings with Content-Encoding, and .wasm as application/wasm (docs/guides/serving.md).")
	} else {
		fmt.Fprintln(w, "  Serve the hashed files with Cache-Control: immutable and .wasm as application/wasm (docs/guides/serving.md).")
	}
}

func kb(n int) string { return fmt.Sprintf("%.1f KB", float64(n)/1024) }
