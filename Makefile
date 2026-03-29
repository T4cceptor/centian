# Build variables
BINARY_NAME=centian
BUILD_DIR=build
MAIN_PATH=./cmd/main.go
LOG_DIR=$(HOME)/.centian/logs
WEB_DIR=web
UI_DIST_DIR=internal/ui/dist

# Release bump (defaults to patch, can be set via `make release minor`)
BUMP ?= patch
ifneq ($(filter major minor patch,$(MAKECMDGOALS)),)
BUMP := $(filter major minor patch,$(MAKECMDGOALS))
endif

# Version info
VERSION ?= dev
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: help build build-go clean test test-integration test-everything test-realworld test-taskverification test-all test-coverage test-coverage-html lint fmt vet tidy run dev web-install web-dev web-build web-stage web-test web-preview web-clean ensure-web-tooling check-main-branch tag-release release major minor patch

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
		gotestsum --format testname -- -race ./internal/... ./cmd/...; \
	else \
		echo "Note: gotestsum not found, using default go test output"; \
		echo "Install with: go install gotest.tools/gotestsum@latest"; \
		go test -v -race ./internal/... ./cmd/...; \
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

test-all: test test-integration ## Run all tests (unit + integration)

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@mkdir -p build
	@go test -coverprofile=build/coverage.out -covermode=atomic ./internal/... ./cmd/...
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

lint: ## Run linter (requires golangci-lint)
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
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/$(BINARY_NAME)
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
