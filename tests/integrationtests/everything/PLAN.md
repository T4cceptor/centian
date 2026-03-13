# Everything MCP Integration Test Plan

## Goal

Build a dedicated integration-test harness around `@modelcontextprotocol/server-everything`
so Centian can be measured against a real capability-focused MCP server instead of
only against mocks or third-party public endpoints.

The main purpose is assessment:

- compare direct server behavior vs proxied behavior through Centian
- identify current protocol gaps in Centian
- create a stable base for later fixes without rewriting the test harness

## Principles

1. Use one dedicated test case per tool/capability from issue `#60`, even when
   multiple tools exercise similar transport paths.
2. Compare two paths for the same probe:
   - direct client -> everything server
   - client -> Centian -> everything server
3. Separate:
   - expected parity checks
   - known-gap probes
   - processor-overlay checks
4. Keep the suite opt-in until the downstream server dependency is stabilized in
   the repo and the first batch of known gaps is classified.

## Scope Phases

### Phase 1: Harness and Baseline

- create a dedicated `tests/integrationtests/everything/` package
- start a real downstream `everything` server from a configured command
- generate Centian config in-memory for the downstream server
- provide reusable helpers for:
  - direct SDK connection
  - proxied SDK connection
  - waiting for tool registration
  - capturing notifications and other client callbacks
- add an initial baseline comparison test for initialize + `tools/list`

### Phase 2: Tool and Metadata Matrix

Create focused comparison tests for the tool list from issue `#60`:

- `echo`
- `get-annotated-message`
- `get-env`
- `get-resource-links`
- `get-resource-reference`
- `get-roots-list`
- `get-structured-content`
- `get-sum`
- `get-tiny-image`
- `gzip-file-as-resource`
- `simulate-research-query`
- `toggle-simulated-logging`
- `toggle-subscriber-updates`
- `trigger-long-running-operation`

Metadata-specific checks:

- tool annotations and tool metadata flags
- output schemas
- content fidelity for text, structured content, image/resource-like content

### Phase 3: Capability and Notification Probes

Add explicit probes for behavior that is likely to diverge in Centian:

- progress notifications
- logging notifications
- roots handling
- resource subscription/update notifications
- elicitation and task/sampling related flows
- tool-list-changed notifications

These tests should classify results instead of immediately assuming parity:

- `match`
- `proxy divergence`
- `unsupported in Centian`

## Expected Early Findings

The current proxy implementation is tool-centric:

- downstream connections currently discover tools and forward tool calls
- the upstream proxy currently advertises only tool capabilities
- downstream tool calls are reconstructed from tool name + arguments

That means the first useful failures will likely be around:

- advertised capability mismatches during initialize
- missing progress forwarding
- missing logging/resource/roots notification forwarding
- missing request metadata passthrough needed by some advanced flows

## Processor Integration Strategy

Processor coverage should be layered on top of representative tool calls, not
mixed into every capability test.

Recommended initial overlays:

- `echo` for plain text payload changes
- `get-structured-content` for structured payload mutation/validation
- one resource-like tool for non-text content pass-through checks

Notification and roots/resource features should still be covered primarily by
the direct-vs-proxied harness, with processor behavior validated separately in
unit or targeted integration tests where necessary.

## Running Strategy

Initial harness is opt-in and expects the downstream server command to be
provided via environment variables:

- `CENTIAN_RUN_EVERYTHING_INTEGRATION=1`
- `CENTIAN_EVERYTHING_SERVER_CMD=<command>`
- `CENTIAN_EVERYTHING_SERVER_ARGS=<space separated args>`

This keeps the normal integration suite stable while the harness is being built
out and while the dependency strategy for `server-everything` is finalized.
