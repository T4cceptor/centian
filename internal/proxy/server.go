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
CentianServer is the main server struct.

It holds 4 critical components:
- mux - used to register URL paths
- server - used to serve the mux
- logger - main logger for all events in the proxied endpoints
- gateways - holds all gateways and proxy endpoints for easy access

Additionally it has a reference to the global config which was loaded to
initialize this server.
*/
type CentianServer struct {
	Name     string
	ServerID string // used to uniquely identify this specific object instance
	Config   *config.GlobalConfig
	Mux      *http.ServeMux
	Server   *http.Server
	Logger   *logging.Logger // Shared base logger (ONE file handle)
	Gateways map[string]*CentianProxyGateway
}

// NewProxyServer takes a GlobalConfig struct and returns a new CentianServer.
//
// Note: the server does not have gateways and endpoints attached until StartCentianServer is called.
func NewProxyServer(globalConfig *config.GlobalConfig) (*CentianServer, error) {
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

	return &CentianServer{
		Config:   globalConfig,
		Mux:      mux,
		Server:   server,
		Logger:   logger,
		ServerID: getServerID(globalConfig),
		Gateways: make(map[string]*CentianProxyGateway),
	}, nil
}

// CentianProxyGateway represents a gateway holding one or multiple CentianProxyEndpoint.
//
// The gateway provides a way to group MCP servers together that should all apply
// the same processing configuration.
type CentianProxyGateway struct {
	name      string
	config    *config.GatewayConfig
	endpoints []*CentianProxyEndpoint
	server    *CentianServer
	endpoint  string // e.g., "/mcp/default"

	// Downstream connections (created but not connected until init)
	downstreams map[string]*DownstreamConnection

	// Tool registry: namespacedTool → serverName
	toolRegistry map[string]string

	// Session management: upstreamSessionID → *CentianSession
	sessions map[string]*CentianSession

	mu sync.RWMutex
}

type CentianSession struct {
	id              string
	initialized     bool
	downstreamConns map[string]*DownstreamConnection // Per-session connections
	authHeaders     map[string]string                // Captured from upstream init
}

// GetNewGateway returns a new CentianProxyGateway for the given parameters.
func GetNewGateway(
	server *CentianServer,
	gatewayName string,
	gatewayConfig *config.GatewayConfig,
) (*CentianProxyGateway, error) {
	endpoint, err := getEndpointString(gatewayName, "")
	if err != nil {
		return nil, err
	}
	return &CentianProxyGateway{
		name:         gatewayName,
		endpoint:     endpoint,
		config:       gatewayConfig,
		endpoints:    make([]*CentianProxyEndpoint, 0),
		server:       server,
		downstreams:  make(map[string]*DownstreamConnection),
		toolRegistry: make(map[string]string),
		sessions:     make(map[string]*CentianSession),
	}, nil
}

func createSession(id string, r *http.Request) *CentianSession {
	authHeaders := make(map[string]string)
	// TODO: make these headers configurable
	for _, h := range []string{"Authorization", "X-API-Key", "X-Auth-Token"} {
		if v := r.Header.Get(h); v != "" {
			authHeaders[h] = v
		}
	}
	return &CentianSession{
		id:              id,
		initialized:     false,
		downstreamConns: make(map[string]*DownstreamConnection),
		authHeaders:     authHeaders,
	}
}

// getServerForRequest returns (or creates) an MCP server for this session
func (cpg *CentianProxyGateway) GetServerForRequest(r *http.Request) *mcp.Server {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = getNewUUIDV7()
	}
	common.LogInfo("Getting server for %s\n", sessionID)

	cpg.mu.Lock()
	defer cpg.mu.Unlock()

	session, exists := cpg.sessions[sessionID]
	if !exists {
		session = createSession(sessionID, r)
		cpg.sessions[sessionID] = session
	}
	return createServerForSession(cpg, session)
}

func createServerForSession(cpg *CentianProxyGateway, session *CentianSession) *mcp.Server {
	var server *mcp.Server
	server = mcp.NewServer(&mcp.Implementation{
		Name:    fmt.Sprintf("centian-gateway-%s", cpg.name),
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			// TODO: log init call
			cpg.handleInitialize(ctx, server, session, req)
		},
	})
	return server
}

type ServerProvider interface {
	GetServerForRequest(*http.Request) *mcp.Server
}

// RegisterHandler registers the aggregated endpoint with the HTTP mux
func RegisterHandler(endpoint string, sp ServerProvider, mux *http.ServeMux, options *mcp.StreamableHTTPOptions) {
	if options == nil {
		// using default options
		options = &mcp.StreamableHTTPOptions{
			SessionTimeout: 10 * time.Minute,
			Stateless:      false,
		}
	}
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			// TODO: log ?
			return sp.GetServerForRequest(r)
		},
		options,
	)
	mux.Handle(endpoint, handler)
	common.LogInfo("Registered handler at %s", endpoint)
}

// Setup uses CentianServer.config to create all gateways and endpoints.
func (c *CentianServer) Setup() error {
	serverConfig := c.Config
	// Iterate through each gateway to create proxy endpoints.
	for gatewayName, gatewayConfig := range serverConfig.Gateways {
		gateway, err := GetNewGateway(c, gatewayName, gatewayConfig)
		if err != nil {
			common.LogError("error while setting up gateway '%s': %s", gatewayName, err.Error())
			continue
		}
		c.Gateways[gatewayName] = gateway
		// Pre-create downstream connection wrappers (not connected yet)
		for serverName, serverCfg := range gateway.config.MCPServers {
			if serverCfg.Enabled {
				gateway.downstreams[serverName] = NewDownstreamConnection(serverName, serverCfg)
				// TODO: register individual endpoint
			}
		}
		RegisterHandler(gateway.endpoint, gateway, gateway.server.Mux, nil)
	}
	return nil
}

// handleInitialize - called when upstream client sends initialize
func (ag *CentianProxyGateway) handleInitialize(
	ctx context.Context,
	server *mcp.Server,
	session *CentianSession,
	req *mcp.InitializedRequest,
) (*mcp.InitializeResult, error) {
	if session.initialized {
		return ag.buildInitializeResult(session), nil
	}
	log.Printf("Initializing aggregated session %s", session.id)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []string
	for serverName, connTemplate := range ag.downstreams {
		wg.Add(1)
		go func(name string, template *DownstreamConnection) {
			defer wg.Done()
			conn := NewDownstreamConnection(name, template.config)

			// TODO: log
			if err := conn.Connect(ctx, session.authHeaders); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				mu.Unlock()
				log.Printf("Failed to connect to %s: %v", name, err)
				return
			}

			mu.Lock()
			session.downstreamConns[name] = conn
			mu.Unlock()

			log.Printf("Connected to downstream %s, found %d tools", name, len(conn.Tools()))
			registerToolsForSession(server, session)
		}(serverName, connTemplate)
	}

	wg.Wait()

	if len(session.downstreamConns) == 0 {
		return nil, fmt.Errorf("failed to connect to any downstream servers: %v", errors)
	}

	ag.mu.Lock()
	for serverName, conn := range session.downstreamConns {
		for _, tool := range conn.Tools() {
			namespacedName := fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
			ag.toolRegistry[namespacedName] = serverName
		}
	}
	ag.mu.Unlock()

	session.initialized = true

	return ag.buildInitializeResult(session), nil
}

func (ag *CentianProxyGateway) buildInitializeResult(session *CentianSession) *mcp.InitializeResult {
	return &mcp.InitializeResult{
		ProtocolVersion: "2025-11-25",
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{}, // TODO: double check if this is correct
		},
		ServerInfo: &mcp.Implementation{
			Name:    fmt.Sprintf("centian-gateway-%s", ag.name),
			Version: "1.0.0",
		},
	}
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

func RegisterToolAtServer(serverName string, server *mcp.Server, tool *mcp.Tool, session *CentianSession) {
	// 1. create namespaced tool name to avoid collision with other servers
	namespacedName := fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
	// 2. deep clone provided tool
	namespacedTool := deepCloneTool(tool)
	namespacedTool.Name = namespacedName
	namespacedTool.Description = fmt.Sprintf("[%s] %s", serverName, tool.Description)

	// 3. attach tool at server
	server.AddTool(namespacedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleToolCall(ctx, session, serverName, req)
	})
}

func registerToolsForSession(server *mcp.Server, session *CentianSession) {
	for serverName, conn := range session.downstreamConns {
		for _, tool := range conn.Tools() {
			RegisterToolAtServer(serverName, server, tool, session)
		}
	}
}

func handleToolCall(
	ctx context.Context,
	session *CentianSession,
	serverName string,
	req *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	// 1. get downstream connection from session
	conn, exists := session.downstreamConns[serverName]
	if !exists || !conn.IsConnected() {
		return nil, fmt.Errorf("server %s not available", serverName)
	}

	// 2. unmarshal tool args
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	// TODO: validate args based on tool
	// TODO: logging and processing -> this should likely not be on the
	// gateway but instead on the server wrapper!

	// 3. call downstream tool
	return conn.CallTool(ctx, req.Params.Name, args)
}

func (ag *CentianProxyGateway) Endpoint() string {
	return ag.endpoint
}

func (ag *CentianProxyGateway) Close() error {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	for _, session := range ag.sessions {
		for _, conn := range session.downstreamConns {
			conn.Close()
		}
	}
	return nil
}
