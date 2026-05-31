# Centian Demos

## Static IT Ops Demo

```bash
centian demo
```

The default demo creates a local Centian workspace, seeds the bundled IT Ops
incident run into SQLite immediately, starts Centian, and opens `/ui/tasks` for
post-hoc inspection.

This flow is optimized for debugging and reviewing completed agent runs,
especially runs with high-impact governance events such as prompt injection
redaction, blocked high-risk tools, and failed process gates.

## Deprecated Custom Synthetic Replay

The `centian demo --file` flow is deprecated and will likely be moved or
removed in a future release. It remains available temporarily for legacy custom
scenario checks.

```bash
centian demo --file ./demo_scenario.json
```

Custom synthetic scenarios are loaded into the event database immediately.

## Deprecated Agent-Based Demos

The `centian demo --agent` flow is deprecated and will likely be moved or
removed in a future release. It remains available temporarily for legacy
agent-based demos.

```bash
centian demo --agent claude
centian demo --agent gemini
centian demo --agent codex --model gpt-5.4-mini
centian demo --agent codex-ollama --codex-config ~/.codex/config.toml --profile my-local-oss
centian demo --agent claude --path ./my-demo
```

These commands create a local demo workspace, start Centian, configure the
selected agent against the demo MCP endpoint, and execute the bundled prompt.
