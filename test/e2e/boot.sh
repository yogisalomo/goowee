#!/usr/bin/env bash
# Build the WASM app + SSR server, serve it, and run the boot-latency / TTI
# measurement against it (roadmap 3.2). Requires Go, Node (>=21), and a
# Chromium/Chrome (CHROME env overrides the path).
# Set THROTTLE=4g|fast3g|slow3g to emulate a real network and see the
# compression/download win (loopback is too fast to show it).
# See docs/plans/boot-latency-measurement.md.
set -euo pipefail
cd "$(dirname "$0")/../.."

PORT="${PORT:-8138}"

echo "==> building WASM + assets + SSR server"
GOOS=js GOARCH=wasm go build -o examples/counter/main.wasm ./examples/counter
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/counter/ 2>/dev/null \
  || cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" examples/counter/
cp runtime/goowee.js examples/counter/goowee.js
go build -o bin/ssr-server ./cmd/ssr-server

echo "==> starting SSR server on :$PORT"
PORT="$PORT" ./bin/ssr-server >/tmp/goowee-boot-ssr.log 2>&1 &
SRV=$!
trap 'kill "$SRV" 2>/dev/null || true' EXIT

for i in $(seq 1 40); do
  if curl -sf "http://localhost:$PORT/counter" >/dev/null 2>&1; then break; fi
  sleep 0.25
done

echo "==> measuring boot"
URL="http://localhost:$PORT" node test/e2e/boot.mjs
