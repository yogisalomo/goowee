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

Release checklist: bump `internal/version.Module` to the new tag before tagging.
It is compiled into the binary and shown in runtime-mismatch errors, so a
release built with the old value reports the wrong version.

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

**This M1 runs in two speed states.** The release records fall on one of two
levels about 1.6× apart, uniformly across every benchmark, including code a
release didn't touch: `BenchmarkSignalSetFanout` is ~1.13 µs in v0.2.3 and
v0.2.6, and ~1.83 µs in v0.2.1, v0.2.2, v0.2.4, v0.2.5 and v0.2.7 (an
interleaved A/B of v0.2.6 and v0.2.7 measured every benchmark the same, p > 0.1,
with v0.2.6's code at 1.825 µs that day). The cause isn't pinned
down (Low Power Mode was off and no thermal warning was recorded for the fast
v0.2.6 run). So when every timing moves together between two records, it is
the machine: compare records from the same state, or run an interleaved A/B of
the two trees under the same conditions (as the v0.2.5 release did).
Allocations and sizes compare across states. Load matters too: `v0.2.5.txt`
was recorded with about five of the eight cores busy, so its `SSRParallel`,
which uses every core, reads slower still (an interleaved A/B of v0.2.4 and
v0.2.5 measured 234.9 µs vs 233.1 µs, p = 1.0).

**v0.3.0 against v0.2.7.** Allocations are unchanged. `ForAppendOne5k`
(-9.9%) and `LayoutTickFrame` (-6.5%) are faster. `SignalSetFanout` is +6.3%
in the record, and an interleaved A/B measured it about 8% slower (1.82 µs to
1.98 µs). Bisecting puts the step at #125, whose change-log hook adds a nil
check to `Signal.Set`. That pushed `Set` over the inliner's budget and moved
code in the notify loop, which runs 1,000 subscribers per `Set`: about
0.16 ns per subscriber, a layout effect rather than added work. The example's
wasm grew 9.1% (5,223,488 raw, 988,195 brotli), mostly from `goowee/canvas`
and the graph page that the tutorial site now includes.

## Size budget

```sh
make size-report   # sizes, sections and code bytes per package
make size-check    # the same, enforcing size-budget.json (CI runs this)
```

`make size-check-tinygo` does the same for TinyGo builds of the minimal app
and the example; CI runs it in the TinyGo job.

Both build three programs: `test/e2e/hello`, the smallest goowee app; the
same app with `-tags goowee_nodev` (dev tools compiled out, #127); and
`examples/counter`, the tutorial site. `scripts/wasm-size.mjs` reports each
one's raw, gzip `-9` and brotli `-q 11` bytes, its section totals, and the code
bytes per Go package. It reads them from the code section and the `name`
section, which default and `-s -w` builds both keep. It has no dependencies
(Node's zlib compresses).

`size-budget.json` holds each program's brotli bytes, which is what users
download. CI fails when a program is more than 3% over its record, and prints
the per-package table so the log shows which package grew. A change that grows
a program on purpose raises its record in the same diff. One that shrinks it
by more than 3% is told to lower the record, so the budget follows the size
down.

Each program also has a deny list: packages it must not link (`net/http`, and
`text/template` for the minimal app; `goowee/devtools` for the `goowee_nodev`
build). When #126 removes `encoding/json` and
`fmt` from the minimal app, they go on its list too.

The records were taken with Go 1.25. A Go upgrade can move the sizes; record
again in that PR.

[benchstat]: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
