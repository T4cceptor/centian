# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Major
- Made task runs **resumable across server restarts and session reconnects**. `centian.task_resume` now accepts an optional `runId`: with no `runId` it resumes the timed-out run in the current session (unchanged), and with a `runId` it hydrates the persisted snapshot back into a fresh session — recompiling the frozen templates through the normal compilation path — so normal task tools continue from the saved phase. Active runs continue as-is; timed-out runs are reactivated. A run now records the principal that registered it (new `owner_principal_id` column on `task_runs`, schema v8), and restoring another principal's run is denied when auth is enabled.
- Enforced **one open task run per principal**. While a principal has an `active` or `timed_out` run, `centian.task_register` is rejected with structured guidance to either resume it (`centian.task_resume {runId}`) or abandon it (`centian.task_fail {runId}`) first. `centian.task_fail` accepts an optional `runId` so a returning principal can fail a stale run without first resuming it. Enforcement applies only when a principal identity is present; legacy runs without a recorded owner are unaffected.
- Added **PostgreSQL** as a backend alongside SQLite for all SQL models (task/action events, task runs, benchmarks) and the SQL-backed auth principal store. A new `internal/sqldb` connector selects the driver (SQLite via sqliteshim, Postgres via the pgx stdlib driver + Bun `pgdialect`) and adapts the canonical SQLite DDL to Postgres at bootstrap (`BLOB → JSONB`, `INTEGER → BIGINT`, `REAL → DOUBLE PRECISION`); JSON payload columns become queryable `jsonb`. Event storage selects Postgres per project via `capabilities.eventStorage` (`driver: "postgres"` + `dsn`, one database per project), and the global `authBackend` gains a `postgres` type (`store` = DSN). SQLite remains the default; nothing changes unless `driver`/`type` is set to `postgres`. See [docs/postgres.md](docs/postgres.md).

## v0.4.4 - 2026-06-07

### Major
- Introduced a `PrincipalProvider` authentication abstraction and an `Authorizer` authorization seam. Authentication now resolves a token to a first-class `Principal` (with a stable, persisted `pr_` id), and the proxy keys request identity, downstream-pool reuse, and authorization off that principal. Two providers ship: a file-based one (backed by `~/.centian/api_keys.json`) and a SQL-backed one (see below); external providers and RBAC roles are designed as additive follow-ups.
- Added a SQL-backed `PrincipalProvider` that stores principals and credentials in a dedicated global SQLite database (`~/.centian/principals.sqlite` by default), independent of the per-project event store. Credentials are modelled generically (a `type` discriminator plus an opaque JSON `data` blob) so future credential types are additive, and authorization grants live in symmetric `principal_gateways`/`principal_projects` tables. The backend is selected via the new global `authBackend` config block (`type` + `store`), and `centian auth new-key` writes to that same backend (use `--config` to target a specific config file), so the CLI and server never drift. SQLite is the default for both the server and key creation.
- Captured the resolved principal's display name as `principal_name` event metadata (in the existing payload, no schema change) alongside `principal_id`, so the human name survives even if the principal id can no longer be resolved to a live principal.
- Replaced the O(n) bcrypt scan on every authenticated request with an O(1) credential-id lookup plus a single bcrypt verification.
- Added a built-in processor toolkit: new redaction processors (`pattern_redaction_processor`, `pii_redactor`, `secret_token_redactor`) and a `tool_call_guard` that can block or annotate tool calls (including path-boundary enforcement), all built on a shared `builtinutil` package (text scanning, redaction, tool-guard, and result helpers). The existing `prompt_injection_guard` was refactored onto the same toolkit, and the processor config surface gained schema definitions and validation for the new processors.
- Added the per-project Activity view ("Intervention Skyline") and its backing API: a timeline that visualizes where Centian intervened on proxied MCP traffic, with headline stats (Interventions, Actions Blocked, Redacted, Context in/out, Requests inspected), a category legend that hides empty categories, an interactive SVG skyline (severity-scaled, category-colored markers over a request-volume baseline) with hoverable/pinnable detail popovers, drag-to-zoom plus double-click zoom-out, a "Live" auto-refreshing 90-second window, and a principal filter. Served by new `GET /api/{projectSlug}/activity` and `GET /api/{projectSlug}/principals` endpoints, with a `ListPrincipals` capability added to both the file- and SQLite-backed principal providers. Activity is now the default landing route and first nav tab.

### Minor
- Refactored the global Events view (`/ui/{projectSlug}/events`): a reworked layout with a dedicated governance-events section and a "with governance event" filter (`withGovernanceEvent`), backed by a new persistence query, making it easier to find events where a processor intervened.

### Breaking changes
- **The default auth backend is now SQLite** (`~/.centian/principals.sqlite`). Existing file-based deployments must either set `"authBackend": {"type": "file"}` in config (to keep reading `api_keys.json`) or recreate their keys with `centian auth new-key`, which now writes to SQLite by default. There is no automatic `api_keys.json` → SQLite import.
- **API key token format changed** to `sk-<credentialId>.<secret>`, and only the secret is bcrypt-hashed. Existing `api_keys.json` keys are invalidated; regenerate them with `centian auth new-key`. Pre-principal tokens are rejected with a clear "regenerate" error.
- **Stored key entries gain a persisted `principal_id`** field; keys created before this change must be regenerated to receive one.
- **Processor `AuthContext.principal_id` changed** from a gateway-scoped `sha256(keyID:gateway)` hex value to the stable, gateway-independent `pr_` principal id (`key_id` remains the credential id).
- **Persisted downstream OAuth bindings are orphaned on upgrade** because they were keyed on the old `auth:<keyId>` identity; affected downstreams must be re-authorized.

## v0.4.3 - 2026-06-01

### Major
- Added the built-in `prompt_injection_guard` processor, including annotate/redact/remove/error modes, prompt-injection governance annotations, and fail-closed validation requiring `required: true` plus `payload` and `annotations` processor parts.
- Added normalized processor annotation persistence for MCP/action events through the new `event_annotations` SQLite table, with typed API projection for task-run detail and global event responses.
- Made projects first-class in the UI/API by adding `/api/projects`, project-scoped task/event APIs under `/api/{projectSlug}/...`, and project-aware UI routes under `/ui/{projectSlug}/...`.
- Reworked `centian demo` into a diskless in-memory IT Ops demonstration that starts a temporary local server, seeds the bundled governed incident run, and opens directly to the demo task-run detail view.
- Redesigned the task-run detail UI around compact metadata, task description, governance events, annotation category icons, saved section states, and hoverable governance context on timeline events.

### Minor
- Scoped task verification writes, MCP action-event persistence, and request JSONL loggers per project so named projects write only to their own stores and log directories.
- Exposed persisted task-run snapshots through the task-run detail API so the UI can use the frozen runtime template instead of the current config template.
- Added `task_description` and optional `annotations` support to Centian task tools, with generated governance annotations for process failures, failed checks, and tool allowlist denials.

### Bugfixes
- Fixed `/api/projects` authorization so project-scoped API keys only see project metadata they are allowed to access.
- Collapsed the unreleased annotation schema migration into schema v7 and ensured fresh/migrated databases create the final annotation columns and indexes.
- Fixed demo and OAuth integration test isolation by avoiding stale local event stores when using temporary demo or test storage.

## v0.4.2 - 2026-04-23

### Minor
- Added per-gateway task verification control via `verificationRequirement`, so each gateway can now independently run with task verification `off`, `optional`, or `required` instead of inheriting one project-wide enforcement mode.
- Added a global persisted MCP events feed with the new `/api/events` API and `/ui/events` page, making it possible to inspect MCP request/response traffic even when an event is not associated with a task run.
- Updated README and task verification/configuration docs to describe the new gateway-level verification modes and the broader event-observability flow.

## v0.4.1 - 2026-04-21

### Major
- Refactored benchmark persistence around a normalized `benchmark_run_task_runs` relation, added the `v5 -> v6` migration path for existing SQLite data, removed persisted `latest_task_run_*` and JSON-linked task-run fields from `benchmark_runs`, and switched benchmark scoring/read paths to operate on the full linked task-run set instead of a privileged “latest” run.

### Minor
- Extended the task-run API and embedded UI so benchmark/task-run relationships are navigable in both directions: `/api/task-runs` now supports benchmark-suite filtering, task-run detail views surface a direct benchmark-run link when applicable, and benchmark suite/run pages now link back into the relevant task-run views.
- Reworked the benchmark suite UI to better support post-v0.4 inspection flows, including compact suite headers, top-level `Overview`/`Sessions`/`Runs History` tabs, cleaner benchmark run detail cards, and removal of redundant linked-run/context panels.
- Added frontend test and typecheck coverage to the standard developer workflow so `make test-all` now runs the web Vitest suite and `make lint` also validates the frontend TypeScript surface.
- Updated benchmark persistence and product documentation to describe the new relational benchmark/task-run model and the benchmark-linked task-run inspection workflow.

### Bugfixes
- Fixed a race between proxy session bootstrap/synchronization and downstream tool-call handling by routing mirrored session state reads through lock-safe accessors, which stabilizes the flaky `TestServerStartIntegration` path under `-race`.
- Fixed the benchmark suite frontend route tests to match the updated suite-header rendering and restored passing frontend test coverage after the UI cleanup pass.

## v0.4.0 - 2026-04-18

### Major
- Added a first-class benchmarking workflow for taskverification, including the new `centian benchmark run` CLI, repeatable suite/case fixtures, preserved per-run artifacts, inline run scoring, and SQLite-backed persistence for benchmark sessions, runs, and derived scorecards.
- Added benchmark observability to the embedded product surface with read-only API routes under `/api/benchmarks/*` and new UI pages under `/ui/benchmarks` for suite, session, run, and comparison views broken down by template variant and agent/model.
- Refactored taskverification runtime state around a dedicated service and persisted run snapshots/stats, freezing richer execution contracts at planning completion and surfacing compact workflow/lifecycle responses plus structured recovery hints for step execution.

### Minor
- Added `codex-ollama` support to the local `centian demo` and benchmark runner flows, including explicit Codex profile/config handling, shared model/profile flag normalization, and updated README guidance for hosted vs local Codex runs.
- Added repository-side benchmark documentation, checked-in benchmark suites for `simple_tdd` and the demo workflow, smoke coverage, and Makefile helpers for running and inspecting local benchmark sessions.
- Added conservative tool-hint override options for gateways (`forceReadOnlyHints` and `forceSafeToolHints`) and propagated those metadata overrides across Centian-owned tools and downstream registrations.

### Bugfixes
- Fixed taskverification step failures so failed preconditions, postconditions, and invariant checks stay retryable in place instead of unnecessarily forcing a restart.
- Fixed task-run persistence and derived stats so benchmark and task UI/API reads can rebuild consistent timing, call-count, and failure aggregates from persisted snapshots and linked action/task events.
- Fixed the Codex runtime config patching flow to replace existing MCP/project blocks deterministically and to keep demo-local Codex runs on conservative non-destructive defaults.

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
