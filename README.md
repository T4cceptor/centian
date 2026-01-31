# Centian CLI

A lightweight MCP (Model Context Protocol) proxy that provides logging and lifecycle hooks for all MCP server communications.

## Feature Highlights

  - **🔧 Programmable MCP traffic processing** – inspect, modify, block, or enrich requests and responses with processor scripts.
  - **🧩 Unified gateway for multiple servers** – expose many downstream MCP servers through one clean endpoint (DRY config).
  - **📊 Structured logging & visibility** – capture MCP events for debugging, auditing, and analysis.
  - **🎯 Fast setup via auto‑discovery** – import existing MCP configs from common tools to get started quickly.

## Quick Start

### Installation

#### Via Script (Recommended)

Download and run the installation script:

```bash
curl -fsSL https://raw.githubusercontent.com/CentianAI/centian-cli/main/scripts/install.sh | bash
```

**Custom Installation Directory:**

```bash
# Install to user directory (no sudo required)
curl -fsSL https://raw.githubusercontent.com/CentianAI/centian-cli/main/scripts/install.sh | bash -s -- --install-dir ~/.local/bin

# Or download and run with custom directory
INSTALL_DIR=~/bin bash install.sh
```

The script will:
- Detect your OS and architecture automatically
- Download the latest release from GitHub
- Install to `/usr/local/bin` (or custom directory)
- Verify the installation

#### Homebrew
Coming soon...

#### From Source

Requirements:
- Go - Version `1.25.0`

```bash
git clone https://github.com/CentianAI/centian-cli.git
cd centian-cli
go build -o build/centian ./cmd/main.go
```

### Usage

Centian is intended as a light-weight http-based MCP proxy for both stdio and http MCP servers.

Some brief concepts before we dive into the config:
- Centian uses "gateways" - this is basically just a way to group downstream MCP servers together under a single endpoint (Note: you can still reach each MCP server individually, more on that later)
- MCP server configuration is purposefully made very similar to how other popular MCP clients configure their MCP servers, e.g. Claude Code, Copilot, Cursor, etc.
- For authentication, Centian currently supports light-weight header auth under a centian-specific header ("X-Centian-Auth"), in order to allow forwarding of downstream auth headers from the MCP client (e.g. "Authorization" header)

#### How to proxy MCP server

- Use a config file (create one via `centian init` and follow the process or run `centian config init` to create a skeleton config)
- Configure gateways and downstream MCP servers in `~/.centian/config.json`
  - Example:
  ```json
  {
    // some other config fields
    "gateways": {
      "my-awesome-gateway": {
        "mcpServers": {
          "my-awesome-server": {
            "url": "https://awesome.mcp",
            "headers": {
              "Authorization": "Bearer 123456"
            }
          }
        }
      }
    }
  }
  ```
- Then start the HTTP proxy via `centian server start`
- This brings up a HTTP server on the configured port (default is: 8080) that proxies all MCP requests.
- The endpoints are based on the provided `gateway` and `mcpServer` name using the `mcp` prefix.
  - Note: by default both an aggregated endpoint (using only the gateway name) and individual endpoints for each server (using the respective server name) are provided
  - Example using the config above:
    - `http://localhost/mcp/my-awesome-gateway/my-awesome-server` - the individual endpoint of the server hosted at `https://awesome.mcp`
    - `http://localhost/mcp/my-awesome-gateway` - an aggregated endpoint of ALL downstream MCP servers - note: namespacing is applied here to avoid naming conflicts

## Commands

### `centian stdio`

Proxy MCP server using stdio transport with logging.

**Syntax:**
```bash
centian stdio [--cmd <command>] [-- <args...>]
```

**Options:**
- `--cmd`: Command to execute (default: `npx`)

**Examples:**
```bash
# NPX-based MCP server
centian stdio -- -y @modelcontextprotocol/server-filesystem /path/to/directory

# Python MCP server
centian stdio --cmd python -- -m my_mcp_server

# Node.js MCP server
centian stdio --cmd node -- ./server.js
```

### `centian config`

Configuration management commands.

#### `centian config init`
Initialize default configuration file at `~/.centian/config.json`.

#### `centian config show`
Display current configuration.

#### `centian config validate`
Validate configuration file syntax.

### `centian logs`

Show recent MCP logs from `~/.centian/logs/`.

### `centian server`

Server management commands.

- `centian server start --config-path <path>` - starts the server given the provided config, default path is `~/.centian/config.json`

## Logging

Centian automatically logs all MCP interactions to provide complete audit trails:

**Log Location:** `s~/.centian/logs/`

**Log Files:**
- `requests.jsonl` - All MCP requests with timestamps and session IDs
- `proxy_operations.jsonl` - Proxy lifecycle events (start/stop)


## Development

Most frequently used commands are available via Makefile:

```bash
make build    # Build the binary at "build/centian"
make install  # Install the binary locally at ~/.local/bin/centian
make test-all # Run Go tests (both unit + integration tests)
make dev      # Run full development loop (clean, fmt, vet, test, build)
```

### Project Structure

```
├── cmd/main.go           # CLI entry point
├── internal/
│   ├── cli/              # CLI command handlers
│   ├── common/           # Shared code
│   ├── proxy/            # MCP stdio proxy logic
│   ├── logging/          # Request/response logging
│   └── config/           # Configuration management
└── docs/                 # Architecture documentation
```

## Roadmap

- **🔧 Lifecycle Hooks**: Pre/post request processing for security and transformation - completed (Dec 26, 2025)
- **🌐 HTTP Transport**: Support for HTTP-based MCP servers - in progress
- **Full MCP server discovery**: including both stdio- and http-based MCP servers
- **Gateway Endpoints**: group together multiple MCP servers to be proxied under a single gateway endpoint, simplifying any MCP client setup to just a single endpoint
- **Cross-Transport Support**: Allow cross-transport communication for more compatibility between MCP servers and Centian - support HTTP-proxying locally without running a server, and support stdio-based MCP servers on a remote host of Centian
- **Conditional Processors**: enable processor execution based on different rules for the request/response, from simple include/exclude rules to full regex-based matching of the MCP event
- **OpenTelemetry Integration**: to support a wide range of logging and monitoring solutions
- **WebHook processor**: ability to call external webhooks via POST requests and react to response for complex processing and validation setups

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0. See LICENSE file for details.
