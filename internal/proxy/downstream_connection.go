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
	StatusPending      ConnectionStatus = "pending"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusFailed       ConnectionStatus = "failed"
	StatusDisconnected ConnectionStatus = "disconnected"
)

// DownstreamConnection represents a connection to a downstream MCP server.
type DownstreamConnection struct {
	serverName string
	config     *config.MCPServerConfig
	client     *mcp.Client
	session    *mcp.ClientSession
	tools      []*mcp.Tool
	mu         sync.RWMutex

	clientState DownstreamClientState

	// Progressive connection tracking
	status      ConnectionStatus
	connError   error
	connectedAt time.Time
}

// GetServerName returns the server name for this DownstreamConnection.
func (dc *DownstreamConnection) GetServerName() string {
	return dc.serverName
}

// Connect establishes connection to downstream server.
func (dc *DownstreamConnection) Connect(ctx context.Context, options DownstreamConnectOptions) error {
	if dc.IsConnected() {
		return nil
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.status = StatusConnecting
	dc.clientState = options.ClientState
	dc.client = mcp.NewClient(&mcp.Implementation{
		Name:    dc.serverName,
		Version: "1.0.0",
	}, dc.buildClientOptions(options))

	if len(dc.clientState.Roots) > 0 {
		dc.client.AddRoots(dc.clientState.Roots...)
	}

	transport, err := dc.createTransport(options.ForwardedHeaders)
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

	if err := dc.discoverTools(ctx); err != nil {
		dc.session.Close() //nolint:errcheck // already returning an error
		dc.status = StatusFailed
		dc.connError = err
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	dc.status = StatusConnected
	dc.connError = nil
	dc.connectedAt = time.Now()
	return nil
}

func (dc *DownstreamConnection) buildClientOptions(options DownstreamConnectOptions) *mcp.ClientOptions {
	clientOptions := &mcp.ClientOptions{
		Capabilities: normalizeClientCapabilities(options.ClientState.ClientCapabilities),
	}
	if options.SamplingHandler != nil {
		clientOptions.CreateMessageHandler = options.SamplingHandler
	}
	if options.ElicitationHandler != nil {
		clientOptions.ElicitationHandler = options.ElicitationHandler
	}
	return clientOptions
}

// SyncClientState updates mutable downstream client state without reconnecting.
func (dc *DownstreamConnection) SyncClientState(ctx context.Context, clientState DownstreamClientState) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	currentRoots := normalizeRoots(dc.clientState.Roots)
	if dc.client == nil {
		dc.clientState = clientState
		return nil
	}

	currentByURI := make(map[string]*mcp.Root, len(currentRoots))
	for _, root := range currentRoots {
		if root == nil {
			continue
		}
		currentByURI[root.URI] = root
	}

	nextRoots := normalizeRoots(clientState.Roots)
	nextByURI := make(map[string]*mcp.Root, len(nextRoots))
	for _, root := range nextRoots {
		if root == nil {
			continue
		}
		nextByURI[root.URI] = root
	}

	removeURIs := make([]string, 0)
	for uri := range currentByURI {
		if _, ok := nextByURI[uri]; !ok {
			removeURIs = append(removeURIs, uri)
		}
	}
	if len(removeURIs) > 0 {
		dc.client.RemoveRoots(removeURIs...)
	}

	addRoots := make([]*mcp.Root, 0)
	for uri, root := range nextByURI {
		if existing, ok := currentByURI[uri]; ok && existing.Name == root.Name {
			continue
		}
		addRoots = append(addRoots, root)
	}
	if len(addRoots) > 0 {
		dc.client.AddRoots(addRoots...)
	}

	dc.clientState = clientState
	dc.clientState.Roots = nextRoots
	dc.clientState.RootsFingerprint = fingerprintRoots(nextRoots)
	dc.clientState.CapabilitiesFingerprint = fingerprintClientCapabilities(dc.clientState.ClientCapabilities)

	if dc.session != nil {
		if err := dc.discoverTools(ctx); err != nil {
			return err
		}
	}
	return nil
}

// NewDownstreamConnection creates an unconnected downstream wrapper.
func NewDownstreamConnection(serverName string, cfg *config.MCPServerConfig) *DownstreamConnection {
	return &DownstreamConnection{
		serverName: serverName,
		config:     cfg,
		status:     StatusPending,
	}
}

// HeaderRoundTripper injects configured headers into outgoing downstream requests.
type HeaderRoundTripper struct {
	Base    http.RoundTripper
	Headers map[string]string
}

// RoundTrip adds the configured headers to the request.
func (rt HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}

	cloned := req.Clone(req.Context())
	for key, value := range rt.Headers {
		cloned.Header.Set(key, value)
	}
	return base.RoundTrip(cloned)
}

func (dc *DownstreamConnection) createTransport(forwardedHeaders map[string]string) (mcp.Transport, error) {
	isHTTPTransport := dc.config.URL != ""
	isStdioTransport := dc.config.Command != ""
	if isHTTPTransport && isStdioTransport {
		return nil, fmt.Errorf("both URL or Command configured for server %s", dc.serverName)
	}

	if isHTTPTransport {
		allHeaders := make(map[string]string)
		for key, value := range dc.config.GetSubstitutedHeaders() {
			allHeaders[key] = value
		}
		for key, value := range forwardedHeaders {
			allHeaders[key] = value
		}

		httpClient := &http.Client{
			Transport: HeaderRoundTripper{Headers: allHeaders},
			Timeout:   30 * time.Second,
		}

		return &mcp.StreamableClientTransport{
			Endpoint:   dc.config.URL,
			HTTPClient: httpClient,
		}, nil
	}

	if isStdioTransport {
		//nolint:gosec // command comes from trusted user config
		cmd := exec.Command(dc.config.Command, dc.config.Args...)
		for key, value := range dc.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
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
	dc.tools = result.Tools
	return nil
}

// CallTool forwards a tool call to the downstream server.
func (dc *DownstreamConnection) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if dc.status != StatusConnected || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}

	return dc.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
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
	dc.status = StatusDisconnected
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
	return dc.status == StatusConnected
}

// GetConfig returns the server configuration for this connection.
func (dc *DownstreamConnection) GetConfig() *config.MCPServerConfig {
	return dc.config
}

// GetStatus returns the current connection status (thread-safe).
func (dc *DownstreamConnection) GetStatus() ConnectionStatus {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status
}

// GetError returns the connection error if status is Failed (thread-safe).
func (dc *DownstreamConnection) GetError() error {
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
