.PHONY: wasm ssr-server test bench size serve serve-ssr clean cpwasm cpjs

WASM_OUT = examples/counter/main.wasm
WASM_BUDGET = 6291456

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

bench:
	go test -run '^$$' -bench . ./core/ ./dom/

size: wasm
	@bytes=$$(wc -c < $(WASM_OUT) | tr -d ' '); \
	echo "main.wasm: $$bytes bytes (budget $(WASM_BUDGET))"; \
	if [ "$$bytes" -gt "$(WASM_BUDGET)" ]; then echo "over budget"; exit 1; fi

serve: wasm cpwasm cpjs
	@echo "Open http://localhost:8083"
	cd examples/counter && python3 -m http.server 8083

serve-ssr: wasm ssr-server cpwasm cpjs
	@echo "Open http://localhost:${PORT:-8081} (SSR-rendered)"
	PORT=${PORT:-8081} ./bin/ssr-server

clean:
	rm -f $(WASM_OUT) bin/ssr-server
