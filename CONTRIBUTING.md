# Contributing

Thanks for your interest in contributing to Centian.

## Getting Started
- Fork the repo and create a branch from `main`.
- Install Go (1.25+ recommended) and make sure `go` is on your PATH.

## Development Workflow
- Build: `make build`
- Test: `make test-all`
- Lint: `make lint`
- Full loop: `make dev`

## Coding Standards
- Run `go fmt ./...` before pushing.
- Prefer idiomatic Go and small, focused changes.
- Add or update tests for behavior changes.
- Keep configuration structs and docs in sync.

## Commit Messages
Use concise, sentence‑case summaries under 72 characters, describing the behavior change (e.g., "Fixed proxy auth validation").

## Pull Requests
Include:
- Summary of the change and user impact
- Tests run
- Any relevant logs or screenshots

By participating, you agree to follow the Code of Conduct in `CODE_OF_CONDUCT.md`.
