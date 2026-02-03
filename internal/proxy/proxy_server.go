package proxy

import (
	"context"
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
		// here we enforce successful logger creation -> this means any request handling can assume a logger exists
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	// loading API Key store
	var apiKeyStore *auth.APIKeyStore
	if globalConfig.IsAuthEnabled() {
		loadedStore, err := auth.LoadDefaultAPIKeys()
		if err != nil {
			if errors.Is(err, auth.ErrAPIKeysNotFound) {
				return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian auth new-key` to create a new api key\nError: %w", err)
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
	initialized     bool                                     // TODO: when is this true?
	upstreamServer  *mcp.Server                              // The upstream server, connecting to the MCP client/AI agent
	downstreamConns map[string]DownstreamConnectionInterface // Downstream MCP servers
	authHeaders     map[string]string                        // Auth headers provided by the client for this session
}

// GetConnectionByName returns a MCP connection for the given server name.
func (s *CentianProxySession) GetConnectionByName(serverName string) (DownstreamConnectionInterface, error) {
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
	eventProcessor ProcessingControllerInterface

	mu sync.RWMutex

	// toolRegMu protects AddTool calls during progressive registration
	toolRegMu sync.Mutex
}

// NewAggregatedProxy creates a proxy that aggregates multiple downstream servers.
// Tools from each server are namespaced as "serverName__toolName" to avoid collisions.
func NewAggregatedProxy(gatewayName, endpoint string, gatewayConfig *config.GatewayConfig) *MCPProxy {
	proxy := &MCPProxy{
		name:              gatewayName,
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
		session.upstreamServer = p.getServerForSession(session)
	}
	return session.upstreamServer
}

func getAuthHeaders(p *MCPProxy, r *http.Request) map[string]string {
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
	return authHeaders
}

func (p *MCPProxy) createSession(id string, r *http.Request) *CentianProxySession {
	authHeaders := getAuthHeaders(p, r)
	return &CentianProxySession{
		id:              id,
		initialized:     false,
		downstreamConns: make(map[string]DownstreamConnectionInterface),
		authHeaders:     authHeaders,
	}
}

// Minimum wait time before returning server during progressive connection.
// Can be overridden via config in the future.
const defaultMinConnectionWait = 15 * time.Second

// getServerForSession connects to downstream server(s) and registers their tools progressively.
// Uses progressive connection: server is returned after minWait even if some downstreams are still connecting.
// Tools are registered as each downstream connects, and SDK auto-sends tools/list_changed notifications.
// Always returns a valid server (possibly with no tools if all connections failed).
func (p *MCPProxy) getServerForSession(session *CentianProxySession) *mcp.Server {
	if session.initialized {
		return session.upstreamServer
	}

	log.Printf("MCPProxy[%s]: Initializing session %s", p.name, session.id)

	// Create server immediately (empty tools initially)
	server := p.NewMcpServer()
	session.upstreamServer = server

	// Pre-create DownstreamConnection objects (status: pending)
	// This allows us to query their state during/after connection
	// Note: No lock needed here - session is not yet visible to other goroutines
	// (session.initialized is false, and we complete all writes before spawning goroutines)
	for serverName, connTemplate := range p.downstreams {
		conn := NewDownstreamConnection(serverName, connTemplate.config)
		session.downstreamConns[serverName] = conn
	}

	totalDownstreams := len(p.downstreams)
	if totalDownstreams == 0 {
		// No downstreams configured - return empty server
		session.initialized = true
		log.Printf("MCPProxy[%s]: No downstream servers configured", p.name)
		return server
	}

	// Channel to signal connection completion (success or failure)
	done := make(chan string, totalDownstreams)

	// Spawn connection goroutines - tools are registered as each connects
	for serverName := range p.downstreams {
		go p.connectAndRegister(session, server, serverName, done)
	}

	// Wait phase: minimum wait time OR all connections resolved (whichever first)
	resolved := p.waitForMinimumReady(done, totalDownstreams, defaultMinConnectionWait)

	session.initialized = true

	// Count successful connections
	connectedCount := 0
	for _, conn := range session.downstreamConns {
		if conn.GetStatus() == StatusConnected {
			connectedCount++
		}
	}

	switch {
	case connectedCount == 0 && resolved >= totalDownstreams:
		// All connections attempted but none succeeded
		var connErrors []string
		for name, conn := range session.downstreamConns {
			if conn.GetStatus() == StatusFailed && conn.GetError() != nil {
				connErrors = append(connErrors, fmt.Sprintf("%s: %v", name, conn.GetError()))
			}
		}
		log.Printf("MCPProxy[%s]: All connections failed: %v", p.name, connErrors)
		// Return server anyway - client will get empty tool list
		// This is arguably correct: no tools are available
	case connectedCount == 0:
		log.Printf("MCPProxy[%s]: No connections ready after initial wait, continuing in background", p.name)
	default:
		log.Printf("MCPProxy[%s]: %d/%d connections ready, %d still connecting",
			p.name, connectedCount, totalDownstreams, totalDownstreams-resolved)
	}

	return server
}

// NewMcpServer returns a new *mcp.Server based on the MCPProxy name.
func (p *MCPProxy) NewMcpServer() *mcp.Server {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}
	return mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: "1.0.0", // TODO: make this configurable - later version will be used to define capabilities!
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{
				// NOTE: setting ListChanged: true is important as we want the client to know we support
				// tools, however these are added on client connect and might not be available immediately
				// on initialize
				ListChanged: true,
			},
		},
	})
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

// ProcessCall handles the request phase processing using handlers.
// It gathers input from handlers, passes it to processors, and applies results back.
//
// If Error is non-nil, a mandatory processor failed to process the CallContext.
// Otherwise, processors ran as intended. Note: this can still lead to an error response.
func (p *MCPProxy) ProcessCall(callCtx CallContext, direction common.McpEventDirection, msgType common.McpMessageType) error {
	if p.eventProcessor == nil {
		// Nothing to do here
		return nil
	}

	// Process through the chain
	callCtx.SetDirection(direction)
	callCtx.SetMessageType(msgType)
	if err := p.eventProcessor.Process(callCtx); err != nil {
		return err
	}
	return nil
}

// connectAndRegister handles single downstream connection + tool registration.
// Connection must already exist in session.downstreamConns with status pending.
func (p *MCPProxy) connectAndRegister(
	session *CentianProxySession,
	server *mcp.Server,
	serverName string,
	done chan<- string,
) {
	defer func() { done <- serverName }()

	// Get the pre-created connection (status: pending)
	// Note: No lock needed - session.downstreamConns was fully populated before this goroutine started
	conn := session.downstreamConns[serverName]

	if conn == nil {
		log.Printf("MCPProxy[%s]: No connection object for %s", p.name, serverName)
		return
	}

	ctx := context.Background()

	// Connect updates conn.status internally (connecting → connected/failed)
	if err := conn.Connect(ctx, session.authHeaders); err != nil {
		// conn.status already set to StatusFailed by Connect()
		log.Printf("MCPProxy[%s]: Failed to connect to %s: %v", p.name, serverName, err)
		return
	}

	// Connection successful - register tools with mutex protection
	p.toolRegMu.Lock()
	for _, tool := range conn.Tools() {
		p.registerTool(server, session, serverName, tool)
	}
	p.toolRegMu.Unlock()

	log.Printf("MCPProxy[%s]: Connected to %s, registered %d tools", p.name, serverName, len(conn.Tools()))
}

// waitForMinimumReady waits for minimum time OR all connections resolved (whichever first).
func (p *MCPProxy) waitForMinimumReady(
	done <-chan string,
	total int,
	minWait time.Duration,
) int {
	resolved := 0

	timer := time.NewTimer(minWait)
	defer timer.Stop()

	for resolved < total {
		select {
		case <-done:
			resolved++
			// If all connections resolved before minWait, exit early
			if resolved >= total {
				return resolved
			}
		case <-timer.C:
			// Minimum wait elapsed - return control to caller
			// Remaining connections continue in background
			log.Printf("MCPProxy[%s]: Minimum wait elapsed, %d/%d connections resolved", p.name, resolved, total)
			return resolved
		}
	}
	return resolved
}

func (p *MCPProxy) handleToolCall(ctx context.Context, session *CentianProxySession, serverName string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Create CallContext
	callCtx, err := NewToolCallContext(ctx, p, session, serverName, req)
	if err != nil {
		return nil, err
	}
	ctx = WithCallContext(ctx, callCtx)

	// 2. Process REQUEST phase (Client → Server)
	if err := p.ProcessCall(callCtx, common.DirectionClientToServer, common.MessageTypeRequest); err != nil {
		// error  != nil indicates the processing failed on a mandatory processor
		// if no error is being returned the response can still be an error,
		// but it will be handled differently!
		return nil, err
	}

	// 3. If a processor/handler produced a result, return it without calling downstream.
	// This handles: rejections, cache hits, short-circuits, etc.
	if callCtx.HasResult() {
		return callCtx.GetResult(), nil
	}

	// 4. CallContext executes the downstream call
	if err := callCtx.SendRequest(ctx); err != nil {
		// Note: err != nil means there was an error in sending the request,
		// this does NOT have an impact on an error state being returned
		// from either the downstream MCP or any of the applied processors
		return nil, err
	}

	// 5. Process RESPONSE phase (Server → Client)
	if err := p.ProcessCall(callCtx, common.DirectionServerToClient, common.MessageTypeResponse); err != nil {
		// same applies here about the error, see above "2. Process REQUEST phase"
		return nil, err
	}
	// Note: HasResult will always be true here, compare to above HasResult check

	// 6. Return (potentially modified) result
	return callCtx.GetResult(), nil
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
	// TODO: move away from this file
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
	// TODO: move into utils file
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
	// TODO: move into utils file
	if strings.EqualFold(headerName, "Authorization") {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

// RegisterEndpoint registers a ServerProvider with the HTTP mux.
func RegisterEndpoint(endpoint string, proxy *MCPProxy, mux *http.ServeMux, options *mcp.StreamableHTTPOptions) {
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
		initErr := gateway.initEventProcessor()
		if initErr != nil {
			return initErr
		}

		// Register aggregated endpoint
		RegisterEndpoint(gateway.endpoint, gateway, c.Mux, nil)

		// Optionally: register individual endpoints for each server
		// TODO: make this configurable
		for serverName, serverCfg := range gatewayConfig.MCPServers {
			if !serverCfg.IsEnabled() {
				continue
			}
			singleEndpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			singleProxy := NewSingleProxy(serverName, singleEndpoint, serverCfg)
			singleProxy.server = c
			sErr := singleProxy.initEventProcessor()
			if sErr != nil {
				return sErr
			}
			RegisterEndpoint(singleEndpoint, singleProxy, c.Mux, nil)
		}
	}
	return nil
}

// initEventProcessor initializes the event processor for this MCPProxy.
// It combines global processors with gateway-specific processors.
func (p *MCPProxy) initEventProcessor() error {
	if p.server == nil {
		return fmt.Errorf("MCPProxy[%s]: Cannot initialize processor - no server reference", p.name)
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

	// Create event processor
	pc, err := NewProcessingController(allProcessors)
	if err != nil {
		return err
	}
	p.eventProcessor = pc
	common.LogInfo("MCPProxy[%s]: Initialized event processor with %d processors", p.name, len(allProcessors))
	return nil
}
