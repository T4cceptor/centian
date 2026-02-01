package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
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
	Name       string
	ServerID   string // used to uniquely identify this specific object instance
	Config     *config.GlobalConfig
	Mux        *http.ServeMux
	Server     *http.Server
	Logger     *logging.Logger // Shared base logger (ONE file handle)
	Gateways   map[string]*MCPProxy
	APIKeys    *auth.APIKeyStore
	AuthHeader string
}

// NewCentianProxy takes a GlobalConfig struct and returns a new CentianProxy.
func NewCentianProxy(globalConfig *config.GlobalConfig) (*CentianProxy, error) {
	if globalConfig == nil || globalConfig.Proxy == nil {
		return nil, fmt.Errorf("proxy settings are required")
	}

	host := globalConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	if host == "0.0.0.0" && globalConfig.AuthEnabled == nil {
		return nil, fmt.Errorf("auth must be explicitly set when binding to 0.0.0.0")
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         net.JoinHostPort(host, globalConfig.Proxy.Port),
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
		WriteTimeout: common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
	}
	logger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	// loading API Key store
	var apiKeyStore *auth.APIKeyStore
	if globalConfig.IsAuthEnabled() {
		loadedStore, err := auth.LoadDefaultAPIKeys()
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeysNotFound) {
				return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian server get-key` to create a new api key\nError: %w", err)
			}
			return nil, fmt.Errorf("failed to load api keys: %w", err)
		}
		apiKeyStore = loadedStore
		common.LogInfo("Loaded %d API keys from %s", apiKeyStore.Count(), apiKeyStore.Path())
	} else {
		common.LogInfo("API key auth disabled via config")
	}

	return &CentianProxy{
		Config:     globalConfig,
		Mux:        mux,
		Server:     server,
		Logger:     logger,
		ServerID:   getServerID(globalConfig.Name),
		Gateways:   make(map[string]*MCPProxy),
		APIKeys:    apiKeyStore,
		AuthHeader: globalConfig.GetAuthHeader(),
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
	upstreamServer  *mcp.Server
	downstreamConns map[string]*DownstreamConnection // serverName → connection
	authHeaders     map[string]string
}

// GetConnectionByName returns a MCP connection for the given server name.
func (s *CentianProxySession) GetConnectionByName(serverName string) (*DownstreamConnection, error) {
	conn, ok := s.downstreamConns[serverName]
	if !ok {
		return nil, fmt.Errorf("no connection to server '%s' found", serverName)
	}
	return conn, nil
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

	// Back-reference to parent server
	server *CentianProxy
	config *config.GatewayConfig

	// Event processor for logging and processing hooks
	eventProcessor *EventProcessor

	mu sync.RWMutex
}

// NewAggregatedProxy creates a proxy that aggregates multiple downstream servers.
// Tools from each server are namespaced as "serverName__toolName" to avoid collisions.
func NewAggregatedProxy(name, endpoint string, gatewayConfig *config.GatewayConfig) *MCPProxy {
	proxy := &MCPProxy{
		name:              name,
		endpoint:          endpoint,
		config:            gatewayConfig,
		downstreams:       make(map[string]*DownstreamConnection),
		sessions:          make(map[string]*CentianProxySession),
		isAggregatedProxy: true,
	}

	// Pre-create downstream templates from config
	for serverName, serverCfg := range gatewayConfig.MCPServers {
		if serverCfg.IsEnabled() {
			proxy.downstreams[serverName] = NewDownstreamConnection(serverName, serverCfg)
		}
	}

	return proxy
}

// NewSingleProxy creates a proxy for a single downstream server.
// Tools pass through with their original names (no namespacing).
func NewSingleProxy(serverName, endpoint string, serverConfig *config.MCPServerConfig) *MCPProxy {
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
		upstreamServer, err := p.getServerForSession(session)
		if err != nil {
			common.LogError(err.Error())
			return nil
		} else {
			session.upstreamServer = upstreamServer
		}
	}
	return session.upstreamServer
}

func (p *MCPProxy) createSession(id string, r *http.Request) *CentianProxySession {
	authHeaders := make(map[string]string)
	// Capture auth headers from upstream request for passthrough
	// TODO: make these headers configurable
	for _, h := range []string{"Authorization", "X-API-Key", "X-Auth-Token"} {
		if p.server != nil && p.server.AuthHeader != "" && strings.EqualFold(h, p.server.AuthHeader) {
			continue
		}
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

// handleInitialize connects to downstream server(s) and registers their tools.
func (p *MCPProxy) getServerForSession(session *CentianProxySession) (*mcp.Server, error) {
	if session.initialized {
		return session.upstreamServer, nil
	}

	log.Printf("MCPProxy[%s]: Initializing session %s", p.name, session.id)

	// Log session initialization event
	p.logSessionEvent(session, "session_init", "Session initialization started")

	// Connect to all downstreams (parallel for aggregated, single iteration for single mode)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var connectionErrors []string

	for serverName, connTemplate := range p.downstreams {
		wg.Add(1)
		go func(name string, template *DownstreamConnection) {
			defer wg.Done()

			// Create fresh connection for this session
			conn := NewDownstreamConnection(name, template.config)
			ctx := context.Background()
			if err := conn.Connect(ctx, session.authHeaders); err != nil {
				mu.Lock()
				connectionErrors = append(connectionErrors, fmt.Sprintf("%s: %v", name, err))
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
		return nil, fmt.Errorf("failed to connect to any downstream servers: %v", connectionErrors)
	}

	// Register tools from all connected downstreams
	server := p.NewMcpServer()
	p.registerToolsForSession(server, session)
	session.initialized = true
	return server, nil
}

// NewMcpServer returns a new *mcp.Server based on the MCPProxy name.
func (p *MCPProxy) NewMcpServer() *mcp.Server {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}
	return mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{
				ListChanged: true,
				// NOTE: this is important as we want the client to know we support tools,
				// however these are NOT added initially and will only be available on the
				// first connect
			},
		},
	})
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

	// =========================================================================
	// INFLECTION POINT 1: Process REQUEST (Client → Server)
	// =========================================================================
	var reqEvent *common.MCPEvent
	if p.eventProcessor != nil {
		reqEvent = p.buildRequestEvent(session, serverName, req)
		if err := p.eventProcessor.Process(reqEvent); err != nil {
			common.LogError("request processing error: %s", err.Error())
		}

		// Check if request was rejected by processor
		if reqEvent.Status >= 400 {
			errMsg := "Request rejected by processor"
			if reqEvent.Error != "" {
				errMsg = reqEvent.Error
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: errMsg},
				},
			}, nil
		}
	}

	// =========================================================================
	// Call downstream MCP server
	// =========================================================================
	toolName := req.Params.Name
	if p.isAggregatedProxy {
		// here we need to modify the tool name to restore the original downstream
		// name, otherwise the tool will not be found (it does not exist on the
		// downstream server)
		parts := strings.SplitN(toolName, NamespaceSeparator, 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("failed to recreate original tool name from: %s", req.Params.Name)
		}
		toolName = strings.Join(parts[1:], "")
	}
	result, err := conn.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, err
	}

	// =========================================================================
	// INFLECTION POINT 2: Process RESPONSE (Server → Client)
	// =========================================================================
	if p.eventProcessor != nil {
		respEvent := p.buildResponseEvent(session, serverName, req, result, reqEvent)
		if err := p.eventProcessor.Process(respEvent); err != nil {
			common.LogError("response processing error: %s", err.Error())
		}

		// If response was modified by processor, we could update result here
		// For now, we just log - modification would require parsing respEvent.RawMessage()
	}

	return result, nil
}

func getRoutingContext(proxy *MCPProxy, session *CentianProxySession, serverName string) *common.RoutingContext {
	connection, err := session.GetConnectionByName(serverName)
	if err != nil {
		common.LogError(err.Error())
		return &common.RoutingContext{
			Gateway:    proxy.name,
			ServerName: serverName,
			Endpoint:   proxy.endpoint,
		}
	}
	transport := common.HTTPTransport
	if connection.config.URL == "" && connection.config.Command != "" {
		transport = common.StdioTransport
	}
	return &common.RoutingContext{
		Transport:         transport,
		Gateway:           proxy.name,
		ServerName:        serverName,
		Endpoint:          proxy.endpoint,
		DownstreamURL:     connection.config.URL,
		DownstreamCommand: connection.config.Command,
		Args:              connection.config.Args,
	}
}

// buildRequestEvent creates an MCPEvent for a tool call request.
func (p *MCPProxy) buildRequestEvent(session *CentianProxySession, serverName string, req *mcp.CallToolRequest) *common.MCPEvent {
	serverID := ""
	if p.server != nil {
		serverID = p.server.ServerID
	}

	// Build JSON-RPC message
	rawMsg, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      req.Params.Name,
			"arguments": req.Params.Arguments,
		},
	})
	routing := getRoutingContext(p, session, serverName)
	event := common.NewMCPRequestEvent(string(routing.Transport)).
		WithRequestID(getNewUUIDV7()).
		WithSessionID(session.id).
		WithServerID(serverID).
		WithToolCall(req.Params.Name, req.Params.Arguments).
		WithRawMessage(string(rawMsg))
	event.Routing = *routing
	return event
}

// buildResponseEvent creates an MCPEvent for a tool call response.
func (p *MCPProxy) buildResponseEvent(
	session *CentianProxySession,
	serverName string,
	req *mcp.CallToolRequest,
	result *mcp.CallToolResult,
	reqEvent *common.MCPEvent,
) *common.MCPEvent {
	serverID := ""
	if p.server != nil {
		serverID = p.server.ServerID
	}

	// Use request ID from request event if available
	requestID := getNewUUIDV7()
	if reqEvent != nil {
		requestID = reqEvent.RequestID
	}

	// Marshal result for raw message
	resultJSON, _ := json.Marshal(result)
	rawMsg, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"result":  result,
	})

	routing := getRoutingContext(p, session, serverName)
	event := common.NewMCPResponseEvent(string(routing.Transport)).
		WithRequestID(requestID).
		WithSessionID(session.id).
		WithServerID(serverID).
		WithToolCall(req.Params.Name, req.Params.Arguments).
		WithToolResult(resultJSON, result.IsError).
		WithRawMessage(string(rawMsg))
	event.Routing = *routing
	event.Success = !result.IsError
	return event
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

	var err error = nil
	for _, session := range p.sessions {
		for _, conn := range session.downstreamConns {
			err = conn.Close()
		}
	}
	return err
}

// ============================================================================
// HTTP Handler Registration
// ============================================================================

func apiKeyMiddlewareWithHeader(store *auth.APIKeyStore, headerName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			next.ServeHTTP(w, r)
			return
		}

		token := extractAuthToken(r.Header.Get(headerName))
		if token == "" {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: missing auth token from %s", r.RemoteAddr)
			return
		}

		if !store.Validate(token) {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: invalid auth token from %s", r.RemoteAddr)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractAuthToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return header
}

func writeUnauthorized(w http.ResponseWriter, headerName string) {
	if strings.EqualFold(headerName, "Authorization") {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

// RegisterHandler registers a ServerProvider with the HTTP mux.
func RegisterHandler(endpoint string, proxy *MCPProxy, mux *http.ServeMux, options *mcp.StreamableHTTPOptions) {
	if options == nil {
		options = &mcp.StreamableHTTPOptions{
			SessionTimeout: 10 * time.Minute,
			Stateless:      false,
		}
	}
	baseHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return proxy.GetServerForRequest(r)
		},
		options,
	)

	var handler http.Handler = baseHandler
	if proxy.server != nil && proxy.server.APIKeys != nil {
		headerName := proxy.server.AuthHeader
		if headerName == "" {
			headerName = strings.Clone(config.DefaultAuthHeader)
		}
		handler = apiKeyMiddlewareWithHeader(proxy.server.APIKeys, headerName, handler)
	}

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

		// Initialize event processor for the gateway
		gateway.initEventProcessor()

		// Register aggregated endpoint
		RegisterHandler(gateway.endpoint, gateway, c.Mux, nil)

		// Optionally: register individual endpoints for each server
		// TODO: make this configurable
		for serverName, serverCfg := range gatewayConfig.MCPServers {
			if !serverCfg.IsEnabled() {
				continue
			}
			singleEndpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			singleProxy := NewSingleProxy(serverName, singleEndpoint, serverCfg)
			singleProxy.server = c
			singleProxy.initEventProcessor()
			RegisterHandler(singleEndpoint, singleProxy, c.Mux, nil)
		}
	}
	return nil
}

// logSessionEvent logs a system event for session lifecycle.
func (p *MCPProxy) logSessionEvent(session *CentianProxySession, eventType, message string) {
	if p.eventProcessor == nil {
		return
	}

	serverID := ""
	if p.server != nil {
		serverID = p.server.ServerID
	}

	routing := common.RoutingContext{
		Gateway:    p.name,
		ServerName: "",
		Endpoint:   p.endpoint,
	}
	event := common.NewMCPSystemEvent("sdk").
		WithRequestID(getNewUUIDV7()).
		WithSessionID(session.id).
		WithServerID(serverID).
		WithRawMessage(fmt.Sprintf(`{"event_type":%q,"message":%q}`, eventType, message))
	event.Routing = routing
	event.Metadata["event_type"] = eventType

	// Only log, don't run through processor chain for system events
	if p.server != nil && p.server.Logger != nil {
		if err := p.server.Logger.LogMcpEvent(event); err != nil {
			common.LogError("failed to log session event: %s", err.Error())
		}
	}
}

// initEventProcessor initializes the event processor for this MCPProxy.
// It combines global processors with gateway-specific processors.
func (p *MCPProxy) initEventProcessor() {
	if p.server == nil {
		common.LogWarn("MCPProxy[%s]: Cannot initialize processor - no server reference", p.name)
		return
	}

	// Collect all processor configs (global + gateway-specific)
	var allProcessors []*config.ProcessorConfig

	// Add global processors
	if p.server.Config.Processors != nil {
		allProcessors = append(allProcessors, p.server.Config.Processors...)
	}

	// Add gateway-specific processors
	if p.config != nil && p.config.Processors != nil {
		allProcessors = append(allProcessors, p.config.Processors...)
	}

	// Create processor chain
	sessionID := fmt.Sprintf("gateway_%s_%d", p.name, time.Now().UnixNano())
	processorChain, err := processor.NewChain(allProcessors, p.server.Config.Name, sessionID)
	if err != nil {
		common.LogError("MCPProxy[%s]: Failed to create processor chain: %s", p.name, err.Error())
		return
	}

	// Create event processor
	p.eventProcessor = NewEventProcessor(p.server.Logger, processorChain)
	common.LogInfo("MCPProxy[%s]: Initialized event processor with %d processors", p.name, len(allProcessors))
}
