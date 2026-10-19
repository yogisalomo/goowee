# Performance across releases

Each file here is one release's numbers in [benchstat] input format: the Go
benchmark suite (`./core/`, `./internal/dom/`, `./ssr/`, `-count 6`) plus a
`BenchmarkExampleWASM` line holding the example app's wasm size raw, gzip `-9`
and brotli `-q 11` (in bytes).

```sh
make perf-record VERSION=v0.3.0   # at a release: writes docs/perf/v0.3.0.txt
make perf-compare                 # the working tree vs the latest release
make perf-compare BASE=v0.2.1     # … or vs a given one
```

`perf-compare` prints benchstat's table, with a delta and a p-value per
benchmark, so a regression shows up before it ships. Run it before cutting a
release, then record the release.

**Compare on one machine.** Timings depend on the CPU, so a comparison is only
meaningful between files recorded on the same machine. Each file's header lines
name the Go version, date and `cpu`. Sizes (`BenchmarkExampleWASM`) and
allocation counts compare anywhere. When the recording machine changes, record
the previous release again on the new one before comparing.

The benchmarks run natively. WASM runs the same code several times slower
(about 7× for `BenchmarkLayoutTickFrame` on an M1), so read native numbers as
relative. To time the WASM build directly:

```sh
GOOS=js GOARCH=wasm go test -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
  -run '^$' -bench LayoutTick -benchtime 40x ./internal/dom/
```

`v0.2.1.txt` was recorded after the release, from the v0.2.1 code with the
benchmark suite as of 2026-10-16 (which adds the layout-tick benchmarks), so the
first comparison covers them too.

[benchstat]: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
