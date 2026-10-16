#!/usr/bin/env bash
# Performance numbers across releases (roadmap 3.5). See docs/perf/README.md.
#
#   scripts/perf.sh record v0.3.0   # benchmarks + wasm size -> docs/perf/v0.3.0.txt
#   scripts/perf.sh compare         # this tree vs the latest recorded release
#   scripts/perf.sh compare v0.2.1  # this tree vs a given release
#
# Results are benchstat input: the Go benchmarks (-count $COUNT) plus one
# BenchmarkExampleWASM line with the example app's raw/gzip/brotli bytes. Timings
# only compare on the same machine; the files record the cpu they ran on.
set -euo pipefail
cd "$(dirname "$0")/.."

COUNT="${COUNT:-6}"
PKGS="./core/ ./internal/dom/ ./ssr/"
# The last golang.org/x/perf that builds with Go 1.25 (later ones need 1.26).
BENCHSTAT="golang.org/x/perf/cmd/benchstat@v0.0.0-20260813145340-fd4a688df892"

run() {
  # shellcheck disable=SC2086
  go test -run '^$' -bench . -benchmem -count "$COUNT" $PKGS | grep -E '^(goos|goarch|cpu|pkg|Benchmark)'
  local dir wasm raw gz br units
  dir=$(mktemp -d)
  wasm="$dir/main.wasm"
  GOOS=js GOARCH=wasm go build -o "$wasm" ./examples/counter
  raw=$(wc -c < "$wasm" | tr -d ' ')
  gz=$(gzip -9 -c "$wasm" | wc -c | tr -d ' ')
  units="$raw raw-B $gz gzip-B"
  if command -v brotli >/dev/null 2>&1; then
    br=$(brotli -q 11 -c "$wasm" | wc -c | tr -d ' ')
    units="$units $br brotli-B"
  fi
  rm -rf "$dir"
  echo "pkg: github.com/yogisalomo/goowee/examples/counter"
  echo "BenchmarkExampleWASM 1 $units"
}

latest() {
  ls docs/perf/v*.txt 2>/dev/null | sed 's|.*/||; s|\.txt$||' | sort -V | tail -1
}

case "${1:-}" in
  record)
    version="${2:?usage: scripts/perf.sh record <version, e.g. v0.3.0>}"
    out="docs/perf/$version.txt"
    {
      echo "# goowee $version — $(go version | cut -d' ' -f3), $(date -u +%Y-%m-%d)"
      run
    } > "$out"
    echo "wrote $out"
    ;;
  compare)
    base="${2:-$(latest)}"
    [ -n "$base" ] || { echo "no recorded release in docs/perf/" >&2; exit 1; }
    new=$(mktemp)
    trap 'rm -f "$new"' EXIT
    run > "$new"
    go run "$BENCHSTAT" "$base=docs/perf/$base.txt" "this tree=$new"
    ;;
  *)
    echo "usage: scripts/perf.sh record <version> | compare [<version>]" >&2
    exit 2
    ;;
esac
