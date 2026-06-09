# Build variables
BINARY_NAME=centian
BUILD_DIR=build
MAIN_PATH=./main.go
LOG_DIR=$(HOME)/.centian/logs
WEB_DIR=web
UI_DIST_DIR=internal/ui/dist
BENCH_SUITE ?= tests/integrationtests/taskverification/benchmarks/simple_tdd_v1
BENCH_CASE ?= assertion_failure_red
BENCH_AGENT ?= codex
BENCH_REPEAT ?= 1
BENCH_OUTPUT_ROOT ?= tests/integrationtests/taskverification/.tmp/benchmarks
BENCH_TIMEOUT ?= 15m

# Release bump (defaults to patch, can be set via `make release minor`)
BUMP ?= patch
ifneq ($(filter major minor patch,$(MAKECMDGOALS)),)
BUMP := $(filter major minor patch,$(MAKECMDGOALS))
endif

# Version info
VERSION ?= dev
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# For dev builds, append build timestamp and commit hash
ifeq ($(VERSION),dev)
VERSION := dev+$(BUILD_DATE).$(COMMIT_HASH)
endif

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: help build build-go clean test test-integration test-everything test-realworld test-taskverification test-taskverification-blackbox test-processor-e2e benchmark-simple-tdd benchmark-score-latest test-all test-coverage test-coverage-html lint fmt vet tidy run dev web-install web-dev web-build web-stage web-test web-lint web-preview web-clean ensure-web-tooling check-main-branch tag-release release major minor patch

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s", $$1, $$2}'

build: web-stage ## Build the MCP proxy binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

build-go: ## Build the MCP proxy binary without staging the frontend
	@echo "Building $(BINARY_NAME) without rebuilding the frontend..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

clean: web-clean ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)

test: ## Run unit tests
	@echo "Running unit tests..."
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname -- -race ./internal/... .; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		go test -v -race ./internal/... .; \
	fi

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	go test -v ./tests/integrationtests/...

test-everything: ## Run everything MCP integration tests via npx
	@echo "Running everything MCP integration tests..."
	@if command -v gotestsum >/dev/null 2>&1; then \
		CENTIAN_RUN_EVERYTHING_INTEGRATION=1 GOCACHE=/tmp/go-build gotestsum --format testname -- ./tests/integrationtests/everything/...; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		CENTIAN_RUN_EVERYTHING_INTEGRATION=1 GOCACHE=/tmp/go-build go test -v ./tests/integrationtests/everything/...; \
	fi

test-realworld: ## Run opt-in real-world MCP integration tests
	@echo "Running real-world MCP integration tests..."
	@if command -v gotestsum >/dev/null 2>&1; then \
		CENTIAN_RUN_REALWORLD_INTEGRATION=1 GOCACHE=/tmp/go-build gotestsum --format testname -- ./tests/integrationtests/realworld/...; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		CENTIAN_RUN_REALWORLD_INTEGRATION=1 GOCACHE=/tmp/go-build go test -v ./tests/integrationtests/realworld/...; \
	fi

test-taskverification: ## Run opt-in Docker task verification integration test
	@echo "Running Docker task verification integration test..."
	@if command -v gotestsum >/dev/null 2>&1; then \
		CENTIAN_RUN_TASKVERIFICATION_INTEGRATION=1 GOCACHE=/tmp/go-build gotestsum --format testname -- ./demo/taskverification; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		CENTIAN_RUN_TASKVERIFICATION_INTEGRATION=1 GOCACHE=/tmp/go-build go test -v ./demo/taskverification; \
	fi

test-taskverification-blackbox: ## Run opt-in host-native black-box taskverification test
	@echo "Running host-native black-box taskverification test..."
	@if command -v gotestsum >/dev/null 2>&1; then \
		CENTIAN_RUN_TASKVERIFICATION_BLACKBOX=1 GOCACHE=/tmp/go-build gotestsum --format standard-verbose -- -v ./tests/integrationtests/taskverification -run TestTaskVerificationBlackBox; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		CENTIAN_RUN_TASKVERIFICATION_BLACKBOX=1 GOCACHE=/tmp/go-build go test -v ./tests/integrationtests/taskverification -run TestTaskVerificationBlackBox; \
	fi

test-processor-e2e: ## Run the interactive processor e2e/demo harness (holds server open; Ctrl+C to stop & clean up)
	@echo "Running interactive processor e2e/demo harness (agent: $${CENTIAN_E2E_AGENT:-claude})..."
	@echo "The server stays open after assertions; press Ctrl+C to shut down and remove temp artifacts."
	CENTIAN_RUN_PROCESSOR_E2E=1 GOCACHE=/tmp/go-build go test -tags e2e -v -count=1 -timeout 0 \
		-run 'TestProcessorE2E$$' ./tests/integrationtests/processore2e/

benchmark-simple-tdd: build-go ## Run one local simple_tdd benchmark case and print newest session
	@echo "Running benchmark suite $(BENCH_SUITE) with agent $(BENCH_AGENT) and case $(BENCH_CASE)..."
	@mkdir -p $(BENCH_OUTPUT_ROOT)
	@./$(BUILD_DIR)/$(BINARY_NAME) benchmark run \
		--suite "$(BENCH_SUITE)" \
		--agent "$(BENCH_AGENT)" \
		--case "$(BENCH_CASE)" \
		--repeat "$(BENCH_REPEAT)" \
		--output-root "$(BENCH_OUTPUT_ROOT)" \
		--timeout "$(BENCH_TIMEOUT)"
	@$(MAKE) --no-print-directory benchmark-score-latest

benchmark-score-latest: build-go ## Score the newest preserved simple_tdd benchmark session
	@session_dir=$$(find "$(BENCH_OUTPUT_ROOT)/simple_tdd_v1" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1); \
	if [ -z "$$session_dir" ]; then \
		echo "No benchmark session found under $(BENCH_OUTPUT_ROOT)/simple_tdd_v1"; \
		exit 1; \
	fi; \
	echo "Newest benchmark session: $$session_dir"; \
	echo "Inspect in UI: /ui/benchmarks/simple_tdd_v1/sessions/<session-id>"; \
	echo "Inspect in API: /api/benchmarks/suites/simple_tdd_v1/sessions/<session-id>"

test-all: test test-integration web-test ## Run all tests (unit + integration + frontend)

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@mkdir -p build
	@go test -coverprofile=build/coverage.out -covermode=atomic ./internal/... .
	@echo ""
	@echo "=== Coverage by File ==="
	@go tool cover -func=build/coverage.out
	@echo ""
	@echo "Coverage report saved to: build/coverage.out"
	@echo "Generate HTML report with: go tool cover -html=build/coverage.out -o build/coverage.html"

test-coverage-html: test-coverage ## Run tests with coverage and open HTML report
	@echo "Generating HTML coverage report..."
	@go tool cover -html=build/coverage.out -o build/coverage.html
	@echo "Opening coverage report in browser..."
	@open build/coverage.html || xdg-open build/coverage.html || echo "Please open build/coverage.html in your browser"

lint: web-lint ## Run linters (Go + frontend)
	@echo "Running linter..."
	golangci-lint run --timeout=5m ./...

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

tidy: ## Tidy and verify dependencies
	@echo "Tidying dependencies..."
	go mod tidy
	go mod verify

run: build ## Build and run the MCP proxy
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

start: build ## Build and start the MCP proxy server
	@echo "Starting MCP proxy server..."
	./$(BUILD_DIR)/$(BINARY_NAME) start

dev: clean fmt vet test-all build ## Run full development workflow (includes integration tests)

web-install: ## Install frontend dependencies
	@echo "Installing frontend dependencies..."
	cd $(WEB_DIR) && npm install

ensure-web-tooling:
	@if ! command -v node >/dev/null 2>&1; then \
		echo "Error: Node.js is required for frontend-backed builds. Install Node 22 or use 'make build-go'."; \
		exit 1; \
	fi
	@if ! command -v npm >/dev/null 2>&1; then \
		echo "Error: npm is required for frontend-backed builds. Install Node 22 or use 'make build-go'."; \
		exit 1; \
	fi

web-dev: ensure-web-tooling ## Run the frontend dev server
	@echo "Starting frontend dev server..."
	cd $(WEB_DIR) && npm run dev

web-build: ensure-web-tooling ## Build the frontend app
	@echo "Building frontend app..."
	cd $(WEB_DIR) && npm run build

web-stage: web-build ## Stage frontend assets for Go embedding
	@echo "Staging frontend assets for embedding..."
	@mkdir -p $(UI_DIST_DIR)
	@find $(UI_DIST_DIR) -mindepth 1 ! -name '.keep' -exec rm -rf {} +
	@cp -R $(WEB_DIR)/dist/. $(UI_DIST_DIR)/

web-test: ensure-web-tooling ## Run frontend tests
	@echo "Running frontend tests..."
	cd $(WEB_DIR) && npm test

web-lint: ensure-web-tooling ## Run frontend lint/type checks
	@echo "Running frontend lint checks..."
	cd $(WEB_DIR) && npm run lint

web-preview: ensure-web-tooling ## Preview the built frontend app
	@echo "Previewing frontend app..."
	cd $(WEB_DIR) && npm run preview

web-clean: ## Clean frontend build and generated config artifacts
	@echo "Cleaning frontend artifacts..."
	@rm -rf $(WEB_DIR)/dist
	@rm -rf $(WEB_DIR)/coverage
	@rm -f $(WEB_DIR)/*.tsbuildinfo
	@rm -f $(WEB_DIR)/*.js
	@rm -f $(WEB_DIR)/*.d.ts
	@mkdir -p $(UI_DIST_DIR)
	@find $(UI_DIST_DIR) -mindepth 1 ! -name '.keep' -exec rm -rf {} +

install: build ## Install binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@dest_dir="$(shell go env GOPATH)/bin"; \
	dest="$$dest_dir/$(BINARY_NAME)"; \
	tmp="$$dest.tmp"; \
	mkdir -p "$$dest_dir"; \
	cp "$(BUILD_DIR)/$(BINARY_NAME)" "$$tmp"; \
	chmod +x "$$tmp"; \
	if [ "$$(uname -s)" = "Darwin" ]; then \
		codesign --force --sign - "$$tmp"; \
	fi; \
	mv "$$tmp" "$$dest"
	@echo "Installed $(BINARY_NAME) to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

tail-log: ## Tail the latest Centian log file
	@echo "Looking for latest log in $(LOG_DIR)..."
	@if [ -d "$(LOG_DIR)" ]; then \
		latest=$$(ls -t "$(LOG_DIR)"/* 2>/dev/null | head -n 1); \
		if [ -n "$$latest" ]; then \
			echo "Tailing $$latest"; \
			tail -f "$$latest"; \
		else \
			echo "No log files found in $(LOG_DIR)"; \
		fi; \
	else \
		echo "Log directory $(LOG_DIR) not found"; \
	fi

tag-release: ## Create and push a release tag (BUMP=major|minor|patch)
	@bash ./scripts/tag-release.sh $(BUMP)



# Release commands
release: check-main-branch ## Tag a new release (usage: make tag-release [major|minor|patch], defaults to patch)
	@./scripts/tag-release.sh $(filter-out $@,$(MAKECMDGOALS))

major minor patch:
	@:

inspect:
	npx @modelcontextprotocol/inspector centian stdio --cmd npx -- -y @modelcontextprotocol/server-memory
check-main-branch: ## Ensure the current git branch is main
	@branch=$$(git rev-parse --abbrev-ref HEAD 2>/dev/null); \
	if [ "$$branch" != "main" ]; then \
		echo "Error: release must be run from main (current branch: $$branch)"; \
		exit 1; \
	fi
