# Changelog

All notable changes to this project will be documented in this file.

## v0.0.3 - 2026-02-10

### Major
- Refactored processing architecture to a handler-driven `CallContext` flow in proxy.
- Introduced default handlers for payload, metadata, routing, and logging parts.
- Moved MCP event ownership into call context lifecycle; added `WithToolRequest` and `WithToolResult`.
- Reworked processor contract to `processor.DataContext` with structured `event`, `payload`, and `routing` parts.
- Added `ToolCallContext` abstraction with immutable original request/result and mutable active state.
- Reworked processor scaffolding for all supported languages to align with `DataContext`; Python scaffolds now include built-in dataclasses.

### Minor
- Improved scaffold contract consistency and generated test input shape.
- Added broad unit/integration test coverage for proxy handlers, tool call context, processor CLI execution, and scaffolding outputs.
- Expanded CLI init command coverage and helper-level tests.
- Added a small UUID generator test seam in proxy utils (no behavior change).
- Updated tests and fixtures to align with refactored processor and call-context APIs.

### Bugfixes
- Fixed processor output application path in tool-call flow for request/result mutations.
- Fixed request/response processor fixture behavior to follow direction-aware handling.
- Fixed JSON marshaling failure in CLI processor caused by non-serializable MCP request fields (`Extra.CloseSSEStream`) using a safe DTO input format.
- Fixed scaffold/runtime schema drift across Python/JS/TS/Bash templates.
- Fixed compile/lint/test regressions introduced during iterative refactoring.

## v0.0.1 - 2026-01-31

### Added
- MCP HTTP proxy with aggregated gateway and single-server endpoints.
- Gateway aggregation with tool namespacing to avoid collisions.
- Processor scaffolding with optional auto-add to config.
- Structured logging to `~/.centian/logs/` for requests and proxy events.
- Auto-discovery of MCP configs from common tools (Claude Desktop, VS Code, generic).
- CLI commands for init, start, auth (API keys), config and logs.
- API key authentication with configurable header.

### Changed
- Default proxy bind host is `127.0.0.1`.
- Binding to `0.0.0.0` requires an explicit `auth` setting to reduce accidental exposure.

### Known limitations
- OAuth is not supported (upstream or downstream) in v0.1.
- Stdio MCP servers run on the host under the same user context as Centian.
- Proxy-level auth headers are shared across downstream requests.
