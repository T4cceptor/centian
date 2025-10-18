# Centian CLI MCP Proxy Implementation Details

## System Components Implementation

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

## Process Communication

```go
// IPC between centian commands and daemon
type DaemonClient struct {
    socket   net.Conn
    timeout  time.Duration
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

## Health Monitoring and Recovery

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

## Configuration Structures

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

## Hook Integration Examples

### Security Evaluation Hook
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

## Policy Enforcement Example

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

## Implementation Phases

### Phase 1: Core Persistent Daemon (MVP)
**Duration: 2-3 weeks**

**Implementation Focus:**
- Persistent daemon with IPC communication
- Basic MCP protocol proxy for stdio transport
- Request/response logging with context
- Simple configuration system
- Daemon lifecycle management (start/stop/status)

**Key Files to Implement:**
- `internal/daemon/daemon.go` - Core daemon process
- `internal/daemon/ipc.go` - Unix socket communication
- `internal/proxy/stdio.go` - stdio transport proxy
- `internal/config/config.go` - Configuration management
- `internal/context/tracker.go` - Basic context tracking

### Phase 2: Hook System & Multi-Transport
**Duration: 2-3 weeks**

**Implementation Focus:**
- Complete hook system (pre/post request)
- HTTP transport support
- Hook configuration management
- Enhanced logging and context tracking
- Performance monitoring and metrics

**Key Files to Implement:**
- `internal/hooks/manager.go` - Hook management system
- `internal/hooks/executor.go` - Hook execution engine
- `internal/proxy/http.go` - HTTP transport proxy
- `internal/metrics/collector.go` - Performance metrics

### Phase 3: Profile Management & Multi-Server Aggregation
**Duration: 2-3 weeks**

**Implementation Focus:**
- Profile creation and management
- Multi-server aggregation mode
- Tool routing configuration
- Profile-based hooks and security
- Health monitoring and auto-recovery

**Key Files to Implement:**
- `internal/profiles/manager.go` - Profile management
- `internal/proxy/aggregator.go` - Multi-server aggregation
- `internal/health/checker.go` - Health monitoring
- `internal/routing/router.go` - Tool routing logic

### Phase 4: Remote Configuration & Enterprise Features
**Duration: 3-4 weeks**

**Implementation Focus:**
- Remote configuration loading
- Profile synchronization
- Enhanced security integrations
- Advanced analytics and reporting
- System service integration

**Key Files to Implement:**
- `internal/remote/config.go` - Remote configuration
- `internal/security/evaluator.go` - Security evaluation
- `internal/analytics/reporter.go` - Analytics and reporting
- `scripts/install-service.sh` - System service installation