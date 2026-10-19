#!/usr/bin/env bash
# Build the WASM app + SSR server, serve it, and run the headless-Chrome smoke
# test against it. Requires Go, Node (>=21), and Google Chrome.
set -euo pipefail
cd "$(dirname "$0")/../.."

PORT="${PORT:-8137}"

echo "==> building WASM + assets + SSR server"
GOOS=js GOARCH=wasm go build -o examples/counter/main.wasm ./examples/counter
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/counter/ 2>/dev/null \
  || cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" examples/counter/
cp runtime/goowee.js examples/counter/goowee.js
go build -o bin/ssr-server ./cmd/ssr-server
# With the brotli CLI, also build the pre-compressed copy the server prefers,
# so the browser boots from a br-encoded wasm (-q 9 is fast; `make brotli`
# uses -q 11 for real serving).
BR=0
rm -f examples/counter/main.wasm.br
if command -v brotli >/dev/null 2>&1; then
  brotli -q 9 -f -k examples/counter/main.wasm && BR=1
fi

echo "==> starting SSR server on :$PORT"
PORT="$PORT" ./bin/ssr-server >/tmp/goowee-e2e-ssr.log 2>&1 &
SRV=$!
trap 'kill "$SRV" 2>/dev/null || true' EXIT

for i in $(seq 1 40); do
  if curl -sf "http://localhost:$PORT/counter" >/dev/null 2>&1; then break; fi
  sleep 0.25
done

if [ "$BR" = 1 ]; then
  enc=$(curl -s -o /dev/null -D - -H 'Accept-Encoding: gzip, br' "http://localhost:$PORT/main.wasm" \
    | tr -d '\r' | awk -F': ' 'tolower($1)=="content-encoding"{print $2}')
  if [ "$enc" != "br" ]; then
    echo "main.wasm.br was built but main.wasm was served as '${enc:-identity}'" >&2
    exit 1
  fi
  echo "==> main.wasm served brotli-encoded"
fi

echo "==> running smoke test"
URL="http://localhost:$PORT" node test/e2e/smoke.mjs

echo "==> running devtools e2e"
URL="http://localhost:$PORT" node test/e2e/devtools.mjs
