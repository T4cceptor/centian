# Changelog

All notable changes to this project will be documented in this file.

## v0.0.6 - 2026-03-18

### Major
- Added first downstream OAuth support for HTTP MCP servers, including browser-based Authorization Code + PKCE login, refresh-token handling, metadata discovery, and hosted `/oauth/start`, `/oauth/status`, and `/oauth/callback` routes.
- Added encrypted local storage for downstream OAuth tokens with a locally managed master key, plus downstream reconnect and tool resync after successful authorization.
- Added proxy-side OAuth tools so clients can inspect and complete downstream login flows with `centian.auth_status` and `centian.login.<server>`.

### Minor
- Added opt-in Centian test tools via `proxy.enableTestTools`, including the new `centian.test_notifications` tool for emitting session-scoped log notifications.
- Expanded config validation and README guidance for OAuth-enabled downstreams, including `proxy.web.publicBaseUrl`, supported client auth methods, and current OAuth limitations.
- Added unit and integration coverage for OAuth config validation, token storage, auth tools, login flows, reconnect behavior, token refresh during reconnect, and URL elicitation.
- Removed the flaky `uvx`-backed real-world fetch parity tests for now.

### Bugfixes
- Fixed downstream HTTP handling so OAuth-managed servers inject and refresh their own `Authorization` header instead of relying on forwarded client auth.
- Fixed OAuth lifecycle handling to surface `auth_required` and `refresh_failed` states cleanly and to retry downstream requests after refresh where possible.
- Fixed config file permissions to write potentially sensitive downstream client secrets with restricted file mode.

## v0.0.5 - 2026-03-16

### Major
- Expanded MCP parity across the proxy by forwarding downstream logging, propagating client capabilities and roots support, and adding downstream resource and resource-template capabilities.
- Added comprehensive `server-everything` integration coverage for tool parity, protocol capability probes, and metadata-sensitive flows.
- Added real-world integration coverage for external MCP servers, including filesystem and memory scenarios.

### Minor
- Added an auth context handler so processors can receive sanitized request authentication context without coupling to transport internals.
- Improved proxy test coverage around capability propagation, logging, resources, real-world parity, and metadata-preserving tool forwarding.
- Updated the `everything` integration documentation to reflect the current conformance-oriented test surface.

### Bugfixes
- Fixed aggregated tool-name normalization so processors and downstream calls keep the correct current and original tool names.
- Fixed logger synchronization issues that could surface under concurrent proxy activity.
- Fixed tool-call forwarding to preserve upstream `_meta` on proxied requests instead of rebuilding calls from only tool name and arguments.
- Fixed stdio downstream environment handling so configured env vars merge with the inherited OS environment instead of replacing it entirely.

## v0.0.4 - 2026-03-12

### Major
- Added `centian processor add` to register existing processors from a file path with inferred name and runtime command.
- Added a unified local demo stack under `./demo` with Docker Compose, seeded data, and ready-to-run logging and redaction gateway examples.
- Added `centian config restore` to restore the active config from a backup file with validation, confirmation prompts, and custom backup path support.
- Reworked proxy session handling to decouple upstream MCP sessions from reusable downstream connection pools keyed by caller identity and forwarded auth context.

### Minor
- Improved CLI processor execution logging and stderr visibility for easier debugging.
- Added configurable proxy/internal logging output and level settings through proxy config.
- Expanded README and demo documentation for processor setup, config restore behavior, and local demo quickstart flows.
- Moved the DeepWiki proxy coverage into an opt-in external integration test flow.
- Added coverage for processor add, config restore, proxy pool reuse/isolation, auth identity propagation, logging config validation, and demo-related regression paths.

### Bugfixes
- Fixed downstream teardown behavior so upstream reconnects or repeated initialization no longer unnecessarily reset client-facing sessions.
- Fixed downstream pool reuse to isolate connections correctly across caller identities and forwarded auth changes while preserving reuse for matching contexts.
- Fixed restore behavior to validate backup configs before overwrite and to support `~` and environment-variable expansion in backup paths.
- Fixed demo and processor routing/logging edge cases that previously caused unexpected failures or timeout-prone behavior in local demo scenarios.

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
- OAuth is not supported (upstream or downstream) in v0.0.3.
- Stdio MCP servers run on the host under the same user context as Centian.
- Proxy-level auth headers are shared across downstream requests.
