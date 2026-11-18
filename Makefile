.PHONY: size-build size-report size-check canvasbench wasm brotli ssr-server test bench perf-record perf-compare ssr-scaling size e2e pages-e2e boot serve serve-ssr clean cpwasm cpjs pages docker

WASM_OUT = examples/counter/main.wasm
WASM_BUDGET = 6291456

# Static site for GitHub Pages (client-only, no SSR). PAGES_BASE is the sub-path
# the site is served under — "/goowee/" for a project page, "/" at a domain root.
PAGES_OUT = _site
PAGES_BASE ?= /goowee/

wasm:
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./examples/counter

# Pre-compress the wasm for cmd/ssr-server, which serves main.wasm.br to
# browsers that accept brotli: ~27% smaller than gzip -9 (see
# docs/guides/serving.md). Needs the brotli CLI. `make brotli serve-ssr`.
brotli: wasm
	brotli -q 11 -f -k $(WASM_OUT)

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

# Performance across releases (docs/perf/README.md): record the benchmarks and
# wasm size for a release, or compare the tree with the latest (or BASE=vX.Y.Z).
perf-record:
	@test -n "$(VERSION)" || { echo "usage: make perf-record VERSION=vX.Y.Z"; exit 2; }
	scripts/perf.sh record $(VERSION)

perf-compare:
	scripts/perf.sh compare $(BASE)

# Parallel SSR speedup (4 workers vs sequential, want >=1.5x): a wall-clock
# check, so it's opt-in. Run it on a quiet machine with >=4 free CPUs.
ssr-scaling:
	GOOWEE_PERF_TESTS=1 go test -run '^TestConcurrentSSRScales$$' -count=1 -v ./ssr/

# What the wasm is made of (#129): raw/gzip/brotli sizes, sections and code
# bytes per Go package, for the smallest app (test/e2e/hello) and the example
# site. size-check also enforces docs/perf/size-budget.json, as CI does.
SIZE_DIR = bin/size
size-build:
	@mkdir -p $(SIZE_DIR)
	GOOS=js GOARCH=wasm go build -o $(SIZE_DIR)/hello.wasm ./test/e2e/hello
	GOOS=js GOARCH=wasm go build -tags goowee_nodev -o $(SIZE_DIR)/hello-nodev.wasm ./test/e2e/hello
	GOOS=js GOARCH=wasm go build -o $(SIZE_DIR)/counter.wasm ./examples/counter

size-report: size-build
	node scripts/wasm-size.mjs $(SIZE_DIR)/hello.wasm $(SIZE_DIR)/hello-nodev.wasm $(SIZE_DIR)/counter.wasm

size-check: size-build
	node scripts/wasm-size.mjs --check docs/perf/size-budget.json \
	  test/e2e/hello=$(SIZE_DIR)/hello.wasm "test/e2e/hello -tags goowee_nodev=$(SIZE_DIR)/hello-nodev.wasm" \
	  examples/counter=$(SIZE_DIR)/counter.wasm

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
	go run ./cmd/site-export -out $(PAGES_OUT) -base $(PAGES_BASE) -shell scripts/pages-index.html
	@echo "built $(PAGES_OUT)/ (base $(PAGES_BASE)) — prerendered static site (ssr.Export)"

clean:
	rm -f $(WASM_OUT) $(WASM_OUT).br bin/ssr-server
	rm -rf $(PAGES_OUT)

e2e:
	./test/e2e/run.sh

# goowee/canvas frame time on examples/graph (10,000 nodes + 10,000 edges per
# frame) in headless Chrome: a wall-clock report, so it's not part of e2e.
canvasbench:
	node test/e2e/canvasbench.mjs

# The GitHub Pages build, served under /goowee/ like Pages, in headless Chrome.
pages-e2e: pages
	node test/e2e/pages.mjs $(PAGES_OUT)

# Boot-latency / TTI measurement (roadmap 3.2). Prints a median phase split;
# set GOOWEE_TTI_BUDGET_MS to gate. See docs/plans/boot-latency-measurement.md.
boot:
	./test/e2e/boot.sh

# Build the counter example into examples/counter/dist and run it in the
# reference nginx image (examples/counter/Dockerfile + nginx.conf, the same
# setup documented in docs/docker/). Requires docker. Serves on :8080.
DOCKER_TAG ?= goowee-counter
EXAMPLE_DIST = examples/counter/dist
docker:
	rm -rf $(EXAMPLE_DIST) && mkdir -p $(EXAMPLE_DIST)
	GOOS=js GOARCH=wasm go build -o $(EXAMPLE_DIST)/main.wasm ./examples/counter
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(EXAMPLE_DIST)/
	cp runtime/goowee.js $(EXAMPLE_DIST)/
	cp examples/counter/index.html examples/counter/site.css examples/counter/counter.js $(EXAMPLE_DIST)/
	docker build -t $(DOCKER_TAG) examples/counter
	docker run --rm -p 8080:8080 $(DOCKER_TAG)
