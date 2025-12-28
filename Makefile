# Build variables
BINARY_NAME=centian
BUILD_DIR=build
MAIN_PATH=./cmd/main.go
LOG_DIR=$(HOME)/.centian/logs

# Version info
VERSION ?= dev
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: help build clean test test-integration test-all lint fmt vet tidy run dev

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s", $$1, $$2}'

build: ## Build the MCP proxy binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)

test: ## Run unit tests
	@echo "Running unit tests..."
	go test -v -race ./internal/... ./cmd/...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	go test -v ./integration_tests/...

test-all: test test-integration ## Run all tests (unit + integration)

lint: ## Run linter (requires golangci-lint)
	@echo "Running linter..."
	golangci-lint run

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

release: ## Create and push a new patch release
	@echo "Creating new patch release..."
	@# Get the latest tag, increment patch version
	@LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	echo "Latest tag: $$LATEST_TAG"; \
	NEW_TAG=$$(echo $$LATEST_TAG | awk -F. '{$$NF = $$NF + 1;} 1' | sed 's/ /./g'); \
	echo "New tag: $$NEW_TAG"; \
	git tag $$NEW_TAG; \
	git push origin $$NEW_TAG; \
	echo "✅ Released $$NEW_TAG - check GitHub Actions for build status"
