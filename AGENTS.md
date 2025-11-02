# Repository Guidelines

## Project Structure & Module Organization
Centian CLI is a Go workspace. The executable entrypoint lives in `cmd/main.go`, while feature modules sit under `internal/cli`, `internal/proxy`, `internal/logging`, and `internal/config`. Shared libraries that need to be exported reside in `pkg/`. Generated binaries and artifacts belong in `build/`. Test data and configuration fixtures are kept in `test_configs/`—keep new fixtures there to avoid polluting source directories.

## Build, Test, and Development Commands
Use Go tools or the Makefile targets. `go build -o build/centian ./cmd/main.go` compiles the CLI once. `make build` wraps the same build with version metadata. `make dev` runs the full developer loop (`clean`, `fmt`, `vet`, `test`, `build`). Run unit tests with `go test ./...`; add `-race` locally when touching concurrency. `make test` enables verbose race-detected tests.

## Coding Style & Naming Conventions
Adhere to Go defaults: tabs for indentation, `go fmt ./...` before pushing, and idiomatic lower-case package names. Favor descriptive but concise exported names (e.g., `StdioProxy`, `LogReader`). Run `golangci-lint run` when available; the Makefile exposes it via `make lint`. Keep configuration structs and JSON/YAML tags in sync with files in `docs/` and `test_configs/`. When adding CLI flags, mirror naming patterns already in `internal/cli`.

## Testing Guidelines
Stage unit tests alongside implementation packages using `_test.go` suffixes. Prefer table-driven cases and embed behavior summaries using "Given/When/Then" comments for clarity. Place longer fixtures under `test_configs/`. Validate concurrent code with `go test -race ./...`. For new proxy flows, include smoke tests that exercise end-to-end stdio communication. If a bug survives two iterations, capture it with a failing test before attempting another fix.

## Debugging & Workflow
Start bug investigations with a quick architecture-level outline of the suspected root cause before editing code. Highlight important edge cases inline, but defer low-likelihood handling until after core paths are solid. Use the Makefile targets to keep the feedback loop tight and share reproduction steps in PR descriptions.

## Commit & Pull Request Guidelines
Commits in this repo use concise, sentence-case summaries (`Fixed issue...`, `Restructured project`). Keep the first line under 72 characters and describe the behavior change, not the implementation. Commit after finishing milestones (roughly every 20–30% of the work). For pull requests, link relevant issues, summarize observable impact, outline test coverage, and attach CLI output or logs when the change affects runtime behavior. Ensure the branch is rebased onto `main` and all Makefile checks pass before requesting review.
