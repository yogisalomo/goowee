#!/usr/bin/env bash
# Builds the example app with TinyGo and runs the E2E smoke test against it
# (#128): SSR + hydration, events, routing and history on a TinyGo binary,
# loaded with TinyGo's own wasm_exec.js. Requires TinyGo (TINYGO, default
# `tinygo` on PATH), Go, Node (>=21) and Google Chrome.
#
# Flags: -opt z for size, -no-debug to drop DWARF, and -stack-size=1MB:
# TinyGo's default 64 KB goroutine stack overflows at about 20 nested
# component/element/scope levels, and the overflow is only detected after
# memory is corrupted (docs/guides/serving.md).
set -euo pipefail
cd "$(dirname "$0")/../.."

TINYGO="${TINYGO:-tinygo}"
PORT="${PORT:-8138}"
TINYGOROOT="$("$TINYGO" env TINYGOROOT)"

echo "==> building the example with $("$TINYGO" version)"
"$TINYGO" build -target wasm -opt z -no-debug -stack-size=1MB -o examples/counter/main.wasm ./examples/counter
cp "$TINYGOROOT/targets/wasm_exec.js" examples/counter/wasm_exec.js
cp runtime/goowee.js examples/counter/goowee.js
rm -f examples/counter/main.wasm.br
go build -o bin/ssr-server ./cmd/ssr-server

PORT="$PORT" ./bin/ssr-server >/tmp/goowee-tinygo-ssr.log 2>&1 &
SRV=$!
cleanup() {
  kill "$SRV" 2>/dev/null || true
  # Leave the standard build behind, as run.sh expects to find it.
  rm -f examples/counter/main.wasm examples/counter/wasm_exec.js
}
trap cleanup EXIT
for i in $(seq 1 40); do
  if curl -sf "http://localhost:$PORT/counter" >/dev/null 2>&1; then break; fi
  sleep 0.25
done

echo "==> running smoke test on the TinyGo build"
URL="http://localhost:$PORT" TOOLCHAIN=tinygo node test/e2e/smoke.mjs

echo "==> running devtools e2e on the TinyGo build"
URL="http://localhost:$PORT" TOOLCHAIN=tinygo node test/e2e/devtools.mjs
