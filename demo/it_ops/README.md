# IT Ops Demo

This demo runs a small IT ops MCP server through Centian with the prompt injection guard processor in `annotate` mode.

Docker Compose only starts Postgres. Centian and the Node MCP server run locally so the processor binary and config are easy to edit.

## Structure

- `configs/it_ops_config.json`: local Centian config for the `it-ops` gateway.
- `server/it_ops_server.mjs`: dependency-free stdio MCP mock server.
- `prompt_injection_guard/`: compiled Go prompt injection guard processor.
- `docker-compose.yml`: local Postgres only.
- `sql/init/`: minimal seed data for the local Postgres container.

## Run

From the repository root:

```bash
cd demo/it_ops
make postgres-up
make run-centian
```

The gateway is available at:

```text
http://127.0.0.1:8577/mcp/it-ops
```

Add it to an MCP client with:

```json
{
  "mcpServers": {
    "centian-it-ops": {
      "url": "http://127.0.0.1:8577/mcp/it-ops"
    }
  }
}
```

Tools are namespaced by Centian. The mock server provides:

- `it-ops-mock___get_docs`
- `it-ops-mock___get_ticket`
- `it-ops-mock___get_logs`
- `it-ops-mock___kubectl`

Stop Postgres with:

```bash
make postgres-down
```

## Synthetic Timeline

The synthetic ops incident timeline is available at `demo/it_ops/synthetic_demo.json`.

Run it with:

```bash
go run . demo --file demo/it_ops/synthetic_demo.json
```

It demonstrates four beats:

- Prompt injection stripped from a Splunk result before delivery to the agent.
- A restart blocked by the root-cause-analysis tool allowlist.
- A resolution start blocked by the `rca_documented` precondition.
- Final completion only after `service_healthy` and `latency_within_target` pass.
