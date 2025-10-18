# Centian CLI MCP Proxy Architecture

## Overview

This document outlines the architecture for Centian CLI as a persistent daemon-based MCP (Model Context Protocol) proxy. The system provides observability, security hooks, and context tracking for MCP interactions while maintaining optimal performance through a persistent daemon that handles all MCP traffic.

## Core Requirements

1. **Transparent MCP Proxy**: Drop-in replacement for `npx` and HTTP MCP servers
2. **Request/Response Hooks**: Execute scripts before and after MCP calls for security evaluation and logging
3. **Context Preservation**: Track project path, config source, and metadata for each MCP interaction
4. **Profile Management**: Pre-configured multi-server setups with explicit tool routing
5. **Security Integration**: External evaluation engine integration via hooks
6. **Performance**: Minimal overhead and resource usage for high-frequency MCP operations

## Architecture Overview

### Core Design: Persistent Daemon with MCP Proxying

```
Claude/Agent → centian stdio npx memory-server → Persistent Daemon → MCP Server
                ↓                                      ↓               ↓
           Route to daemon                        Hook(pre) → Log Request → Hook(post)
                ↓                                      ↓                    ↓
        Single process handles                   Security Check      Context Tracking
        all MCP traffic                              ↓                    ↓
                                                Shared logging      Centralized state
```

### Key Principles

- **Persistent Operation**: Single daemon process handles all MCP traffic
- **Zero Startup Delay**: Daemon runs continuously, eliminating cold-start overhead
- **Resource Efficiency**: Shared hooks, logging, and configuration across all MCP servers
- **Transparency**: Drop-in replacement for existing MCP workflows
- **Observability**: Complete visibility into MCP request/response lifecycle
- **Context Awareness**: Preserve project and configuration source information

## System Components

### 1. Persistent Daemon (`internal/daemon/`)

```go
// PersistentDaemon manages all MCP proxy operations
type PersistentDaemon struct {
    // Core functionality
    servers     map[string]*ProxyServer
    hooks       *SharedHookManager
    logger      *SharedLogger
    profiles    *ProfileManager

    // Communication
    unixSocket  net.Listener            // IPC for centian commands
    httpServer  *http.Server            // Optional HTTP API

    // Reliability
    healthCheck *HealthChecker
    autoRestart bool
    recovery    *RecoveryManager

    // Resource management
    maxServers  int
    memoryLimit int64
    cleanup     *CleanupManager

    // State
    startTime   time.Time
    requests    int64                   // Total requests handled
    activeSessions map[string]*Session  // Active MCP sessions
}

type ProxyServer struct {
    ID          string
    Command     []string                // Command to start MCP server
    URL         string                  // For HTTP-based MCP servers
    Process     *exec.Cmd               // Running MCP server process
    Transport   TransportType           // stdio, http, websocket
    Status      ServerStatus
    LastUsed    time.Time

    // Communication channels
    stdin       io.WriteCloser
    stdout      io.Reader
    stderr      io.Reader

    // Clients using this server
    clients     map[string]*ClientSession
}

type ServerStatus int
const (
    StatusStopped ServerStatus = iota
    StatusStarting
    StatusRunning
    StatusError
    StatusRestarting
)
```

**Key Responsibilities:**
- Protocol-aware MCP message parsing and forwarding
- Multi-transport support (stdio, HTTP, WebSocket)
- Server lifecycle management (start, stop, restart, health monitoring)
- Request/response interception and hook execution
- Centralized logging and context tracking

### 2. Hook System (`internal/hooks/`)

```go
// SharedHookManager executes hooks efficiently across all servers
type SharedHookManager struct {
    globalHooks   []Hook
    serverHooks   map[string][]Hook
    profileHooks  map[string][]Hook
    executor      *HookExecutor
    cache         *HookCache          // Cache hook results for performance
}

type Hook struct {
    ID          string              // Unique hook identifier
    Type        HookType            // PreRequest, PostResponse
    Command     string              // Script/binary to execute
    Args        []string            // Additional arguments
    Timeout     time.Duration
    Env         map[string]string   // Environment variables
    Conditions  []HookCondition     // When to execute hook
    Cache       bool                // Whether to cache results
}

type HookType int
const (
    HookPreRequest HookType = iota
    HookPostResponse
    HookOnConnect
    HookOnDisconnect
    HookOnServerStart
    HookOnServerStop
)

type HookCondition struct {
    Field    string              // project_path, server_id, tool_name, etc.
    Operator string              // equals, contains, matches, etc.
    Value    string              // Condition value
}

type HookContext struct {
    RequestID     string              // Unique request identifier
    SessionID     string              // Session identifier
    Timestamp     time.Time
    ProjectPath   string              // Working directory context
    ConfigSource  string              // Configuration origin
    ServerID      string              // Target MCP server identifier
    MCPRequest    json.RawMessage     // Original MCP request
    MCPResponse   json.RawMessage     // MCP response (post-hook only)
    Metadata      map[string]any      // Additional context

    // Performance data
    RequestDuration time.Duration
    HookDuration    time.Duration
}
```

**Hook Execution Flow:**
1. Pre-request hooks receive MCP request + full context
2. Hook can allow, deny, or modify the request
3. Request forwarded to appropriate MCP server
4. Post-response hooks receive request + response + context
5. Hook can log, audit, or trigger external systems
6. Response returned to client

### 3. Context Tracking (`internal/context/`)

```go
// ContextTracker preserves request metadata and relationships
type ContextTracker struct {
    storage     ContextStorage
    sessions    map[string]*Session
    analytics   *Analytics
}

type Session struct {
    SessionID     string              `json:"session_id"`
    StartTime     time.Time           `json:"start_time"`
    ProjectPath   string              `json:"project_path"`
    ConfigSource  string              `json:"config_source"`
    UserID        string              `json:"user_id,omitempty"`
    Requests      []string            `json:"request_ids"`
    ServerUsage   map[string]int      `json:"server_usage"`
    LastActivity  time.Time           `json:"last_activity"`
}

type RequestContext struct {
    RequestID     string              `json:"request_id"`
    SessionID     string              `json:"session_id"`
    Timestamp     time.Time           `json:"timestamp"`
    ProjectPath   string              `json:"project_path"`
    ConfigSource  string              `json:"config_source"`  // global|project|profile
    ServerID      string              `json:"server_id"`

    // MCP Protocol Data
    MCPMethod     string              `json:"mcp_method"`      // call_tool, list_tools, etc.
    ToolName      string              `json:"tool_name,omitempty"`

    // Request/Response
    Request       json.RawMessage     `json:"mcp_request"`
    Response      json.RawMessage     `json:"mcp_response,omitempty"`

    // Timing and Performance
    StartTime     time.Time           `json:"start_time"`
    EndTime       time.Time           `json:"end_time,omitempty"`
    Duration      time.Duration       `json:"duration,omitempty"`
    HookDuration  time.Duration       `json:"hook_duration,omitempty"`

    // Status and Results
    Status        string              `json:"status"`          // success, error, denied
    Error         string              `json:"error,omitempty"`
    HookResults   []HookResult        `json:"hook_results,omitempty"`

    // Security and Compliance
    SecurityLevel string              `json:"security_level,omitempty"`
    PolicyResults []PolicyResult      `json:"policy_results,omitempty"`
}

type Analytics struct {
    TotalRequests     int64
    RequestsByServer  map[string]int64
    RequestsByProject map[string]int64
    AverageLatency    time.Duration
    ErrorRate         float64
    TopTools          map[string]int64
}
```

### 4. Profile Management (`internal/profiles/`)

```go
// ProfileManager handles multi-server configurations
type ProfileManager struct {
    profiles    map[string]*Profile
    active      string                  // Currently active profile
    configPath  string
    remote      *RemoteConfigManager    // For remote profile loading
}

type Profile struct {
    Name        string              `json:"name"`
    Description string              `json:"description"`
    Version     string              `json:"version"`
    Servers     []ServerConfig      `json:"servers"`
    Routing     map[string]string   `json:"routing"`        // tool_name -> server_id
    Hooks       []Hook              `json:"hooks,omitempty"`
    Security    *SecurityConfig     `json:"security,omitempty"`
    Metadata    map[string]any      `json:"metadata,omitempty"`

    // Remote configuration
    RemoteURL   string              `json:"remote_url,omitempty"`
    Checksum    string              `json:"checksum,omitempty"`
    LastSync    time.Time           `json:"last_sync,omitempty"`
}

type ServerConfig struct {
    ID          string              `json:"id"`
    Name        string              `json:"name"`
    Transport   string              `json:"transport"`      // stdio, http, websocket
    Command     []string            `json:"command,omitempty"`
    URL         string              `json:"url,omitempty"`
    Headers     map[string]string   `json:"headers,omitempty"`
    Env         map[string]string   `json:"env,omitempty"`
    Enabled     bool                `json:"enabled"`
    HealthCheck *HealthCheckConfig  `json:"health_check,omitempty"`
    Resources   *ResourceLimits     `json:"resources,omitempty"`
}

type SecurityConfig struct {
    RequireAuth     bool            `json:"require_auth"`
    AllowedPaths    []string        `json:"allowed_paths"`
    DeniedTools     []string        `json:"denied_tools"`
    EvalEngine      *EvalConfig     `json:"eval_engine,omitempty"`
}
```

## Daemon Lifecycle Management

### Startup and Initialization

```bash
# Start persistent daemon (typically in shell startup)
centian daemon start --persistent

# Daemon initialization process:
# 1. Check if already running
# 2. Load configuration
# 3. Set up IPC (Unix socket)
# 4. Initialize hook system
# 5. Start health monitoring
# 6. Begin listening for requests
```

### Process Communication

#### Cross-Platform IPC Strategy

The daemon uses **TCP localhost sockets** for cross-platform compatibility:

- ✅ **Windows, macOS, Linux**: Universal support without platform-specific code
- ✅ **Dynamic Port Allocation**: Avoids port conflicts with automatic assignment
- ✅ **Security**: Localhost-only binding prevents external access
- ✅ **Performance**: Minimal overhead (~0.5ms vs 0.1ms for Unix sockets)
- ✅ **Reliability**: Standard TCP connection handling and error recovery

**Connection Discovery:**
- Daemon writes connection info to `~/.centian/daemon.addr` on startup
- CLI commands read this file to discover daemon endpoint
- Format: `127.0.0.1:8432` (or assigned port)

```go
// Cross-platform IPC implementation
type DaemonIPC struct {
    listener  net.Listener
    transport string      // "tcp" (primary) or "unix" (fallback)
    address   string      // "127.0.0.1:8432" or socket path
}

func NewDaemonIPC() (*DaemonIPC, error) {
    // Primary: TCP localhost with dynamic port allocation
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        return nil, fmt.Errorf("failed to create TCP listener: %w", err)
    }

    port := listener.Addr().(*net.TCPAddr).Port
    address := fmt.Sprintf("127.0.0.1:%d", port)

    // Store connection info for client discovery
    if err := writeConnectionFile(address); err != nil {
        listener.Close()
        return nil, fmt.Errorf("failed to write connection file: %w", err)
    }

    return &DaemonIPC{
        listener:  listener,
        transport: "tcp",
        address:   address,
    }, nil
}

// IPC between centian commands and daemon
type DaemonClient struct {
    conn     net.Conn
    timeout  time.Duration
    address  string
}

func (c *DaemonClient) Connect() error {
    // Read daemon address from connection file
    address, err := readConnectionFile()
    if err != nil {
        return fmt.Errorf("daemon not running or connection file missing: %w", err)
    }

    conn, err := net.DialTimeout("tcp", address, c.timeout)
    if err != nil {
        return fmt.Errorf("failed to connect to daemon at %s: %w", address, err)
    }

    c.conn = conn
    c.address = address
    return nil
}

// Command requests sent to daemon
type DaemonRequest struct {
    Type      string          `json:"type"`       // stdio, http, profile, config
    Command   []string        `json:"command,omitempty"`
    URL       string          `json:"url,omitempty"`
    Headers   map[string]string `json:"headers,omitempty"`
    Profile   string          `json:"profile,omitempty"`
    ProjectPath string        `json:"project_path"`
    ConfigSource string       `json:"config_source"`
}

type DaemonResponse struct {
    Success   bool            `json:"success"`
    ServerID  string          `json:"server_id,omitempty"`
    Error     string          `json:"error,omitempty"`
    Port      int             `json:"port,omitempty"`     // For HTTP connections
}
```

**Configuration:**
```json
{
  "daemon": {
    "ipc": {
      "transport": "tcp",
      "bind": "127.0.0.1:0",
      "timeout": "30s",
      "connectionFile": "~/.centian/daemon.addr"
    }
  }
}
```

### Health Monitoring and Recovery

```go
type HealthChecker struct {
    interval    time.Duration
    timeout     time.Duration
    checks      []HealthCheck
}

type HealthCheck struct {
    Name        string
    Check       func() error
    Critical    bool            // Whether failure should trigger restart
}

// Built-in health checks:
// - IPC socket responsiveness
// - Memory usage monitoring
// - MCP server process health
// - Hook execution timeouts
// - Configuration file integrity
```

## Configuration System

### Configuration Hierarchy
```
~/.centian/
├── config.json              # Global daemon configuration
├── daemon.pid                # Daemon process ID
├── daemon.addr               # TCP connection info (e.g., "127.0.0.1:8432")
├── profiles/                 # Profile definitions
│   ├── web-dev.json
│   ├── data-analysis.json
│   └── team-configs/         # Remote/shared configurations
└── logs/                     # Request logs and context
    ├── daemon.log            # Daemon operational log
    ├── requests.jsonl        # Structured request logs
    ├── hooks.log             # Hook execution log
    └── context.db            # SQLite context database
```

### Global Configuration (`~/.centian/config.json`)
```json
{
  "version": "2.0.0",
  "daemon": {
    "mode": "persistent",
    "autoRestart": true,
    "healthCheck": {
      "enabled": true,
      "interval": "30s",
      "timeout": "10s"
    },
    "ipc": {
      "socket": "~/.centian/daemon.sock",
      "timeout": "30s"
    },
    "resources": {
      "maxServers": 50,
      "memoryLimit": "100MB",
      "maxConcurrentRequests": 1000
    }
  },
  "proxy": {
    "defaultTimeout": 30,
    "logLevel": "info",
    "contextTracking": true,
    "performanceMetrics": true
  },
  "hooks": {
    "global": [
      {
        "id": "security-check",
        "type": "pre-request",
        "command": "./security-check.sh",
        "timeout": 5,
        "cache": true,
        "conditions": [
          {
            "field": "tool_name",
            "operator": "contains",
            "value": "file"
          }
        ]
      }
    ],
    "execution": {
      "parallel": true,
      "maxConcurrency": 10,
      "timeout": "30s"
    }
  },
  "logging": {
    "format": "jsonl",
    "file": "~/.centian/logs/requests.jsonl",
    "contextDB": "~/.centian/logs/context.db",
    "rotation": {
      "maxSize": "100MB",
      "maxAge": "30d",
      "compress": true
    }
  },
  "remote": {
    "enabled": false,
    "configURL": null,
    "profileSync": false,
    "syncInterval": "1h"
  }
}
```

### Project-Specific Configuration (`.centian/config.json`)
```json
{
  "projectID": "my-web-app",
  "servers": {
    "memory": {
      "command": ["npx", "@modelcontextprotocol/server-memory"],
      "hooks": [
        {
          "type": "post-response",
          "command": "./audit-memory-usage.sh"
        }
      ]
    }
  },
  "security": {
    "requireAuth": false,
    "allowedPaths": ["./src", "./docs"],
    "evalEngine": {
      "enabled": true,
      "endpoint": "http://localhost:8081/eval"
    }
  }
}
```

## CLI Interface

### Core Commands

#### Daemon Management
```bash
# Start persistent daemon
centian daemon start --persistent

# Start with auto-restart
centian daemon start --persistent --auto-restart

# Daemon status and health
centian daemon status
centian daemon health
centian daemon logs

# Stop daemon
centian daemon stop
centian daemon restart

# Install as system service (systemd/launchd)
centian daemon install-service --user
centian daemon uninstall-service --user
```

#### Proxy Commands
```bash
# stdio transport (routes through persistent daemon)
# Default: uses npx as command prefix
centian stdio @modelcontextprotocol/server-memory
centian stdio --cmd npx @modelcontextprotocol/server-memory  # explicit npx
centian stdio --cmd python -m my_mcp_server --config config.json

# HTTP transport (routes through persistent daemon)
# Connect to existing HTTP MCP server
centian http https://api.example.com/mcp
centian http https://api.example.com/mcp --auth bearer:token
centian http https://api.example.com/mcp --header "Custom-Header: value"

# Start MCP server using command (HTTP transport)
centian http --cmd python -m my_http_mcp_server --port 8080
centian http --cmd node http-mcp-server.js --config config.json
```

#### Profile Commands
```bash
# Profile management
centian profile create <name>
centian profile list
centian profile show <name>
centian profile activate <name>
centian profile delete <name>

# Server management within profiles
centian profile add-server <profile> <server-id> <command...>
centian profile remove-server <profile> <server-id>
centian profile enable-server <profile> <server-id>
centian profile disable-server <profile> <server-id>

# Tool routing configuration
centian profile route <profile> <tool-name> <server-id>
centian profile unroute <profile> <tool-name>

# Remote profile management
centian profile sync --remote <url>
centian profile pull <profile-id>
centian profile push <profile>

# Connect using profile (multi-server aggregation)
centian connect --profile <name>
centian connect --profile <name> --port 8080
```

#### Hook Management
```bash
# Global hooks
centian hooks add --global --type pre-request --command "./security-check.sh"
centian hooks add --global --type post-response --command "./audit-log.sh"

# Server-specific hooks
centian hooks add --server memory --type pre-request --command "./memory-check.sh"

# Profile-specific hooks
centian hooks add --profile web-dev --type post-response --command "./profile-audit.sh"

# Conditional hooks
centian hooks add --global --type pre-request \
  --command "./file-security.sh" \
  --condition "tool_name contains file"

# List and manage hooks
centian hooks list
centian hooks show <hook-id>
centian hooks remove <hook-id>
centian hooks test <hook-id>    # Test hook execution
```

#### Logging and Analytics
```bash
# View logs
centian logs tail                    # Real-time log streaming
centian logs search --tool search    # Filter by tool name
centian logs search --project ./     # Filter by project
centian logs search --server memory  # Filter by server
centian logs export --format csv     # Export logs

# Context queries
centian context sessions             # List recent sessions
centian context requests --session <id>  # Requests in session
centian context timeline --project ./    # Project request timeline
centian context analytics           # Usage analytics

# Performance monitoring
centian metrics show                 # Current performance metrics
centian metrics export              # Export metrics for analysis
```

#### Configuration Management
```bash
# Configuration
centian config show                 # Show current configuration
centian config init                 # Initialize configuration
centian config validate             # Validate configuration
centian config reload               # Reload daemon configuration

# Remote configuration
centian config sync --remote <url>
centian config pull-profiles
centian config set-remote <url>
```

## Usage Examples

### Basic Setup and Usage

#### Initial Setup
```bash
# Install centian and set up persistent daemon
centian daemon install-service --user

# Or manually add to shell startup
echo 'centian daemon start --persistent --quiet' >> ~/.bashrc
source ~/.bashrc

# Verify daemon is running
centian daemon status
```

#### Basic Proxy Usage
```bash
# Traditional usage
npx @modelcontextprotocol/server-memory

# Centian proxy (routes through persistent daemon)
# Default: automatically prefixes with npx
centian stdio @modelcontextprotocol/server-memory
# -> Instant response (no startup delay)
# -> Automatically logs all MCP requests/responses with context
# -> Executes any configured hooks
# -> Preserves project path and configuration source

# Explicit command specification
centian stdio --cmd npx @modelcontextprotocol/server-memory
centian stdio --cmd python -m my_mcp_server --config config.json
```

#### HTTP Transport
```bash
# HTTP MCP server with authentication
centian http https://api.example.com/mcp --auth bearer:token

# With custom headers
centian http https://api.example.com/mcp \
  --header "X-API-Key: key123" \
  --header "X-Project: my-project"
```

### Advanced Hook Integration

#### Security Evaluation Hook
```bash
# Configure security evaluation hook
centian hooks add --global --type pre-request \
  --command "./security-eval.sh" \
  --condition "tool_name contains file"

# security-eval.sh receives JSON on stdin:
{
  "request_id": "req_123",
  "session_id": "sess_xyz789",
  "project_path": "/workspace/my-project",
  "server_id": "file-server",
  "mcp_request": {
    "method": "call_tool",
    "params": {
      "name": "file_read",
      "arguments": {"path": "/etc/passwd"}
    }
  },
  "metadata": {
    "user_id": "developer@company.com",
    "git_branch": "feature/new-feature"
  }
}

# Hook can return:
# {"action": "allow"}                    - Allow request
# {"action": "deny", "reason": "..."}    - Deny request
# {"action": "modify", "request": {...}} - Modify request
```

#### Performance Monitoring Hook
```bash
# Configure performance monitoring
centian hooks add --global --type post-response \
  --command "./performance-monitor.sh"

# Monitor request latency, error rates, resource usage
# Send metrics to external monitoring systems
```

### Profile-Based Multi-Server Setup

```bash
# Create development profile
centian profile create web-dev

# Add multiple servers
centian profile add-server web-dev memory npx @modelcontextprotocol/server-memory
centian profile add-server web-dev web python -m web_mcp_server
centian profile add-server web-dev db http https://db.example.com/mcp

# Configure tool routing (explicit, no namespacing conflicts)
centian profile route web-dev search memory    # search() -> memory server
centian profile route web-dev fetch web        # fetch() -> web server
centian profile route web-dev query db         # query() -> db server

# Activate profile
centian profile activate web-dev

# Use profile in Claude Desktop config
{
  "mcpServers": {
    "centian-web-dev": {
      "command": "centian",
      "args": ["connect", "--profile", "web-dev"]
    }
  }
}
```

### Context Tracking and Analytics

```bash
# View daemon performance
centian daemon status
# Output:
# Daemon: Running (uptime: 2h 15m)
# Servers: 5 active, 2 idle
# Requests: 1,247 total, 15/min average
# Memory: 45MB used, 100MB limit

# Analyze MCP usage
centian context analytics
# Output:
# Top Tools: search (45%), fetch (23%), store (18%)
# Top Servers: memory-server (67%), web-server (33%)
# Average Latency: 23ms
# Error Rate: 0.1%

# Project-specific analysis
centian logs search --project /workspace/my-app --format json
centian context timeline --project /workspace/my-app
```

## Security Integration

### External Evaluation Engine Integration

The persistent daemon's hook system enables seamless integration with external security evaluation engines:

```bash
# Configure evaluation engine hook
centian hooks add --global --type pre-request \
  --command "./eval-engine-client.sh" \
  --timeout 10

# eval-engine-client.sh communicates with external evaluation service
# Receives full MCP request context
# Returns allow/deny/modify decision
# Daemon caches results for performance
```

### Policy Enforcement Example

```json
{
  "security": {
    "evalEngine": {
      "endpoint": "http://localhost:8081/eval",
      "timeout": "5s",
      "cache": {
        "enabled": true,
        "ttl": "1h"
      },
      "failMode": "deny"  // deny requests if eval engine unavailable
    },
    "policies": [
      {
        "name": "file-access-control",
        "condition": "tool_name contains 'file'",
        "action": "evaluate"
      },
      {
        "name": "network-restrictions",
        "condition": "tool_name in ['fetch', 'http']",
        "action": "evaluate"
      }
    ]
  }
}
```

## Performance Characteristics

### Resource Usage
- **Idle daemon**: ~8-10MB RAM, minimal CPU
- **Active processing**: ~20-50MB RAM depending on concurrent requests
- **Per-server overhead**: ~1-2MB additional RAM per MCP server
- **Hook execution**: Parallel execution with configurable concurrency limits

### Latency Profile
- **Request routing**: <1ms overhead
- **Hook execution**: 5-50ms depending on hook complexity
- **Context logging**: <1ms with async writes
- **Total overhead**: Typically 5-20ms end-to-end

### Scalability
- **Concurrent requests**: 1000+ concurrent MCP requests
- **Server limit**: 50+ MCP servers per daemon
- **Hook throughput**: 100+ hooks/second with caching

## Implementation Plan

### Phase 1: Core Persistent Daemon (MVP)
**Duration: 2-3 weeks**

**Deliverables:**
- Persistent daemon with IPC communication
- Basic MCP protocol proxy for stdio transport
- Request/response logging with context
- Simple configuration system
- Daemon lifecycle management (start/stop/status)

**CLI Commands:**
- `centian daemon start --persistent`
- `centian daemon status`
- `centian stdio <command>`
- `centian logs tail`

**Success Criteria:**
- Daemon runs persistently with <10MB memory usage
- All MCP requests route through daemon with <10ms overhead
- Basic logging and context tracking functional

### Phase 2: Hook System & Multi-Transport
**Duration: 2-3 weeks**

**Deliverables:**
- Complete hook system (pre/post request)
- HTTP transport support
- Hook configuration management
- Enhanced logging and context tracking
- Performance monitoring and metrics

**CLI Commands:**
- `centian http <url>`
- `centian hooks add/list/remove`
- `centian context sessions`
- `centian metrics show`

**Success Criteria:**
- Hook system supports conditional execution
- HTTP transport works with authentication
- Performance metrics and monitoring available

### Phase 3: Profile Management & Multi-Server Aggregation
**Duration: 2-3 weeks**

**Deliverables:**
- Profile creation and management
- Multi-server aggregation mode
- Tool routing configuration
- Profile-based hooks and security
- Health monitoring and auto-recovery

**CLI Commands:**
- `centian profile create/add-server/route`
- `centian connect --profile <name>`
- `centian daemon health`

**Success Criteria:**
- Profiles enable complex multi-server setups
- Tool routing works without namespace conflicts
- Daemon recovers gracefully from failures

### Phase 4: Remote Configuration & Enterprise Features
**Duration: 3-4 weeks**

**Deliverables:**
- Remote configuration loading
- Profile synchronization
- Enhanced security integrations
- Advanced analytics and reporting
- System service integration

**CLI Commands:**
- `centian config sync --remote <url>`
- `centian profile sync --team <org>`
- `centian daemon install-service`

**Success Criteria:**
- Remote configuration enables team collaboration
- Enterprise security features functional
- Production deployment capabilities

## Success Criteria

### Functional Requirements
- ✅ Persistent daemon with minimal resource usage
- ✅ Drop-in replacement for `npx` with zero startup delay
- ✅ Transparent proxy for HTTP MCP servers
- ✅ Complete request/response hook system with caching
- ✅ Context preservation and correlation across calls
- ✅ Profile-based multi-server aggregation
- ✅ External security evaluation integration
- ✅ System service integration for production deployment

### Performance Requirements
- ✅ Daemon memory usage < 50MB under normal load
- ✅ Request routing overhead < 5ms
- ✅ Hook execution with configurable timeouts
- ✅ Support for 1000+ concurrent requests
- ✅ Log rotation and cleanup
- ✅ Graceful handling of server failures

### Developer Experience Requirements
- ✅ One-time setup with persistent operation
- ✅ Familiar command patterns unchanged
- ✅ Clear error messages and debugging
- ✅ Comprehensive logging and observability
- ✅ Rich analytics and performance monitoring

## Risk Mitigation

### Technical Risks
1. **Daemon Reliability**: Comprehensive health monitoring and auto-restart
2. **Resource Leaks**: Proper cleanup and resource limits
3. **Hook Performance**: Parallel execution with caching
4. **Configuration Complexity**: Sensible defaults and validation

### Operational Risks
1. **System Integration**: Support for systemd/launchd services
2. **Multi-User Environments**: Proper isolation and permissions
3. **Container Compatibility**: Environment detection and adaptation
4. **Upgrade Paths**: Configuration migration and compatibility

This persistent daemon architecture provides optimal performance and reliability while maintaining the simplicity and transparency required for a developer-focused MCP proxy tool.