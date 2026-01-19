package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/*
CentianProxy is the main server struct.

It holds 4 critical components:
- mux - used to register URL paths
- server - used to serve the mux
- logger - main logger for all events in the proxied endpoints
- gateways - holds all gateways and proxy endpoints for easy access

Additionally it has a reference to the global config which was loaded to
initialize this server.
*/
type CentianProxy struct {
	Name     string
	ServerID string // used to uniquely identify this specific object instance
	Config   *config.GlobalConfig
	Mux      *http.ServeMux
	Server   *http.Server
	Logger   *logging.Logger // Shared base logger (ONE file handle)
	Gateways map[string]*MCPProxy
}

// NewCentianProxy takes a GlobalConfig struct and returns a new CentianProxy
func NewCentianProxy(globalConfig *config.GlobalConfig) (*CentianProxy, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         ":" + globalConfig.Proxy.Port,
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
		WriteTimeout: common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
	}
	logger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	return &CentianProxy{
		Config:   globalConfig,
		Mux:      mux,
		Server:   server,
		Logger:   logger,
		ServerID: getServerID(globalConfig.Name),
		Gateways: make(map[string]*MCPProxy),
	}, nil
}

// ============================================================================
// MCPProxy - Unified proxy for both aggregated and single-server modes
// ============================================================================

// CentianProxySession represents a session with one or more downstream connections.
// For single-server mode, the map has exactly one entry.
// For aggregated mode, the map has multiple entries.
type CentianProxySession struct {
	id              string
	initialized     bool
	downstreamConns map[string]*DownstreamConnection // serverName → connection
	authHeaders     map[string]string
}

// MCPProxy is a unified proxy that handles both aggregated (multiple servers
// with namespaced tools) and single-server (pass-through) modes.
//
// Mode is controlled by the namespaceTools flag:
//   - true:  Aggregated mode - tools are prefixed with "serverName__"
//   - false: Single mode - tools pass through with original names
type MCPProxy struct {
	name     string
	endpoint string

	// Downstream connection templates (created on init, cloned per-session)
	downstreams map[string]*DownstreamConnection

	// Session management: sessionID → *ProxySession
	sessions map[string]*CentianProxySession

	// Mode configuration: determines if the proxy is for a single downstream connection or multiple
	//
	// - true = multiple, aggregated MCP servers
	//
	// - false = pass-through for a single MCP server
	isAggregatedProxy bool

	// Tool registry for aggregated mode: namespacedTool → serverName
	toolRegistry map[string]string

	// Back-reference to parent server (for old CentianProxyEndpoint compatibility)
	server *CentianProxy
	config *config.GatewayConfig

	mu sync.RWMutex
}

// NewAggregatedProxy creates a proxy that aggregates multiple downstream servers.
// Tools from each server are namespaced as "serverName__toolName" to avoid collisions.
func NewAggregatedProxy(name string, endpoint string, gatewayConfig *config.GatewayConfig) *MCPProxy {
	proxy := &MCPProxy{
		name:              name,
		endpoint:          endpoint,
		config:            gatewayConfig,
		downstreams:       make(map[string]*DownstreamConnection),
		sessions:          make(map[string]*CentianProxySession),
		isAggregatedProxy: true,
		toolRegistry:      make(map[string]string),
	}

	// Pre-create downstream templates from config
	for serverName, serverCfg := range gatewayConfig.MCPServers {
		if serverCfg.Enabled {
			proxy.downstreams[serverName] = NewDownstreamConnection(serverName, serverCfg)
		}
	}

	return proxy
}

// NewSingleProxy creates a proxy for a single downstream server.
// Tools pass through with their original names (no namespacing).
func NewSingleProxy(serverName string, endpoint string, serverConfig *config.MCPServerConfig) *MCPProxy {
	return &MCPProxy{
		name:     serverName,
		endpoint: endpoint,
		downstreams: map[string]*DownstreamConnection{
			serverName: NewDownstreamConnection(serverName, serverConfig),
		},
		sessions:          make(map[string]*CentianProxySession),
		isAggregatedProxy: false,
	}
}

// GetServerForRequest returns (or creates) an MCP server for the given HTTP request's session.
func (p *MCPProxy) GetServerForRequest(r *http.Request) *mcp.Server {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = getNewUUIDV7()
	}
	common.LogInfo("MCPProxy[%s]: Getting server for session %s", p.name, sessionID)

	p.mu.Lock()
	defer p.mu.Unlock()

	session, exists := p.sessions[sessionID]
	if !exists {
		session = p.createSession(sessionID, r)
		p.sessions[sessionID] = session
	}

	return p.createServerForSession(session)
}

func (p *MCPProxy) createSession(id string, r *http.Request) *CentianProxySession {
	authHeaders := make(map[string]string)
	// Capture auth headers from upstream request for passthrough
	// TODO: make these headers configurable
	for _, h := range []string{"Authorization", "X-API-Key", "X-Auth-Token"} {
		if v := r.Header.Get(h); v != "" {
			authHeaders[h] = v
		}
	}
	return &CentianProxySession{
		id:              id,
		initialized:     false,
		downstreamConns: make(map[string]*DownstreamConnection),
		authHeaders:     authHeaders,
	}
}

func (p *MCPProxy) createServerForSession(session *CentianProxySession) *mcp.Server {
	var server *mcp.Server
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}

	server = mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		InitializedHandler: func(ctx context.Context, _ *mcp.InitializedRequest) {
			p.handleInitialize(ctx, server, session)
		},
	})
	return server
}

// handleInitialize connects to downstream server(s) and registers their tools.
func (p *MCPProxy) handleInitialize(ctx context.Context, server *mcp.Server, session *CentianProxySession) (*mcp.InitializeResult, error) {
	if session.initialized {
		return p.buildInitializeResult(), nil
	}

	log.Printf("MCPProxy[%s]: Initializing session %s", p.name, session.id)

	// Connect to all downstreams (parallel for aggregated, single iteration for single mode)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []string

	for serverName, connTemplate := range p.downstreams {
		wg.Add(1)
		go func(name string, template *DownstreamConnection) {
			defer wg.Done()

			// Create fresh connection for this session
			conn := NewDownstreamConnection(name, template.config)

			if err := conn.Connect(ctx, session.authHeaders); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				mu.Unlock()
				log.Printf("MCPProxy[%s]: Failed to connect to %s: %v", p.name, name, err)
				return
			}

			mu.Lock()
			session.downstreamConns[name] = conn
			mu.Unlock()

			log.Printf("MCPProxy[%s]: Connected to %s, found %d tools", p.name, name, len(conn.Tools()))
		}(serverName, connTemplate)
	}

	wg.Wait()

	if len(session.downstreamConns) == 0 {
		return nil, fmt.Errorf("failed to connect to any downstream servers: %v", errors)
	}

	// Register tools from all connected downstreams
	p.registerToolsForSession(server, session)

	// Update tool registry (for aggregated mode)
	if p.isAggregatedProxy {
		p.mu.Lock()
		for serverName, conn := range session.downstreamConns {
			for _, tool := range conn.Tools() {
				namespacedName := fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
				p.toolRegistry[namespacedName] = serverName
			}
		}
		p.mu.Unlock()
	}

	session.initialized = true
	return p.buildInitializeResult(), nil
}

func (p *MCPProxy) buildInitializeResult() *mcp.InitializeResult {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}
	return &mcp.InitializeResult{
		ProtocolVersion: "2025-11-25",
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
		ServerInfo: &mcp.Implementation{
			Name:    serverName,
			Version: "1.0.0",
		},
	}
}

// registerToolsForSession registers tools from all downstream connections.
// If namespaceTools is true, tools are prefixed with "serverName__".
func (p *MCPProxy) registerToolsForSession(server *mcp.Server, session *CentianProxySession) {
	for serverName, conn := range session.downstreamConns {
		for _, tool := range conn.Tools() {
			p.registerTool(server, session, serverName, tool)
		}
	}
}

func (p *MCPProxy) registerTool(server *mcp.Server, session *CentianProxySession, serverName string, tool *mcp.Tool) {
	clonedTool := deepCloneTool(tool)
	toolServerName := serverName // capture for closure

	if p.isAggregatedProxy {
		// Aggregated mode: namespace tools to avoid collisions
		clonedTool.Name = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
		clonedTool.Description = fmt.Sprintf("[%s] %s", serverName, tool.Description)
	}
	// else: pass-through mode - keep original name

	server.AddTool(clonedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleToolCall(ctx, session, toolServerName, req)
	})
}

func (p *MCPProxy) handleToolCall(ctx context.Context, session *CentianProxySession, serverName string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conn, exists := session.downstreamConns[serverName]
	if !exists || !conn.IsConnected() {
		return nil, fmt.Errorf("server %s not available", serverName)
	}

	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool arguments: %w", err)
	}

	// TODO: logging and processing hooks

	return conn.CallTool(ctx, req.Params.Name, args)
}

func deepCloneTool(tool *mcp.Tool) *mcp.Tool {
	return &mcp.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		Annotations:  tool.Annotations,
		Meta:         tool.Meta,
		OutputSchema: tool.OutputSchema,
		Title:        tool.Title,
		Icons:        tool.Icons,
	}
}

// Close terminates all sessions and their downstream connections.
func (p *MCPProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, session := range p.sessions {
		for _, conn := range session.downstreamConns {
			conn.Close()
		}
	}
	return nil
}

// ============================================================================
// HTTP Handler Registration
// ============================================================================

// RegisterHandler registers a ServerProvider with the HTTP mux.
func RegisterHandler(endpoint string, proxy *MCPProxy, mux *http.ServeMux, options *mcp.StreamableHTTPOptions) {
	if options == nil {
		options = &mcp.StreamableHTTPOptions{
			SessionTimeout: 10 * time.Minute,
			Stateless:      false,
		}
	}
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return proxy.GetServerForRequest(r)
		},
		options,
	)
	mux.Handle(endpoint, handler)
	common.LogInfo("Registered handler at %s", endpoint)
}

// ============================================================================
// CentianServer Setup
// ============================================================================

// Setup uses CentianServer.config to create all gateways and endpoints.
func (c *CentianProxy) Setup() error {
	serverConfig := c.Config

	for gatewayName, gatewayConfig := range serverConfig.Gateways {
		endpoint, err := getEndpointString(gatewayName, "")
		if err != nil {
			common.LogError("error creating endpoint for gateway '%s': %s", gatewayName, err.Error())
			continue
		}

		// Create aggregated proxy for the gateway
		gateway := NewAggregatedProxy(gatewayName, endpoint, gatewayConfig)
		gateway.server = c
		c.Gateways[gatewayName] = gateway

		// Register aggregated endpoint
		// TODO: make this configurable
		// TODO: allow "tool registry mode" where we provide tool search
		RegisterHandler(gateway.endpoint, gateway, c.Mux, nil)

		// Optionally: register individual endpoints for each server
		// TODO: make this configurable
		for serverName, serverCfg := range gatewayConfig.MCPServers {
			if serverCfg.Enabled {
				singleEndpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
				singleProxy := NewSingleProxy(serverName, singleEndpoint, serverCfg)
				RegisterHandler(singleEndpoint, singleProxy, c.Mux, nil)
			}
		}
	}

	return nil
}
