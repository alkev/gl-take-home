.PHONY: build test bench run load-test semcheck docker clean lint fmt help

# Default target: show available targets
help:
	@echo "Reviewer-facing targets:"
	@echo "  make build       - compile bin/vecstore and bin/loader"
	@echo "  make test        - unit + HTTP contract tests with -race"
	@echo "  make bench       - Go micro-benchmarks"
	@echo "  make run         - build and run the service on :8888"
	@echo "  make docker      - build the Docker image"
	@echo "  make load-test   - run vegeta load scenarios (requires running service)"
	@echo "  make semcheck    - run the 100-query semantic plausibility spot-check"
	@echo "  make clean       - remove build artifacts"
	@echo ""
	@echo "Dev-only targets (gracefully skip if the tool is missing):"
	@echo "  make lint        - golangci-lint run ./..."
	@echo "  make fmt         - gofmt + goimports"

# ─── Reviewer-facing (Go + optional Docker/vegeta only) ─────────

build:
	go build -o bin/vecstore ./cmd/vecstore
	go build -o bin/loader   ./cmd/loader
	go build -o bin/semcheck ./cmd/semcheck

test:
	go test -race ./...

bench:
	go test -bench=. -benchmem -run=^$$ ./...

run: build
	./bin/vecstore

load-test:
	@test -x ./bench/run.sh || { echo "bench/run.sh missing or not executable"; exit 1; }
	./bench/run.sh

semcheck: build
	./bin/semcheck -url $${VECSTORE_URL:-http://localhost:8888} \
	               -fixture testdata/semantic_queries.txt

docker:
	docker build -t vecstore:dev .

clean:
	rm -rf bin/

# ─── Dev-only (requires golangci-lint, goimports) ───────────────

lint:
	@command -v golangci-lint >/dev/null || { \
	    echo "golangci-lint not installed — dev-only tool, skipping"; exit 0; }
	golangci-lint run ./...

fmt:
	@command -v golangci-lint >/dev/null || { \
	    echo "golangci-lint not installed — dev-only tool, skipping"; exit 0; }
	golangci-lint fmt ./...
