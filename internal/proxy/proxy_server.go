package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	centauth "github.com/T4cceptor/centian/internal/auth"
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
	APIKeys    *centauth.APIKeyStore
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
	var apiKeyStore *centauth.APIKeyStore
	if globalConfig.IsAuthEnabled() {
		loadedStore, err := centauth.LoadDefaultAPIKeys()
		if err != nil {
			if errors.Is(err, centauth.ErrAPIKeysNotFound) {
				return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian auth new-key` to create a new api key\nError: %w", err)
			}
			return nil, fmt.Errorf("failed to load api keys: %w", err)
		}
		apiKeyStore = loadedStore
		keyCount := apiKeyStore.Count()
		if keyCount == 0 {
			common.LogWarn("Auth enabled but no API keys available from %s\n", apiKeyStore.Path())
		} else {
			common.LogInfo("Loaded %d API keys from %s\n", apiKeyStore.Count(), apiKeyStore.Path())
		}
	} else {
		common.LogInfo("API key auth disabled via config\n")
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

// UpstreamSession represents one MCP client session talking to this proxy endpoint.
//
// It owns only upstream-facing state:
// - the upstream MCP session ID
// - the per-session upstream mcp.Server exposed to the SDK
// - the resolved request identity and forwarded auth headers captured from the client
//
// It does not own downstream lifecycle. Instead, it points at a reusable
// DownstreamConnectionPool via poolKey and receives whichever downstream
// connections that pool currently owns.
type UpstreamSession struct {
	id              string
	initialized     bool
	upstreamServer  *mcp.Server                              // Server returned to the MCP SDK for this upstream session.
	downstreamConns map[string]DownstreamConnectionInterface // Current downstream connections borrowed from the assigned pool.
	registeredTools map[string]struct{}                      // Tools already registered on upstreamServer for this session.
	authHeaders     map[string]string                        // Forwarded auth headers captured from the client request that created this session.
	identityKey     string                                   // Stable caller identity used when choosing a reusable downstream pool.
	poolKey         string                                   // Lookup key of the DownstreamConnectionPool backing this upstream session.
}

// DownstreamConnectionPool owns the reusable downstream connection set for one
// pool key.
//
// A pool key represents one proxy endpoint plus the caller identity and any
// forwarded auth state that affects downstream initialization. Multiple
// UpstreamSessions can attach to the same pool so reconnecting clients can reuse
// already-initialized downstream MCP sessions.
type DownstreamConnectionPool struct {
	identityKey      string
	poolKey          string
	downstreamConns  map[string]DownstreamConnectionInterface // Reusable downstream connections owned by this pool.
	upstreamSessions map[string]*UpstreamSession              // Upstream sessions currently attached to this pool.
	connecting       map[string]bool                          // Tracks downstreams currently connecting in the background.
	lastUsed         time.Time                                // Timestamp for future cleanup/eviction decisions.
}

const initialDownstreamReadyWait = 500 * time.Millisecond

// GetConnectionByName returns a MCP connection for the given server name.
func (s *UpstreamSession) GetConnectionByName(serverName string) (DownstreamConnectionInterface, error) {
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

	// upstreamSessions tracks MCP client sessions by the upstream session ID
	// provided by the SDK/client.
	upstreamSessions map[string]*UpstreamSession

	// downstreamPools tracks reusable downstream connection ownership by poolKey.
	downstreamPools map[string]*DownstreamConnectionPool

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

	connectionFactory func(string, *config.MCPServerConfig) DownstreamConnectionInterface
}

// NewAggregatedProxy creates a proxy that aggregates multiple downstream servers.
// Tools from each server are namespaced as "serverName__toolName" to avoid collisions.
func NewAggregatedProxy(gatewayName, endpoint string, gatewayConfig *config.GatewayConfig) *MCPProxy {
	proxy := &MCPProxy{
		name:              gatewayName,
		endpoint:          endpoint,
		config:            gatewayConfig,
		downstreams:       make(map[string]*DownstreamConnection),
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
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
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamConnectionPool),
		isAggregatedProxy: false,
	}
}

// GetServerForRequest returns (or creates) an MCP server for the given HTTP request's session.
func (p *MCPProxy) GetServerForRequest(r *http.Request) *mcp.Server {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = "csid-" + getNewUUIDV7() // we add "csid" to identify the session id as "internal" / not provided by the client!
	}
	common.LogInfo("MCPProxy[%s]: Getting server for session %s", p.name, sessionID)

	identityKey, err := p.resolveIdentityKey(r)
	if err != nil {
		common.LogError("MCPProxy[%s]: Failed to resolve request identity: %v", p.name, err)
		return nil
	}

	p.mu.Lock()
	upstreamSession, exists := p.upstreamSessions[sessionID]
	var (
		downstreamPool *DownstreamConnectionPool
		reusedPool     bool
		newSession     bool
	)
	if !exists {
		upstreamSession = p.createUpstreamSession(sessionID, r, identityKey)
		p.upstreamSessions[sessionID] = upstreamSession
		downstreamPool, reusedPool = p.initializeUpstreamSessionLocked(upstreamSession)
		newSession = true
	} else if upstreamSession.identityKey != identityKey {
		p.mu.Unlock()
		common.LogWarn(
			"MCPProxy[%s]: session identity mismatch for session %s (existing=%s current=%s)",
			p.name,
			sessionID,
			upstreamSession.identityKey,
			identityKey,
		)
		return nil
	}
	p.mu.Unlock()

	if newSession {
		if !reusedPool {
			p.waitForFirstUsableDownstream(downstreamPool, initialDownstreamReadyWait)
		}
		p.registerConnectedToolsForUpstreamSession(upstreamSession, downstreamPool)
	}
	return upstreamSession.upstreamServer
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

// createUpstreamSession captures the request state needed to attach a new
// upstream MCP session to the correct reusable downstream pool.
func (p *MCPProxy) createUpstreamSession(id string, r *http.Request, identityKey string) *UpstreamSession {
	authHeaders := getAuthHeaders(p, r)
	return &UpstreamSession{
		id:              id,
		initialized:     false,
		downstreamConns: make(map[string]DownstreamConnectionInterface),
		registeredTools: make(map[string]struct{}),
		authHeaders:     authHeaders,
		identityKey:     identityKey,
		poolKey:         p.getDownstreamPoolKey(identityKey, authHeaders),
	}
}

// sanitizeLogValue strips control characters to prevent log injection.
func sanitizeLogValue(value string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r':
			return ' '
		case r < 32 || r == 127:
			return -1
		default:
			return r
		}
	}, value)
	return strings.TrimSpace(sanitized)
}

func (p *MCPProxy) resolveIdentityKey(r *http.Request) (string, error) {
	if p.server == nil || p.server.APIKeys == nil {
		return sharedLocalIdentity, nil
	}
	identity, ok := requestIdentityFromContext(r.Context())
	if !ok {
		return "", fmt.Errorf("authenticated request missing resolved API key identity")
	}
	return identity, nil
}

func (p *MCPProxy) getDownstreamPoolKey(identityKey string, authHeaders map[string]string) string {
	return fmt.Sprintf("%s|%s|%s", p.endpoint, identityKey, authHeadersFingerprint(authHeaders))
}

func (p *MCPProxy) newDownstreamConnection(name string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
	if p.connectionFactory != nil {
		return p.connectionFactory(name, cfg)
	}
	return NewDownstreamConnection(name, cfg)
}

// initializeUpstreamSessionLocked attaches a new upstream session to the
// appropriate downstream pool. Caller must hold p.mu.
func (p *MCPProxy) initializeUpstreamSessionLocked(session *UpstreamSession) (*DownstreamConnectionPool, bool) {
	//nolint:gosec // session.id is sanitized for log safety; gosec cannot infer custom sanitizers.
	common.LogDebug("MCPProxy[%s]: Initializing session %s", p.name, sanitizeLogValue(session.id))

	// Create server immediately (empty tools initially)
	server := p.NewMcpServer()
	session.upstreamServer = server

	downstreamPool, reused := p.getOrCreateDownstreamPool(session)
	session.downstreamConns = downstreamPool.downstreamConns
	session.initialized = true
	return downstreamPool, reused
}

// getOrCreateDownstreamPool returns the reusable downstream pool matching the
// upstream session's poolKey and ensures any missing/failed downstream
// connections are being connected.
func (p *MCPProxy) getOrCreateDownstreamPool(session *UpstreamSession) (*DownstreamConnectionPool, bool) {
	if existing, ok := p.downstreamPools[session.poolKey]; ok {
		existing.lastUsed = time.Now()
		existing.upstreamSessions[session.id] = session
		p.ensureDownstreamConnectionsLocked(existing, session.authHeaders)
		return existing, true
	}

	entry := &DownstreamConnectionPool{
		identityKey:      session.identityKey,
		poolKey:          session.poolKey,
		downstreamConns:  make(map[string]DownstreamConnectionInterface, len(p.downstreams)),
		upstreamSessions: map[string]*UpstreamSession{session.id: session},
		connecting:       make(map[string]bool),
		lastUsed:         time.Now(),
	}
	p.downstreamPools[session.poolKey] = entry
	p.ensureDownstreamConnectionsLocked(entry, session.authHeaders)
	return entry, false
}

func (p *MCPProxy) ensureDownstreamConnectionsLocked(entry *DownstreamConnectionPool, authHeaders map[string]string) {
	for serverName, connTemplate := range p.downstreams {
		conn, exists := entry.downstreamConns[serverName]
		if !exists || conn.GetStatus() == StatusFailed {
			conn = p.newDownstreamConnection(serverName, connTemplate.config)
			entry.downstreamConns[serverName] = conn
		}
		if entry.connecting[serverName] {
			continue
		}
		if conn.GetStatus() == StatusConnected && conn.IsConnected() {
			continue
		}
		entry.connecting[serverName] = true
		go p.connectDownstreamPoolConnection(entry.poolKey, serverName, conn, cloneAuthHeaders(authHeaders))
	}
}

func (p *MCPProxy) waitForFirstUsableDownstream(pool *DownstreamConnectionPool, timeout time.Duration) {
	if pool == nil {
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.poolHasUsableDownstream(pool) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (p *MCPProxy) poolHasUsableDownstream(pool *DownstreamConnectionPool) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for serverName, conn := range pool.downstreamConns {
		if pool.connecting[serverName] {
			continue
		}
		if conn.IsConnected() && len(conn.Tools()) > 0 {
			return true
		}
	}
	return false
}

func (p *MCPProxy) registerConnectedToolsForUpstreamSession(
	upstreamSession *UpstreamSession,
	downstreamPool *DownstreamConnectionPool,
) {
	if upstreamSession == nil || upstreamSession.upstreamServer == nil || downstreamPool == nil {
		return
	}

	totalDownstreams := len(downstreamPool.downstreamConns)
	if totalDownstreams == 0 {
		common.LogWarn("MCPProxy[%s]: No downstream servers available for pool key %s", p.name, upstreamSession.poolKey)
		return
	}

	connectedCount := 0
	connectingCount := 0
	var connErrors []string

	p.mu.RLock()
	p.toolRegMu.Lock()
	for serverName, conn := range downstreamPool.downstreamConns {
		if downstreamPool.connecting[serverName] {
			connectingCount++
			continue
		}
		if conn.GetStatus() == StatusConnected && conn.IsConnected() {
			connectedCount++
			for _, tool := range conn.Tools() {
				p.registerTool(upstreamSession.upstreamServer, upstreamSession, serverName, tool)
			}
			continue
		}
		if conn.GetStatus() == StatusConnecting || conn.GetStatus() == StatusPending {
			connectingCount++
			continue
		}
		if conn.GetStatus() == StatusFailed && conn.GetError() != nil {
			connErrors = append(connErrors, fmt.Sprintf("%s: %v", serverName, conn.GetError()))
		}
	}
	p.toolRegMu.Unlock()
	p.mu.RUnlock()

	switch {
	case connectedCount == 0 && connectingCount > 0:
		common.LogInfo(
			"MCPProxy[%s]: Initializing pooled downstream session %s in background (%d/%d still connecting)",
			p.name,
			upstreamSession.poolKey,
			connectingCount,
			totalDownstreams,
		)
	case connectedCount == 0 && len(connErrors) > 0:
		common.LogError("MCPProxy[%s]: All connections failed: %v", p.name, connErrors)
	case connectedCount == 0:
		common.LogWarn("MCPProxy[%s]: No pooled downstream connections are connected for %s", p.name, upstreamSession.poolKey)
	case connectedCount > 0:
		common.LogInfo(
			"MCPProxy[%s]: Upstream session %s using pooled downstream session %s with %d/%d connected servers",
			p.name,
			upstreamSession.id,
			upstreamSession.poolKey,
			connectedCount,
			totalDownstreams,
		)
	}
}

// connectDownstreamPoolConnection establishes one downstream connection owned by
// a reusable pool and registers its tools with every currently attached
// UpstreamSession once the connect succeeds.
func (p *MCPProxy) connectDownstreamPoolConnection(
	poolKey string,
	serverName string,
	conn DownstreamConnectionInterface,
	authHeaders map[string]string,
) {
	if err := conn.Connect(context.Background(), authHeaders); err != nil {
		p.mu.Lock()
		if entry, ok := p.downstreamPools[poolKey]; ok {
			if current, exists := entry.downstreamConns[serverName]; exists && current == conn {
				delete(entry.connecting, serverName)
			}
		}
		p.mu.Unlock()
		common.LogWarn("MCPProxy[%s]: Failed to connect pooled downstream %s: %v", p.name, serverName, err)
		return
	}

	p.mu.Lock()
	upstreamSessions := make([]*UpstreamSession, 0)
	if entry, ok := p.downstreamPools[poolKey]; ok {
		if current, exists := entry.downstreamConns[serverName]; exists && current == conn {
			delete(entry.connecting, serverName)
			entry.lastUsed = time.Now()
			for _, upstreamSession := range entry.upstreamSessions {
				upstreamSessions = append(upstreamSessions, upstreamSession)
			}
		}
	}
	p.mu.Unlock()

	if len(upstreamSessions) == 0 {
		return
	}

	p.toolRegMu.Lock()
	for _, upstreamSession := range upstreamSessions {
		if upstreamSession == nil || upstreamSession.upstreamServer == nil {
			continue
		}
		for _, tool := range conn.Tools() {
			p.registerTool(upstreamSession.upstreamServer, upstreamSession, serverName, tool)
		}
	}
	p.toolRegMu.Unlock()

	common.LogInfo("MCPProxy[%s]: Connected pooled downstream %s with %d tools", p.name, sanitizeLogValue(serverName), len(conn.Tools()))
}

// NewMcpServer returns a new *mcp.Server based on the MCPProxy name.
func (p *MCPProxy) NewMcpServer() *mcp.Server {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}
	return mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: p.server.Config.Version,
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

// registerTool adds one downstream tool to one upstream-facing server instance.
func (p *MCPProxy) registerTool(server *mcp.Server, upstreamSession *UpstreamSession, serverName string, tool *mcp.Tool) {
	if upstreamSession.registeredTools == nil {
		upstreamSession.registeredTools = make(map[string]struct{})
	}

	clonedTool := deepCloneTool(tool)
	toolServerName := serverName // capture for closure

	if p.isAggregatedProxy {
		// Aggregated mode: namespace tools to avoid collisions
		clonedTool.Name = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
		clonedTool.Description = fmt.Sprintf("[%s] %s", serverName, tool.Description)
	}

	if _, exists := upstreamSession.registeredTools[clonedTool.Name]; exists {
		return
	}
	upstreamSession.registeredTools[clonedTool.Name] = struct{}{}

	// else: pass-through mode - keep original name
	server.AddTool(clonedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleToolCall(ctx, upstreamSession, toolServerName, req)
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

func (p *MCPProxy) handleToolCall(ctx context.Context, upstreamSession *UpstreamSession, serverName string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Create CallContext
	callCtx, err := NewToolCallContext(ctx, p, upstreamSession, serverName, req)
	if err != nil {
		return nil, err
	}
	ctx = WithCallContext(ctx, callCtx)
	common.LogInfo("Tool called: %s :: %s", callCtx.GetServerName(), callCtx.GetToolName())

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
		p.invalidatePooledDownstream(upstreamSession.poolKey)
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
func (p *MCPProxy) Close() []error {
	p.mu.Lock()
	defer p.mu.Unlock()

	errs := make([]error, 0)
	for poolKey, entry := range p.downstreamPools {
		closeErrs := p.closePoolEntryLocked(entry)
		errs = append(errs, closeErrs...)
		delete(p.downstreamPools, poolKey)
	}
	return errs
}

// ============================================================================
// HTTP Handler Registration
// ============================================================================

func apiKeyMiddlewareWithHeader(store *centauth.APIKeyStore, headerName string, next http.Handler) http.Handler {
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

		entry, ok := store.Lookup(token)
		if !ok {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: invalid auth token from %s", r.RemoteAddr)
			return
		}

		ctx := withRequestIdentity(r.Context(), "auth:"+entry.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
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

func logRequestForDebug(r *http.Request) {
	if !common.DebugLoggingEnabled() || r == nil || r.Body == nil {
		return
	}

	common.LogDebug("Received request: %s - %s - %s", r.Method, r.URL, r.UserAgent())
	common.LogDebug("Received headers: %#v", redactHeaders(r.Header))

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		common.LogWarn("Failed to read request body for debug logging: %v", err)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}

	if len(bodyBytes) == 0 {
		common.LogDebug("Received request body: <empty>")
		return
	}

	common.LogDebug("Received request body (%d bytes): %s", len(bodyBytes), string(bodyBytes))
}

func authHeadersFingerprint(authHeaders map[string]string) string {
	if len(authHeaders) == 0 {
		return "forwarded-auth:none"
	}

	keys := make([]string, 0, len(authHeaders))
	for key := range authHeaders {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(authHeaders[key])
		builder.WriteString("\n")
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:8])
}

func cloneAuthHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func redactHeaders(headers http.Header) http.Header {
	redacted := make(http.Header, len(headers))
	for key, values := range headers {
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "X-Centian-Auth") ||
			strings.EqualFold(key, "X-API-Key") || strings.EqualFold(key, "X-Auth-Token") {
			redacted[key] = []string{"<redacted>"}
			continue
		}
		copied := make([]string, len(values))
		copy(copied, values)
		redacted[key] = copied
	}
	return redacted
}

func (p *MCPProxy) invalidatePooledDownstream(poolKey string) {
	if poolKey == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.downstreamPools[poolKey]
	if !ok {
		return
	}
	common.LogWarn("MCPProxy[%s]: Invalidating pooled downstream session %s", p.name, poolKey)
	_ = p.closePoolEntryLocked(entry)
	delete(p.downstreamPools, poolKey)
}

func (p *MCPProxy) closePoolEntryLocked(entry *DownstreamConnectionPool) []error {
	if entry == nil {
		return nil
	}
	errs := make([]error, 0)
	for _, conn := range entry.downstreamConns {
		if closeErr := conn.Close(); closeErr != nil {
			errs = append(errs, closeErr)
		}
	}
	return errs
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
			logRequestForDebug(r)
			return proxy.GetServerForRequest(r)
		},
		options,
	)

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseHandler.ServeHTTP(w, r)
	})
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
	processorCount := len(allProcessors)
	if processorCount > 0 {
		common.LogInfo("MCPProxy[%s]: Initialized event processor with %d processors", p.name, processorCount)
	}
	return nil
}
