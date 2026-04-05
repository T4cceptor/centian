# Changelog

All notable changes to this project will be documented in this file.

## v0.3.3 - 2026-04-06

### Major
- reworked `simple_tdd` template, also benchmarking it in an A/B test: old vs. new, which was significantly better in first pass rate (task run without restart, fail, timeout), median time to completion, and tool calls/errors.
- added `project` as a super-level entity over gateways, enabling isolation support for auth, and configuration settings

### Minor
- Polished README.md to reflect current implementation properly
- Persisting client name and version per task run, allow display of which MCP client was used for a task run

### Bugfixes
- Fixed `Dockerfile` to properly use new build entry point - see `v0.3.1`

## v0.3.2 - 2026-04-03

### Minor
- Removed the Python runtime dependency from the local `centian demo` task flow by switching the bundled prompt and integrated taskverification template from a Python-specific TDD exercise to a language-agnostic guided TDD workflow that runs on stock Node.js tooling.
- Renamed and propagated the bundled demo task template to `guided_tdd_workflow`, updated the agent-runner assets accordingly, and refreshed the related regression fixtures/tests so the demo and taskverification coverage stay aligned.

### Bugfixes
- Fixed the README demo section to better explain the showcased workflow and to document the actual local prerequisites and behavior for the updated Node-based demo task.

## v0.3.1 - 2026-04-03

### Major
- Added a Codex adapter to the `centian demo` runner, including bundled Codex config assets, `CODEX_HOME` propagation, demo-local auth material handling, and supporting regression coverage so local demo runs now support Codex alongside the existing agent adapters.
- Added per-gateway `forceReadOnlyHints` proxy configuration to override exposed tool annotations to `readOnlyHint=true` at registration time for both downstream proxied tools and Centian's built-in auth/taskverification tools, making Codex-oriented demo flows safer to run through Centian as the trust boundary.

### Minor
- Added GitHub bug-report and feature-request issue templates plus a pull request template with a standard checklist for repository contributions.
- Added UTC build timestamp and short commit-hash metadata to the default `dev` version string used by local builds, while leaving explicitly versioned release builds unchanged.

### Bugfixes
- Moved the CLI entrypoint from `cmd/main.go` to repo-root `main.go` so `go install github.com/T4cceptor/centian@latest` now produces a `centian` binary instead of `main`, and aligned the Makefile, CI workflow, release workflow, and installation documentation with the corrected entrypoint path.

## v0.3.0 - 2026-04-03

### Major
- Overhauled the public documentation surface around the newer "control plane for AI agents" positioning, including a rewritten top-level `README`, a canonical docs index, and dedicated guides for getting started, configuration, processor development, HTTP proxy setup, taskverification, and MCP proxy best practices.
- Expanded the product-facing taskverification and demo presentation with new checked-in README media assets, updated task template guidance, and clearer README/demo/processors documentation so the current local evaluation path is easier to understand and promote.
- Standardized Centian's default proxy port to `9666` across runtime defaults, generated snippets, examples, fixtures, and operator-facing startup output so the product surface now has one canonical default endpoint.

### Minor
- Updated the release/install story to focus on the supported paths for v0.3: installer script, release binaries, Docker images, and source builds via `make install`.
- Removed the FreeBSD artifact from the automated GitHub release workflow and aligned generated release notes with the actual support matrix.
- Added regression coverage for the new default-port expectations, the init command's MCP client snippet output, and quiet metadata/help CLI behavior.
- Added a short bridge page for task template authoring and refreshed related docs/readmes so the canonical documentation paths are easier to discover.

### Bugfixes
- Fixed broken internal documentation links and corrected the README hero asset reference to use checked-in media that actually exists in the repository.
- Fixed `centian --version` and `centian --help` so they no longer initialize the internal logger or emit permission-related warnings in restricted environments.
- Moved duplicate taskverification prompt fixtures out of the visible test surface into archived `.tmp` storage to reduce repository noise in promotion-facing paths.

## v0.2.5 - 2026-04-02

### Major
- Bundled the current built-in taskverification templates into the Centian binary and introduced `task-templates/integrated/` as the explicit build-time source for embedded templates, while still allowing filesystem templates from `task-templates/` to load at runtime and override embedded templates by `task.id`.
- Removed the older `demo/taskverification` Docker demo in favor of the newer integrated `centian demo` flow, reducing duplicate demo paths and aligning local taskverification demos around the maintained agent-runner entrypoint.
- Reworked the processor demo container flow so `demo/processors` now builds a local demo image, layers the local Centian binary and demo processor sources into it, and keeps the required OpenTelemetry Python dependencies available for the logging processor demo.

### Minor
- Added a short `task-templates/README.md` describing the new distinction between disk-only templates and the stricter `integrated/` set that ships in release binaries.
- Updated the demo agent runner defaults to use `gemini-2.5-flash` as the Gemini model for the local `centian demo` workflow.
- Added startup logging for the taskverification command working directory so operators can immediately see which workspace Centian will use for taskverification command execution.
- Expanded template-loading and proxy startup tests to cover embedded-template fallback, local template override behavior, and task-tool exposure when no on-disk template directory exists.

### Bugfixes
- Fixed the task-run UI timeline so repeated phase sections created by task restarts no longer share collapse state; collapsing one `Planning`, `Scaffolding`, or repeated step section now only affects that specific occurrence.
- Fixed the processor demo image setup so the demo no longer depends on the published `t4ce/centian:latest` binary contents alone and instead consistently includes the local demo processor code and OTEL Python packages required by the logging demo.

## v0.2.4 - 2026-04-01

### Major
- Added a required `planning.planSummary` field to the taskverification planning artifact so each run freezes a human-readable execution summary at planning completion, and surfaced that summary through `centian.task_complete_planning`, structured task-tool output, and persisted planning-completed event payloads.
- Hardened the local `centian demo` workflow so repeated runs can safely reuse the same demo root while preserving only durable history artifacts (`events.sqlite`, internal log, and agent stdout/stderr logs) and recreating all disposable demo assets such as workspace, templates, generated config, prompt, and agent MCP config files.
- Refined the embedded task-run UI timeline and inspector behavior so Centian lifecycle events and correlated built-in task-tool exchanges render more consistently, including improved hiding of raw `centian.task_*` MCP rows once they are represented by task lifecycle events.

### Minor
- Updated taskverification authoring guidance and runtime/proxy test coverage for the frozen `planSummary` planning contract, including validation and payload assertions.
- Updated the demo CLI completion flow to print the active UI URL and prompt operators to shut the demo server down explicitly after the agent run finishes.
- Added demo-root safety checks for reused runs, including recognition of prior demo layouts, stale PID cleanup, and clear failure when a previous demo server is still running from the same root.
- Expanded task-run route coverage for timeline correlation, inspector status normalization, live duration updates, and terminal duration freeze behavior.

### Bugfixes
- Fixed taskverification planning completion to fail fast when `planning.planSummary` is missing or blank instead of freezing an unreadable planning artifact.
- Fixed `centian demo` to append agent stdout/stderr logs across runs with clear per-run separators instead of truncating prior output.
- Fixed the task-run timeline to avoid leaving stray raw Centian task-tool request rows visible when a matching task lifecycle event already exists, even when the raw MCP event appears earlier in the persisted event stream.
- Fixed section header status badges in the task-run detail timeline so completed Centian phases render as completed instead of incorrectly staying active.
- Fixed inspector status labels and colors so Centian task events consistently show `success`, `error`, or `timed_out` with matching visual treatment instead of mixing `active`, `succeeded`, `ok`, or mismatched colors.
- Fixed the detail-page duration counter to keep ticking while the run remains active and to freeze only when the final derived run state becomes `failed` or `timed_out`.

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
