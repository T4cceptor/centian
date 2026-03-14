# Repository Guidelines (Source of Truth)

This file is authoritative for repo workflows, structure, and expectations.
Other agent guidance should link here and avoid duplication.

## Repository Identity & Purpose
Centian is the MCP proxy for this repository.

- Repository owner: `T4cceptor`
- Repository name: `centian`
- Repository URL: `https://github.com/T4cceptor/centian`
- Primary role: proxy MCP communication through a single CLI-managed endpoint
- Core capabilities:
  - Proxy MCP communication to downstream systems
  - Log and monitor MCP requests and results
  - Process requests and responses to inspect, modify, or block them
  - Aggregate downstream systems behind one endpoint, using tool namespacing to keep them distinguishable

## Project Structure & Module Organization
Centian is a Go CLI application. The executable entrypoint lives in `cmd/main.go`.

Basic repository structure:
- `cmd/`: CLI entrypoint and top-level command wiring
- `internal/cli/`: command handlers for config, logs, auth, processor, server, and completion flows
- `internal/proxy/`: MCP proxy server, downstream connection handling, tool-call context, and request/response processing hooks
- `internal/logging/`: log readers and shared logging helpers
- `internal/config/`: config models, validation, accessors, and config-oriented CLI operations
- `internal/processor/`: processor interfaces, scaffolding, and CLI processor execution support
- `internal/discovery/`: discovery and parsing of MCP-related configuration sources
- `internal/auth/`: API key and authentication helpers
- `internal/common/`: shared MCP models, auth context, utilities, and internal logging primitives
- `docs/`: architecture and setup documentation
- `demo/`: demo assets, configs, and supporting scripts
- `scripts/`: repository scripts
- `tests/test_configs/`: config fixtures and sample MCP client/server configs
- `tests/integrationtests/`: end-to-end and processor integration coverage
- `build/`: compiled binaries and generated artifacts

There is currently no active `pkg/` export surface in this repository; prefer `internal/` packages unless a true public package is needed.

## Build, Test, and Development Commands
Use Go tools or the Makefile targets. `go build -o build/centian ./cmd/main.go` compiles the CLI once. `make build` wraps the same build with version metadata. `make dev` runs the full developer loop (`clean`, `fmt`, `vet`, `test`, `build`). Run unit tests with `go test ./...`; add `-race` locally when touching concurrency. `make test` enables verbose race-detected tests. Run `golangci-lint run` when available; the Makefile exposes it via `make lint`.

## Coding Style & Naming Conventions
Adhere to Go defaults: tabs for indentation, `go fmt ./...` before pushing, and idiomatic lower-case package names. Favor descriptive but concise exported names (e.g., `StdioProxy`, `LogReader`). Keep configuration structs and JSON/YAML tags in sync with files in `docs/` and `tests/test_configs/`. When adding CLI flags, mirror naming patterns already in `internal/cli`.

## Testing Guidelines
Stage unit tests alongside implementation packages using `_test.go` suffixes. Prefer table-driven cases and embed behavior summaries using "Given/When/Then" comments for clarity. Place longer fixtures under `tests/test_configs/`. Validate concurrent code with `go test -race ./...`. For new proxy flows, include smoke tests that exercise end-to-end stdio communication. If a bug survives two iterations, capture it with a failing test before attempting another fix.

## Debugging & Workflow
Start bug investigations with a quick architecture-level outline of the suspected root cause before editing code. Highlight important edge cases inline, but defer low-likelihood handling until after core paths are solid. Use the Makefile targets to keep the feedback loop tight and share reproduction steps in PR descriptions.

## Commit & Pull Request Guidelines
Commits in this repo use concise, sentence-case summaries (`Fixed issue...`, `Restructured project`). Keep the first line under 72 characters and describe the behavior change, not the implementation. Commit after finishing milestones (roughly every 20–30% of the work). For pull requests, link relevant issues, summarize observable impact, outline test coverage, and attach CLI output or logs when the change affects runtime behavior. Ensure the branch is rebased onto `main` and all Makefile checks pass before requesting review.
