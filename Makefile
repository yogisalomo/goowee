.PHONY: wasm ssr-server test bench size e2e boot serve serve-ssr clean cpwasm cpjs pages

WASM_OUT = examples/counter/main.wasm
WASM_BUDGET = 6291456

# Static site for GitHub Pages (client-only, no SSR). PAGES_BASE is the sub-path
# the site is served under — "/goowee/" for a project page, "/" at a domain root.
PAGES_OUT = _site
PAGES_BASE ?= /goowee/

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
	go test -run '^$$' -bench . ./core/ ./internal/dom/

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

pages:
	rm -rf $(PAGES_OUT) && mkdir -p $(PAGES_OUT)
	GOOS=js GOARCH=wasm go build -o $(PAGES_OUT)/main.wasm ./examples/counter
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(PAGES_OUT)/
	cp runtime/goowee.js $(PAGES_OUT)/
	cp examples/counter/site.css $(PAGES_OUT)/
	sed 's|__BASE__|$(PAGES_BASE)|g' scripts/pages-index.html > $(PAGES_OUT)/index.html
	cp $(PAGES_OUT)/index.html $(PAGES_OUT)/404.html
	@echo "built $(PAGES_OUT)/ (base $(PAGES_BASE)) — client-only static site"

clean:
	rm -f $(WASM_OUT) bin/ssr-server
	rm -rf $(PAGES_OUT)

e2e:
	./test/e2e/run.sh

# Boot-latency / TTI measurement (roadmap 3.2). Prints a median phase split;
# set GOOWEE_TTI_BUDGET_MS to gate. See docs/plans/boot-latency-measurement.md.
boot:
	./test/e2e/boot.sh
