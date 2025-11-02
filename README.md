# Centian CLI

A lightweight MCP (Model Context Protocol) proxy that provides logging and lifecycle hooks for all MCP server communications.

## Features

- **📊 Request Logging**: Complete monitoring of MCP interactions
- **🔧 Lifecycle Hooks**: Pre/post request processing
- **🎯 Context Tracking**: Session and project-aware request handling

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

```bash
git clone https://github.com/CentianAI/centian-cli.git
cd centian-cli
go build -o build/centian ./cmd/main.go
```

### Usage

Use `centian stdio` as a drop-in replacement for `npx` (or other MCP server commands):

```bash
# Basic usage (defaults to npx)
centian stdio -- -y @modelcontextprotocol/server-memory

# With explicit npx
centian stdio --cmd npx -- -y @modelcontextprotocol/server-memory

# Custom command with flags (use -- to separate flags)
centian stdio --cmd python -- -m my_mcp_server --config config.json

# Direct node command
centian stdio --cmd node -- /path/to/mcp-server.js
```

### Configuration in MCP Clients

Replace your MCP server commands in your MCP client configuration:

**Before:**
```json
{
  "mcpServers": {
    "memory": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"]
    }
  }
}
```

**After:**
```json
{
  "mcpServers": {
    "memory": {
      "command": "centian",
      "args": ["stdio", "--", "-y", "@modelcontextprotocol/server-memory"]
    }
  }
}
```

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
Initialize default configuration file at `~/.centian/config.jsonc`.

#### `centian config show`
Display current configuration.

#### `centian config validate`
Validate configuration file syntax.

### `centian logs`

Show recent MCP logs from `~/.centian/logs/`.

## Logging

Centian automatically logs all MCP interactions to provide complete audit trails:

**Log Location:** `~/.centian/logs/`

**Log Files:**
- `requests.jsonl` - All MCP requests with timestamps and session IDs
- `proxy_operations.jsonl` - Proxy lifecycle events (start/stop)


## Development

Most frequently used commands are available via Makefile:

```bash
make build   # Build the binary at "build/centian"
make install # Install the binary locally at ~/.local/bin/centian
make test    # Run Go tests
make dev     # Run full development loop (clean, fmt, vet, test, build)
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

- **🌐 HTTP Transport**: Support for HTTP-based MCP servers
- **🔧 Lifecycle Hooks**: Pre/post request processing for security and transformation
- **📋 Profile Management**: Multi-server configuration profiles
- **🛡️ Security Integration**: External security evaluation engine
- **📊 Analytics Dashboard**: Web UI for metrics and performance monitoring

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0. See LICENSE file for details.