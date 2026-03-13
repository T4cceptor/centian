# Everything MCP Integration Tests

This package contains integration tests for comparing a direct connection to
`@modelcontextprotocol/server-everything` with the same server accessed through
Centian.

The main goal is to show where Centian matches MCP behavior and where it
currently diverges.

## What Is Implemented

The current test package covers three layers:

1. Baseline connection and tool discovery
2. Tool and metadata parity checks for deterministic tool calls
3. Capability probes for logging, resources, roots, sampling, elicitation, and
   other protocol surfaces that are expected to be weaker in Centian today

## How The Harness Works

Each comparison test creates two connections:

- direct client -> `server-everything`
- client -> Centian -> `server-everything`

The harness uses the same Go SDK client shape for both paths so differences are
attributable to Centian rather than to the test client setup.

The client is configured with:

- roots support
- sampling support
- elicitation support
- logging notification handlers
- progress notification handlers
- resource update handlers

## Running The Tests

Preferred:

```bash
make test-everything
```

Directly with `go test`:

```bash
CENTIAN_RUN_EVERYTHING_INTEGRATION=1 \
GOCACHE=/tmp/go-build \
go test -v ./tests/integrationtests/everything/...
```

By default the harness launches:

```bash
npx -y @modelcontextprotocol/server-everything
```

You can override that with:

- `CENTIAN_EVERYTHING_SERVER_CMD`
- `CENTIAN_EVERYTHING_SERVER_ARGS`

Example:

```bash
CENTIAN_RUN_EVERYTHING_INTEGRATION=1 \
CENTIAN_EVERYTHING_SERVER_CMD=node \
CENTIAN_EVERYTHING_SERVER_ARGS="/path/to/server-everything/dist/index.js" \
GOCACHE=/tmp/go-build \
go test -v ./tests/integrationtests/everything/...
```

## What The Tests Assert

### Baseline

- direct and proxied sessions both initialize
- tools can be listed on both paths
- tool catalog differences are shown as directional diffs

### Phase 2

- proxied tool catalog matches the direct catalog
- proxied tool metadata matches the direct metadata
- deterministic tools return the same result directly and through Centian

Current deterministic tool-call coverage includes:

- `echo`
- `get-sum`
- `get-structured-content`
- `get-annotated-message`
- `get-resource-links`
- `get-resource-reference`
- `get-tiny-image`
- `gzip-file-as-resource`
- `get-roots-list`

### Phase 3

Phase 3 probes classify results as:

- `match`
- `proxy_divergence`
- `unsupported_in_centian`

These probes currently focus on:

- `trigger-long-running-operation`
- `toggle-simulated-logging`
- `trigger-sampling-request`
- `trigger-elicitation-request`
- `simulate-research-query`
- `resources/list`

## Interpreting Failures

Not every failure means the test harness is wrong. The suite is intentionally
designed to expose current Centian gaps.

Examples of meaningful failures:

- tool exists directly but is missing through Centian
- direct path receives notifications but proxied path does not
- roots are visible directly but not through Centian
- direct server exposes resources or other capabilities that the proxied path
  hides

If a failure includes `proxy_divergence` or `unsupported_in_centian`, it should
generally be treated as a product gap to evaluate, not as a flaky test by
default.

## Current Supporting Material

- Plan: `tests/integrationtests/everything/PLAN.md`
- Issue drafts: `tests/integrationtests/everything/ISSUE_DRAFTS.md`
- Example captured output: `tests/integrationtests/everything/tmp/`

## Notes

- These tests are intentionally separate from the general integration suite
  because they rely on a real downstream MCP server implementation.
- The package currently focuses on assessment and gap discovery, not on
  providing a permanently green conformance suite yet.
