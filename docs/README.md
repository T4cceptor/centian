# Centian Documentation

This directory is the canonical documentation surface for Centian. The top-level `README.md` stays intentionally short; detailed behavior, configuration, and workflow guidance belongs here.

## Start Here

- [Getting Started](getting_started.md) for first-run setup, proxy startup, and MCP client hookup.
- [Configuration Reference](configuration_reference.md) for the full config surface and validation rules.
- [HTTP Proxy Setup](HTTP_PROXY_SETUP.md) for transport and deployment-oriented setup guidance.
- [PostgreSQL Backend](postgres.md) for using Postgres (instead of SQLite) for event storage and auth, including a local Docker setup.

## Core Guides

- [Processor Development Guide](processor_development_guide.md) for CLI and webhook processors.
- [Task Template Authoring](TASK_TEMPLATE_AUTHORING.md) for authoring workflow YAML.
- [Taskverification Runtime](TASKVERIFICATION.md) for the runtime model and task tool lifecycle.
- [Benchmarking](BENCHMARKING.md) for benchmark suites, preserved artifacts, scorecards, and benchmark UI/API behavior.
- [MCP Proxy Best Practices](mcp_proxy_best_practices.md) for deployment, gateway design, auth, processors, and taskverification guidance.
