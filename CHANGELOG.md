# Changelog

All notable changes to this project will be documented in this file.

## v0.2.3 - 2026-04-01

### Major
- Refactored taskverification registration and planning so `centian.task_register` only selects a template and all template parameters are frozen later through `planning.parameters`, removing the earlier draft-parameter shell and making onboarding/planning decisions the source of truth for execution.
- Added stronger agent-facing taskverification workflow metadata and validation, including required planning parameter exposure, enforced `planning.parameters` schema requirements, structured planning-contract failures, and richer current-step contracts with check and invariant explanations.
- Added initial Gemini support to `centian demo`, including embedded Gemini settings, demo wiring, and supporting test coverage for the new demo agent option.

### Minor
- Updated built-in taskverification templates, demo prompts, integration fixtures, and authoring documentation to use planning-time parameters and `required_inputs` consistently.
- Expanded proxy and taskverification tests to cover the new planning contract, structured task-tool payloads, step metadata, and demo/taskverification integration flows.
- Updated the task run UI/task state payloads to surface the newer planning metadata without relying on the removed draft-parameter model.

### Bugfixes
- Fixed brittle taskverification planning behavior where parameters were effectively decided too early and could diverge from onboarding/planning context.
- Fixed agent-facing failure responses so planning-contract problems return actionable structured details instead of only plain string errors.
- Fixed taskverification step metadata gaps by preserving authored check/invariant descriptions and exposing concise technical meanings for built-in file/output checks and invariants.

## v0.2.2 - 2026-04-01

### Major
- Added a host-native black-box taskverification integration harness under `tests/integrationtests/taskverification` that runs real local coding agents against a real Centian process, preserves artifacts under `.tmp/`, and asserts the task lifecycle from Centian task runs, events, request logs, and produced project files.
- Added the new `centian demo` command for a self-contained local product demo, backed by a new `internal/agentrunner` package with embedded demo assets, a generated Centian config and workspace, a Claude headless adapter, preserved agent stdout/stderr logs, and automatic UI startup while keeping Centian running after the agent exits.

### Minor
- Added detailed black-box integration documentation in `tests/integrationtests/taskverification/README.md`, plus README coverage for the new `centian demo` flow, generated artifacts, and current agent support.
- Added explicit task-tool annotations and richer agent-facing metadata across the `centian.task_*` tools, including `workspaceRoot`, `pathMode`, `commandWorkingDirectory`, and short `nextAction` guidance to make the taskverification workflow easier for agents to follow.
- Added a dedicated demo integration test and expanded taskverification fixture assets to keep the runtime demo flow and the stricter black-box harness aligned.

### Bugfixes
- Fixed taskverification step verification to support `output_contains` and `output_not_contains` conditions over combined stdout/stderr, removing the need for agent-side workarounds when standard-library Python test failures write to stderr.
- Fixed the task run UI to poll the list view in place, poll active detail views once per second, pause hidden-tab refreshes, and avoid incorrectly stopping detail polling after non-terminal failed step events.
- Fixed the host-native taskverification harness to use a consistent project-root workspace model with versioned local configs/prompts/templates instead of ad hoc demo-path assumptions.

## v0.2.1 - 2026-03-29

### Major
- No code delta relative to `v0.2.0`. `v0.2.1` tags the same `Task Verification - Phase 1 (#108)` release commit and serves as a republished release marker for that rollout.

## v0.2.0 - 2026-03-29

### Major
- Added taskverification as an opt-in workflow-driven task runtime on top of Centian's MCP proxy surface, including the `centian.task_*` lifecycle tools for registration, onboarding, planning, execution, restart, and explicit failure handling.
- Added persisted task-run observability with a SQLite-backed event store, a read-only task-run API (`GET /api/task-runs` and `GET /api/task-runs/{runID}/events`), and an embedded UI under `/ui/tasks` and `/ui/tasks/:runID`.
- Added workflow-path and node-kind based task execution with planning contracts, step checks, invariants, approval-wait nodes, and downstream tool governance tied to the active workflow node.
- Added the `demo/taskverification` Docker-based demo flow with sample templates, persisted timelines, approval-wait coverage, and headless agent/e2e scenarios for manual and automated validation.

### Minor
- Added comprehensive documentation for taskverification enablement, capability boundaries, run inspection, current gaps, and recommended usage in `docs/TASKVERIFICATION.md`, plus README updates for opt-in usage and UI-integrated builds.
- Added frontend build and embedding support across CI, release binaries, and Docker images, including the `make build-go` fallback path for local Go-only builds when rebuilding the frontend is unnecessary.
- Expanded test coverage across the taskverification runtime, proxy integration, persistence store, API handlers, embedded UI, build paths, and demo/e2e flows.
- Hardened the release path for the integrated UI by aligning Docker and release builds on frontend staging before Go compilation and by documenting the `v0.2.0` release readiness checks in this changelog.

### Bugfixes
- Protected the task-run API with the existing API-key model while keeping the embedded UI shell reachable and adding a minimal browser-side API-key retry flow for protected API fetches.
- Replaced destructive event-store schema reset behavior with fail-closed schema mismatch handling that preserves existing data until an explicit migration is available.
- Fixed race-prone task-run bookkeeping by switching tool-call event/context recording to lock-safe run snapshots instead of reading mutable run state after unlock.
- Fixed planning-output validation so all required scalar planning fields are actually enforced before execution can proceed.
- Fixed file-based taskverification conditions to reject absolute paths and path traversal outside the task working directory.
- Fixed persistence read helpers to return explicit errors instead of silently returning empty result sets on database failures.
- Fixed timeline normalization and detail selection in the embedded UI so malformed exchange rows are skipped instead of causing invalid anchor dereferences.
- Fixed build and release flows to always embed a valid frontend artifact, while preserving a documented Go-only fallback build path for local development.

### Known limitations
- Mutable task run state is in-memory only and is not durably restorable yet.
- The embedded UI is read-only and does not perform task control actions.
- SQLite is the only implemented event storage backend.
- Task templates are filesystem-based and must be present on disk.
- Approval waits can block execution, but there is no dedicated approve/resume tool yet.

## v0.1.0 - 2026-03-20

### Major
- Formalized the processor `DataContext` contract as v1.0 with golden fixtures, updated scaffolds, and clarified that processors run around proxied `tools/call` requests and results.
- Added retry handling for downstream connection establishment to make pooled downstream recovery more resilient to transient failures.
- Changed aggregated resource and resource-template collision handling to fail loudly instead of silently hiding conflicting downstream entries.

### Minor
- Expanded tests and helper coverage for the formalized processor contract, downstream retry behavior, and aggregated resource collision scenarios.
- Clarified README and processor documentation around processor scope, timeout behavior, downstream OAuth PKCE `S256` expectations, and demo setup details.
- Added a release safeguard so the release target must be run from `main`.

### Bugfixes
- Fixed MCP event/logging structures and related handler behavior to match the formalized processor contract consistently.
- Fixed OAuth metadata handling and tests to validate `S256` PKCE support more explicitly.

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
