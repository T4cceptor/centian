# Centian CLI

A comprehensive MCP (Model Context Protocol) proxy tool that provides centralized configuration, lifecycle hooks, and performance optimization for MCP servers.

## Features

- **🔄 MCP Proxy**: Drop-in replacement for `npx` with MCP servers
- **⚡ Persistent Daemon**: Background process for improved performance
- **📊 Request Logging**: Complete audit trail of MCP interactions
- **🎯 Context Tracking**: Session and project-aware request handling
- **🔧 Lifecycle Hooks**: Pre/post request processing (future)
- **🌐 Multi-Transport**: Support for stdio and HTTP MCP servers

## Quick Start

### Basic MCP Proxy Usage

Use `centian stdio` as a drop-in replacement for `npx` with MCP servers:

```bash
# Traditional npx approach
echo '{"method":"ping"}' | npx @modelcontextprotocol/server-memory

# Centian approach (same functionality + logging)
echo '{"method":"ping"}' | centian stdio @modelcontextprotocol/server-memory

# Custom command
echo 'test message' | centian stdio --cmd cat
```

### Daemon Mode (Recommended)

Start the persistent daemon for better performance:

```bash
# Start daemon in background
centian daemon start

# Use stdio proxy (automatically uses daemon if running)
echo '{"method":"ping"}' | centian stdio @modelcontextprotocol/server-memory

# Check daemon status
centian daemon status

# Stop daemon
centian daemon stop
```

## Installation

### From Source

```bash
git clone https://github.com/CentianAI/centian-cli.git
cd centian-cli
go build -o build/centian ./cmd/main.go
```

### Usage

The CLI provides several main commands:

- `centian stdio` - MCP server proxy with stdio transport
- `centian daemon` - Daemon lifecycle management
- `centian config` - Configuration management

## Commands

### MCP Proxy Commands

#### `centian stdio`

Proxy MCP servers using stdio transport with comprehensive logging.

```bash
# Basic usage (defaults to npx)
centian stdio @modelcontextprotocol/server-memory

# Custom command
centian stdio --cmd python -m my_mcp_server --config config.json

# Simple commands (no additional args needed)
centian stdio --cmd cat
```

**Options:**
- `--cmd <command>` - Command to execute (default: "npx")

**Behavior:**
- Automatically uses daemon if running, otherwise starts direct proxy
- Logs all requests and responses to `~/.centian/logs/`
- Maintains session context and correlation IDs

### Daemon Commands

#### `centian daemon start`
Start the persistent daemon process in the background.

#### `centian daemon stop`
Stop the running daemon process.

#### `centian daemon status`
Show current daemon status and process information.

#### `centian daemon restart`
Restart the daemon process.

**Benefits of Daemon Mode:**
- Eliminates MCP server startup overhead
- Maintains persistent server connections
- Improved performance for frequent requests
- Centralized resource management

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

**Log Format:**
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "type": "request",
  "request_id": "req_1234567890",
  "session_id": "session_987654321",
  "command": "npx",
  "args": ["@modelcontextprotocol/server-memory"],
  "server_id": "stdio_npx_1234567890",
  "content": "{\"method\":\"ping\"}",
  "success": true
}
```

## Architecture

### Persistent Daemon

The daemon process provides:
- **Cross-platform IPC** via TCP localhost sockets
- **Dynamic port allocation** for daemon discovery
- **Concurrent MCP server management**
- **Automatic health monitoring**
- **Graceful shutdown handling**

### Request Flow

1. **Command Execution**: `centian stdio <args>`
2. **Daemon Check**: Automatically detects if daemon is running
3. **Routing**: Uses daemon if available, otherwise direct execution
4. **Logging**: All requests/responses logged with context
5. **Proxy**: Bidirectional stdio communication with MCP server

## Development

### Building

```bash
go build -o build/centian ./cmd/main.go
```

### Testing

```bash
go test ./...
```

### Project Structure

```
├── cmd/main.go           # CLI entry point
├── internal/
│   ├── cli/              # CLI command handlers
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
