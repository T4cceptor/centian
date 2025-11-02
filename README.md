# Centian CLI

A comprehensive MCP (Model Context Protocol) proxy that provides centralized configuration, lifecycle hooks, and performance optimization for MCP servers.

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

- To get started run `centian init` and allow the initialization wizard
- Alternatively, use `centian stdio` as a drop-in replacement for `npx` (or other MCP server commands) with MCP servers:

```bash
# Basic usage (defaults to npx)
centian stdio -- -y @modelcontextprotocol/server-memory

# With npx
centian stdio --cmd npx -- -y @modelcontextprotocol/server-memory

# Custom command with flags (use -- to separate flags)
centian stdio --cmd python -- -m my_mcp_server --config config.json
```

Note: running a MCP server proxy command (`centian stdio` or `centian http`) starts the centian daemon process, which runs in the background and centralizes MCP connections. Further calls to `centian stdio` will NOT start another daemon, but register the provided arguments at the already running process. See below how to work with the daemon process most effectively.

The CLI provides several additional commands:
- `centian daemon` - Daemon lifecycle management
- `centian config` - Configuration management
- `centian logs` - Shows the latest MCP logs from the centian daemon

## Commands

### Daemon Commands

#### `centian daemon start`
Start the persistent daemon process in the background.

#### `centian daemon stop`
Stop the running daemon process.

#### `centian daemon status`
Show current daemon status and process information.

#### `centian daemon restart`
Restart the daemon process.

### Configuration Commands

#### `centian config init`
Initialize default configuration file.

#### `centian config show`
Display current configuration.

#### `centian config validate`
Validate configuration file syntax.

## Logging

Centian automatically logs all MCP interactions to provide complete audit trails:

**Log Location:** `~/.centian/logs/`

**Log Files:**
- `requests.jsonl` - All MCP requests and responses
- `proxy_operations.jsonl` - Proxy lifecycle events

## Development

- Most frequently used commands are available via Makefile:
```bash

make build # builds the binary at "build/centian"

make install # stop daemon (if running) and installs the binary locally

make test # runs Go tests
```

### Project Structure

```
├── cmd/main.go           # CLI entry point
├── internal/
│   ├── cli/              # CLI command handlers
│   ├── common/           # Shared code
│   ├── daemon/           # Persistent daemon implementation
│   ├── proxy/            # MCP proxy logic
│   ├── logging/          # Request/response logging
│   └── config/           # Configuration management
└── docs/                 # Architecture documentation
```

## Roadmap

- **🔧 Lifecycle Hooks**: Pre/post request processing
- **🌐 HTTP Transport**: Full HTTP MCP server support
- **📋 Profile Management**: Multi-server configurations
- **🛡️ Security Integration**: External evaluation engine
- **📊 Analytics**: Request metrics and performance monitoring

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

Licensed under the Apache License, Version 2.0. See LICENSE file for details.
