.PHONY: wasm ssr-server test serve serve-ssr clean cpwasm cpjs

WASM_OUT = examples/counter/main.wasm

wasm:
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./examples/counter

ssr-server:
	go build -o bin/ssr-server ./cmd/ssr-server

cpwasm:
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/counter/

cpjs:
	cp runtime/goowee.js examples/counter/

test:
	go test ./...

serve: wasm cpjs
	@echo "Open http://localhost:8083"
	cd examples/counter && python3 -m http.server 8083

serve-ssr: wasm ssr-server cpjs
	@echo "Open http://localhost:${PORT:-8081} (SSR-rendered)"
	PORT=${PORT:-8081} ./bin/ssr-server

clean:
	rm -f $(WASM_OUT) bin/ssr-server
