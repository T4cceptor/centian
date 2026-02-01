# Changelog

All notable changes to this project will be documented in this file.

## v0.1.0 - 2026-01-31

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
