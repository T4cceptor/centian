# MCP Proxy Best Practices

This guide is specific to running Centian well. It is not a general MCP design guide.

## Gateway Design

Prefer one aggregated gateway per workflow context, not one giant gateway for everything.

Good gateway boundaries:

- one gateway per team
- one gateway per project
- one gateway per trust boundary
- one gateway per agent workflow bundle

Use a single-server route when:

- you want to expose exactly one downstream server
- namespacing is unnecessary
- you are debugging one specific downstream server

Use the aggregated gateway when:

- an agent needs several tools at once - e.g. for workflow management (taskverification)
- you want Centian to namespace tools cleanly
- you want one MCP client entry instead of several

## Auth Defaults

Keep `auth` enabled unless you are doing short-lived local development.

Recommended practice:

- local experimentation on `127.0.0.1`: disabling auth can be acceptable
- shared machines, demos, or anything bound beyond loopback: keep auth enabled
- if you bind to `0.0.0.0`, set `auth` explicitly and treat that deployment as exposed infrastructure

Remember:

- the Centian auth header is for Centian only
- downstream auth belongs in downstream server headers or downstream OAuth config

## Downstream Isolation

Group downstream servers by trust level.

Do not mix:

- highly privileged local filesystem access
- broad shell access
- third-party remote services with unrelated credentials

inside one gateway unless the agent truly needs all of them together.

The safest default is to create smaller, purpose-built gateways.

## Processor Design

Keep processors:

- fast
- deterministic
- narrow in scope
- explicit about whether failure should stop the chain

Practical rules:

- prefer read-only processors before mutation processors
- keep `parts` small so each processor receives only the context it needs
- make mutation processors idempotent where possible
- treat webhook processors as part of the critical path unless marked non-required

If a processor exists mainly for logging or metrics, it should almost always be non-required.

## Logging and Observability

Use Centian's internal logs for Centian runtime behavior:

- startup
- config issues
- processor failures
- downstream transport problems

Use processors for domain-specific observability:

- audit events
- OpenTelemetry export
- payload classification
- redaction

Do not overload processors with basic proxy-health logging that Centian already does.

## Taskverification Usage

Enable taskverification when you want Centian to govern an agent workflow, not just proxy MCP traffic.

Good fits:

- structured implementation tasks
- reviewable multi-step execution
- tasks that benefit from explicit planning and verification
- demos where you want a persisted timeline

Poor fits:

- extremely open-ended chat
- tasks where checks cannot be made stable
- environments where the working directory and shell contract are too variable

Template design guidance:

- keep phases explicit
- keep parameters minimal
- verify concrete outcomes, not vague intentions
- avoid brittle checks that depend on noisy output
- use approval nodes sparingly and only where a real pause is valuable

## Deployment Hygiene

For local stdio servers:

- remember the downstream command runs with the permissions of the Centian process
- keep the Centian working directory intentional
- avoid mixing unrelated local workspaces in one process when taskverification is enabled

For downstream OAuth:

- set `proxy.web.publicBaseUrl` correctly
- keep OAuth credentials out of committed config
- prefer environment-variable injection for secrets

For shared environments:

- keep auth enabled
- store event data intentionally
- review what task and action history is persisted
- do not assume the embedded UI is a write surface; it is read-only

## Config Fields That Are Present but Not Operationally Important Yet

`allowDynamic` and `setupGateway` exist in the config model, but current server startup does not branch on them.

Treat them as reserved fields for now rather than active controls.

## Recommended Rollout Pattern

1. Start with one small gateway and no processors.
2. Add one read-only processor.
3. Add taskverification only after the underlying proxy routes are stable.
4. Add UI and event storage once you actually need persisted run inspection.
5. Split gateways when trust boundaries or tool surfaces start to blur.

## Related Reads

- [Getting Started](getting_started.md)
- [Configuration Reference](configuration_reference.md)
- [Processor Development Guide](processor_development_guide.md)
- [Task Template Authoring](TASK_TEMPLATE_AUTHORING.md)
