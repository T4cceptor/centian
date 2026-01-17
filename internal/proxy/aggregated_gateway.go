package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const NamespaceSeparator = "___"

type AggregatedGateway struct {
	name          string
	gatewayConfig *config.GatewayConfig
	server        *CentianServer // Parent server reference
	endpoint      string         // e.g., "/mcp/default"

	// Downstream connections (created but not connected until init)
	downstreams map[string]*DownstreamConnection

	// Tool registry: namespacedTool → serverName
	toolRegistry map[string]string

	// Session management: upstreamSessionID → *AggregatedSession
	sessions map[string]*AggregatedSession

	mu sync.RWMutex
}

type AggregatedSession struct {
	id              string
	initialized     bool
	downstreamConns map[string]*DownstreamConnection // Per-session connections
	authHeaders     map[string]string                // Captured from upstream init
}

// NewAggregatedGateway creates an aggregated gateway (connections are lazy)
func NewAggregatedGateway(
	name string,
	cfg *config.GatewayConfig,
	parentServer *CentianServer,
) *AggregatedGateway {
	endpoint := fmt.Sprintf("/mcp/%s", name)
	ag := &AggregatedGateway{
		name:          name,
		gatewayConfig: cfg,
		server:        parentServer,
		endpoint:      endpoint,
		downstreams:   make(map[string]*DownstreamConnection),
		toolRegistry:  make(map[string]string),
		sessions:      make(map[string]*AggregatedSession),
	}
	// Pre-create downstream connection wrappers (not connected yet)
	for serverName, serverCfg := range cfg.MCPServers {
		if serverCfg.Enabled {
			ag.downstreams[serverName] = NewDownstreamConnection(serverName, serverCfg)
		}
	}
	return ag
}

// RegisterHandler registers the aggregated endpoint with the HTTP mux
func (ag *AggregatedGateway) RegisterHandler(mux *http.ServeMux) {
	handler := mcp.NewStreamableHTTPHandler(
		ag.getServerForRequest,
		&mcp.StreamableHTTPOptions{
			SessionTimeout: 10 * time.Minute,
			Stateless:      false,
		},
	)

	mux.Handle(ag.endpoint, handler)
	log.Printf("Registered aggregated gateway at %s", ag.endpoint)
}

// getServerForRequest returns (or creates) an MCP server for this session
func (ag *AggregatedGateway) getServerForRequest(r *http.Request) *mcp.Server {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = getNewUUIDV7()
	}

	ag.mu.Lock()
	defer ag.mu.Unlock()

	session, exists := ag.sessions[sessionID]
	if !exists {
		session = ag.createSession(sessionID, r)
		ag.sessions[sessionID] = session
	}

	return ag.createServerForSession(session)
}

func (ag *AggregatedGateway) createSession(id string, r *http.Request) *AggregatedSession {
	authHeaders := make(map[string]string)

	// Common auth headers to passthrough
	// TODO: make this configurable
	for _, h := range []string{"Authorization", "X-API-Key", "X-Auth-Token"} {
		if v := r.Header.Get(h); v != "" {
			authHeaders[h] = v
		}
	}

	return &AggregatedSession{
		id:              id,
		initialized:     false,
		downstreamConns: make(map[string]*DownstreamConnection),
		authHeaders:     authHeaders,
	}
}

func (ag *AggregatedGateway) createServerForSession(session *AggregatedSession) *mcp.Server {
	var server *mcp.Server
	server = mcp.NewServer(&mcp.Implementation{
		Name:    fmt.Sprintf("centian-gateway-%s", ag.name),
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			ag.handleInitialize(ctx, server, session, req)
		},
	})

	if session.initialized {
		ag.registerToolsForSession(server, session)
	}

	return server
}

// handleInitialize - called when upstream client sends initialize
func (ag *AggregatedGateway) handleInitialize(
	ctx context.Context,
	server *mcp.Server,
	session *AggregatedSession,
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

			for _, tool := range conn.Tools() {
				// TODO: double check if this is appropriate / correct
				// if this works as expected, we might want to remove all
				// other occurrences of RegisterToolAtServer
				ag.RegisterToolAtServer(serverName, server, tool, session)
			}
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

func (ag *AggregatedGateway) buildInitializeResult(session *AggregatedSession) *mcp.InitializeResult {
	return &mcp.InitializeResult{
		ProtocolVersion: "2025-11-25",
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
		ServerInfo: &mcp.Implementation{
			Name:    fmt.Sprintf("centian-gateway-%s", ag.name),
			Version: "1.0.0",
		},
	}
}

func (ag *AggregatedGateway) RegisterToolAtServer(serverName string, server *mcp.Server, tool *mcp.Tool, session *AggregatedSession) {
	namespacedName := fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
	originalName := tool.Name
	sName := serverName

	namespacedTool := &mcp.Tool{
		Name:        namespacedName,
		Description: fmt.Sprintf("[%s] %s", serverName, tool.Description),
		InputSchema: tool.InputSchema,
	}

	server.AddTool(namespacedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return ag.handleToolCall(ctx, session, sName, originalName, req)
	})
}

func (ag *AggregatedGateway) registerToolsForSession(server *mcp.Server, session *AggregatedSession) {
	for serverName, conn := range session.downstreamConns {
		for _, tool := range conn.Tools() {
			ag.RegisterToolAtServer(serverName, server, tool, session)
		}
	}
}

func (ag *AggregatedGateway) handleToolCall(
	ctx context.Context,
	session *AggregatedSession,
	serverName string,
	toolName string,
	req *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	conn, exists := session.downstreamConns[serverName]
	if !exists || !conn.IsConnected() {
		return nil, fmt.Errorf("server %s not available", serverName)
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	// TODO: validate args based on tool
	// TODO: logging and processing -> this should likely not be on the
	// gateway but instead on the server wrapper!
	return conn.CallTool(ctx, toolName, args)
}

func (ag *AggregatedGateway) Endpoint() string {
	return ag.endpoint
}

func (ag *AggregatedGateway) Close() error {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	for _, session := range ag.sessions {
		for _, conn := range session.downstreamConns {
			conn.Close()
		}
	}
	return nil
}
