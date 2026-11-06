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

`make ssr-scaling` checks that 4 parallel server renders beat sequential ones by
≥1.5×, the regression ADR-024 removed a lock for. It's wall-clock, so it's
opt-in (`GOOWEE_PERF_TESTS=1`): run it on a quiet machine with ≥4 free CPUs.
The default suite guards the same property deterministically
(`TestSSRRendersOverlap`).

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

**Record on a quiet machine, and check the spread.** Machine state (load,
power and thermal state) shifts every timing at once, including benchmarks of
code the release didn't touch, and often widens the `±` column; when timings
move uniformly or the spread is wide, re-record. `v0.2.3.txt` is such an
outlier: its timings run about 35% fast across the board (v0.2.2 and v0.2.4,
recorded on the same M1, agree with each other within noise), so compare
timings against another release. Its allocations and sizes are sound.

[benchstat]: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
