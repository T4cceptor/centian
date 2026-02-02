package proxy

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConnectionStatus represents the state of a downstream connection.
type ConnectionStatus string

// Connection status constants for tracking downstream connection lifecycle.
const (
	StatusPending    ConnectionStatus = "pending"
	StatusConnecting ConnectionStatus = "connecting"
	StatusConnected  ConnectionStatus = "connected"
	StatusFailed     ConnectionStatus = "failed"
)

// DownstreamConnection represents a connection to a downstream MCP server.
type DownstreamConnection struct {
	serverName string
	config     *config.MCPServerConfig
	client     *mcp.Client
	session    *mcp.ClientSession
	tools      []*mcp.Tool
	connected  bool
	mu         sync.RWMutex

	// Progressive connection tracking
	status      ConnectionStatus
	connError   error
	connectedAt time.Time
}

// Connect establishes connection to downstream server
// authHeaders: additional headers from upstream request (for passthrough auth).
func (dc *DownstreamConnection) Connect(ctx context.Context, authHeaders map[string]string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.connected {
		return nil // Already connected
	}

	dc.status = StatusConnecting

	dc.client = mcp.NewClient(&mcp.Implementation{
		Name:    dc.serverName,
		Version: "1.0.0", // TODO: replace with gateway version
	}, nil)

	transport, err := dc.createTransport(authHeaders)
	if err != nil {
		dc.status = StatusFailed
		dc.connError = err
		return fmt.Errorf("failed to create transport: %w", err)
	}

	session, err := dc.client.Connect(ctx, transport, nil)
	if err != nil {
		dc.status = StatusFailed
		dc.connError = err
		return fmt.Errorf("failed to connect: %w", err)
	}
	dc.session = session

	// Discover tools
	if err := dc.discoverTools(ctx); err != nil {
		dc.session.Close() //nolint:errcheck // we are already returning an error
		dc.status = StatusFailed
		dc.connError = err
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	dc.connected = true
	dc.status = StatusConnected
	dc.connectedAt = time.Now()
	return nil
}

// NewDownstreamConnection creates an unconnected downstream wrapper.
func NewDownstreamConnection(name string, cfg *config.MCPServerConfig) *DownstreamConnection {
	return &DownstreamConnection{
		serverName: name,
		config:     cfg,
		connected:  false,
		status:     StatusPending,
	}
}

// HeaderRoundTripper is used to store.
type HeaderRoundTripper struct {
	Base    http.RoundTripper
	Headers map[string]string
}

// RoundTrip adds the header from HeaderRoundTripper to the request.
// This is done so both configured headers as well as headers from the client
// are included in the request.
func (rt HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}
	// Clone to avoid mutating the original request.
	cloned := req.Clone(req.Context())
	for k, v := range rt.Headers {
		// Use Set to overwrite, or Add to append.
		cloned.Header.Set(k, v)
	}
	// TODO: raw request processing
	resp, err := base.RoundTrip(cloned)
	// TODO: raw response processing
	return resp, err
}

func (dc *DownstreamConnection) createTransport(authHeaders map[string]string) (mcp.Transport, error) {
	isHTTPtransport := dc.config.URL != ""
	isStdioTransport := dc.config.Command != ""
	if isHTTPtransport && isStdioTransport {
		return nil, fmt.Errorf("both URL or Command configured for server %s", dc.serverName)
	}

	if isHTTPtransport {
		// Merge config headers with passed auth headers
		allHeaders := make(map[string]string)
		for k, v := range dc.config.GetSubstitutedHeaders() {
			allHeaders[k] = v
		}
		for k, v := range authHeaders {
			allHeaders[k] = v // Auth headers override config
		}

		// HTTP transport
		httpClient := &http.Client{
			Transport: HeaderRoundTripper{
				Headers: allHeaders,
			},
			Timeout: 30 * time.Second,
		}

		// This requires a custom RoundTripper
		transport := &mcp.StreamableClientTransport{
			Endpoint:   dc.config.URL,
			HTTPClient: httpClient,
		}
		return transport, nil
	}

	if isStdioTransport {
		// Stdio transport
		//nolint:gosec // dc.config.Command comes from user config
		// this is the responsibility of the user setting up the config.
		cmd := exec.Command(dc.config.Command, dc.config.Args...)
		// Add environment variables
		for k, v := range dc.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	}

	return nil, fmt.Errorf("no URL or Command configured for server %s", dc.serverName)
}

func (dc *DownstreamConnection) discoverTools(ctx context.Context) error {
	result, err := dc.session.ListTools(ctx, nil)
	if err != nil {
		return err
	}

	// TODO: logging & processing
	// TODO: tool aggregation/federation

	dc.tools = result.Tools
	return nil
}

// CallTool forwards a tool call to the downstream server.
func (dc *DownstreamConnection) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.connected || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}

	result, err := dc.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	return result, err
}

// Close terminates the downstream connection.
func (dc *DownstreamConnection) Close() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.session != nil {
		if err := dc.session.Close(); err != nil {
			return err
		}
	}
	dc.connected = false
	return nil
}

// Tools returns the cached tools (nil if not connected).
func (dc *DownstreamConnection) Tools() []*mcp.Tool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.tools
}

// IsConnected returns true if connection was established and not yet closed.
func (dc *DownstreamConnection) IsConnected() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.connected
}

// Status returns the current connection status (thread-safe).
func (dc *DownstreamConnection) Status() ConnectionStatus {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status
}

// Error returns the connection error if status is Failed (thread-safe).
func (dc *DownstreamConnection) Error() error {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.connError
}

// ConnectedAt returns when connection was established (zero if not connected).
func (dc *DownstreamConnection) ConnectedAt() time.Time {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.connectedAt
}

// ServerName returns the name of this downstream server.
func (dc *DownstreamConnection) ServerName() string {
	return dc.serverName
}
