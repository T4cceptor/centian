package proxy

// This file implements live downstream MCP client connections and applies the
// mirrored upstream client state to them.

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

// IsPending returns true if ConnectionStatus is Pending.
func (s ConnectionStatus) IsPending() bool {
	return s == StatusPending
}

// IsConnecting returns true if ConnectionStatus is Conecting.
func (s ConnectionStatus) IsConnecting() bool {
	return s == StatusConnecting
}

// IsConnected returns true if ConnectionStatus is Connected.
func (s ConnectionStatus) IsConnected() bool {
	return s == StatusConnected
}

// IsFailed returns true if ConnectionStatus is Failed.
func (s ConnectionStatus) IsFailed() bool {
	return s == StatusFailed
}

// IsDisconnected returns true if ConnectionStatus is Disconnected.
func (s ConnectionStatus) IsDisconnected() bool {
	return s == StatusDisconnected
}

// DownstreamConnection represents a connection to a downstream MCP server.
type DownstreamConnection struct {
	serverName        string
	config            *config.MCPServerConfig
	client            *mcp.Client
	session           *mcp.ClientSession
	tools             []*mcp.Tool
	resources         []*mcp.Resource
	resourceTemplates []*mcp.ResourceTemplate
	prompts           []*mcp.Prompt
	mu                sync.RWMutex

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
func (dc *DownstreamConnection) Connect(ctx context.Context, options *DownstreamConnectOptions) error {
	if dc.IsConnected() {
		return nil
	}
	if options == nil {
		options = &DownstreamConnectOptions{}
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

func (dc *DownstreamConnection) buildClientOptions(options *DownstreamConnectOptions) *mcp.ClientOptions {
	clientOptions := &mcp.ClientOptions{
		Capabilities: normalizeClientCapabilities(options.ClientState.ClientCapabilities),
	}
	if options.SamplingHandler != nil {
		clientOptions.CreateMessageHandler = options.SamplingHandler
	}
	if options.ElicitationHandler != nil {
		clientOptions.ElicitationHandler = options.ElicitationHandler
	}
	if options.LoggingHandler != nil {
		clientOptions.LoggingMessageHandler = options.LoggingHandler
	}
	if options.ResourceListChangedHandler != nil {
		clientOptions.ResourceListChangedHandler = options.ResourceListChangedHandler
	}
	if options.ResourceUpdatedHandler != nil {
		clientOptions.ResourceUpdatedHandler = options.ResourceUpdatedHandler
	}
	return clientOptions
}

// SyncClientState updates mutable downstream client state without reconnecting.
func (dc *DownstreamConnection) SyncClientState(ctx context.Context, clientState *DownstreamClientState) error {
	if clientState == nil {
		clientState = &DownstreamClientState{}
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	currentRoots := normalizeRoots(dc.clientState.Roots)
	if dc.client == nil {
		dc.clientState = *clientState
		return nil
	}

	nextRoots := normalizeRoots(clientState.Roots)
	currentByURI := rootsByURI(currentRoots)
	nextByURI := rootsByURI(nextRoots)

	removeURIs := removedRootURIs(currentByURI, nextByURI)
	if len(removeURIs) > 0 {
		dc.client.RemoveRoots(removeURIs...)
	}

	addRoots := addedOrUpdatedRoots(currentByURI, nextByURI)
	if len(addRoots) > 0 {
		dc.client.AddRoots(addRoots...)
	}

	dc.clientState = *clientState
	dc.clientState.Roots = nextRoots
	dc.clientState.RootsFingerprint = fingerprintJSON(nextRoots)
	dc.clientState.CapabilitiesFingerprint = fingerprintJSON(dc.clientState.ClientCapabilities)

	if dc.session != nil {
		if err := dc.discoverTools(ctx); err != nil {
			return err
		}
	}
	return nil
}

func rootsByURI(roots []*mcp.Root) map[string]*mcp.Root {
	byURI := make(map[string]*mcp.Root, len(roots))
	for _, root := range roots {
		if root == nil {
			continue
		}
		byURI[root.URI] = root
	}
	return byURI
}

func removedRootURIs(currentByURI, nextByURI map[string]*mcp.Root) []string {
	removeURIs := make([]string, 0)
	for uri := range currentByURI {
		if _, ok := nextByURI[uri]; !ok {
			removeURIs = append(removeURIs, uri)
		}
	}
	return removeURIs
}

func addedOrUpdatedRoots(currentByURI, nextByURI map[string]*mcp.Root) []*mcp.Root {
	addRoots := make([]*mcp.Root, 0)
	for uri, root := range nextByURI {
		if existing, ok := currentByURI[uri]; ok && existing.Name == root.Name {
			continue
		}
		addRoots = append(addRoots, root)
	}
	return addRoots
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

// discoverResources and discoverPrompts are the internal (lock-unsafe) variants of
// DiscoverResources and DiscoverPrompts. They must only be called by code that already
// holds dc.mu for writing (e.g. during Connect or SyncClientState).
// The exported DiscoverResources/DiscoverPrompts are the pool-facing public API that
// acquire the lock themselves and are safe to call concurrently from outside this struct.
func (dc *DownstreamConnection) discoverResources(ctx context.Context) error {
	result, err := dc.session.ListResources(ctx, nil)
	if err != nil {
		return err
	}
	dc.resources = result.Resources
	return nil
}

func (dc *DownstreamConnection) discoverResourceTemplates(ctx context.Context) error {
	result, err := dc.session.ListResourceTemplates(ctx, nil)
	if err != nil {
		return err
	}
	dc.resourceTemplates = result.ResourceTemplates
	return nil
}

func (dc *DownstreamConnection) discoverPrompts(ctx context.Context) error {
	result, err := dc.session.ListPrompts(ctx, nil)
	if err != nil {
		return err
	}
	dc.prompts = result.Prompts
	return nil
}

// DiscoverResources fetches and caches the resource list from the downstream server.
// Safe to call after Connect returns; acquires the write lock for the duration of the RPC.
func (dc *DownstreamConnection) DiscoverResources(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.session == nil {
		return nil
	}
	return dc.discoverResources(ctx)
}

// DiscoverResourceTemplates fetches and caches the resource template list from the downstream server.
// Safe to call after Connect returns; acquires the write lock for the duration of the RPC.
func (dc *DownstreamConnection) DiscoverResourceTemplates(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.session == nil {
		return nil
	}
	return dc.discoverResourceTemplates(ctx)
}

// DiscoverPrompts fetches and caches the prompt list from the downstream server.
// Safe to call after Connect returns; acquires the write lock for the duration of the RPC.
func (dc *DownstreamConnection) DiscoverPrompts(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.session == nil {
		return nil
	}
	return dc.discoverPrompts(ctx)
}

// Resources returns the cached resources discovered on connect (nil if not connected or unsupported).
func (dc *DownstreamConnection) Resources() []*mcp.Resource {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.resources
}

// ResourceTemplates returns the cached resource templates discovered on connect (nil if not connected or unsupported).
func (dc *DownstreamConnection) ResourceTemplates() []*mcp.ResourceTemplate {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.resourceTemplates
}

// Prompts returns the cached prompts discovered on connect (nil if not connected or unsupported).
func (dc *DownstreamConnection) Prompts() []*mcp.Prompt {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.prompts
}

// ReadResource reads a resource by URI from the downstream server.
func (dc *DownstreamConnection) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
}

// GetPrompt retrieves a prompt by name from the downstream server.
func (dc *DownstreamConnection) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: args})
}

// Complete forwards a completion request to the downstream server.
func (dc *DownstreamConnection) Complete(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.Complete(ctx, req.Params)
}

// Subscribe requests resource update notifications for the given URI from the downstream server.
func (dc *DownstreamConnection) Subscribe(ctx context.Context, uri string) error {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri})
}

// Unsubscribe cancels resource update notifications for the given URI from the downstream server.
func (dc *DownstreamConnection) Unsubscribe(ctx context.Context, uri string) error {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri})
}

// SetLoggingLevel forwards a logging level request to the downstream server.
func (dc *DownstreamConnection) SetLoggingLevel(ctx context.Context, params *mcp.SetLoggingLevelParams) error {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
		return fmt.Errorf("not connected to %s", dc.serverName)
	}
	return dc.session.SetLoggingLevel(ctx, params)
}

// CallTool forwards a tool call to the downstream server.
func (dc *DownstreamConnection) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.status.IsConnected() || dc.session == nil {
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
	return dc.status.IsConnected()
}

// IsConnecting returns true if connection is being established and not yet closed.
func (dc *DownstreamConnection) IsConnecting() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status.IsConnecting()
}

// IsDisconnected returns true if connection is closed.
func (dc *DownstreamConnection) IsDisconnected() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status.IsDisconnected()
}

// IsFailed returns true if connection could not be established.
func (dc *DownstreamConnection) IsFailed() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status.IsFailed()
}

// IsPending returns true if connection is currently pending.
func (dc *DownstreamConnection) IsPending() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status.IsPending()
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
