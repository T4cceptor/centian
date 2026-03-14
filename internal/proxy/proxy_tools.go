package proxy

import (
	"context"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newUpstreamServer returns a new upstream-facing MCP server.
func (p *MCPProxy) newUpstreamServer(sessionID string) *mcp.Server {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}

	return mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: p.server.Config.Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: true},
		},
		GetSessionID: func() string {
			return sessionID
		},
	})
}

func (p *MCPProxy) refreshAvailableTools(session *UpstreamSession) {
	if session == nil || session.upstreamServer == nil {
		return
	}
	if session.registeredTools == nil {
		session.registeredTools = make(map[string]struct{})
	}

	desiredTools := make(map[string]*mcp.Tool)
	toolServers := make(map[string]string)
	for serverName, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, tool := range conn.Tools() {
			upstreamName := tool.Name
			if p.isAggregatedProxy {
				upstreamName = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
			}
			desiredTools[upstreamName] = tool
			toolServers[upstreamName] = serverName
		}
	}

	staleTools := make([]string, 0)
	for toolName := range session.registeredTools {
		if _, ok := desiredTools[toolName]; !ok {
			staleTools = append(staleTools, toolName)
		}
	}
	if len(staleTools) > 0 {
		session.upstreamServer.RemoveTools(staleTools...)
		for _, toolName := range staleTools {
			delete(session.registeredTools, toolName)
		}
	}

	for upstreamName, tool := range desiredTools {
		if _, ok := session.registeredTools[upstreamName]; ok {
			continue
		}
		p.registerTool(session, toolServers[upstreamName], tool)
	}
}

func (p *MCPProxy) registerAvailableTools(session *UpstreamSession) {
	if session == nil {
		return
	}

	p.mu.RLock()
	pool := p.downstreamPools[session.downstreamSessionKey]
	p.mu.RUnlock()

	if pool == nil {
		return
	}

	totalDownstreams := len(pool.downstreamConns)
	if totalDownstreams == 0 {
		common.LogWarn("MCPProxy[%s]: No downstream servers available for session %s", p.name, session.downstreamSessionKey)
		return
	}

	summary := downstreamRegistrationSummary{}
	p.mu.RLock()
	for serverName, conn := range pool.downstreamConns {
		p.collectDownstreamToolState(pool, serverName, conn, &summary)
	}
	p.mu.RUnlock()

	p.toolRegMu.Lock()
	p.refreshAvailableTools(session)
	p.toolRegMu.Unlock()

	switch {
	case summary.connectedCount > 0:
		common.LogInfo(
			"MCPProxy[%s]: Upstream session %s using pooled downstream session %s with %d/%d connected servers",
			p.name,
			session.id,
			session.downstreamSessionKey,
			summary.connectedCount,
			totalDownstreams,
		)
	case summary.connectingCount > 0:
		common.LogInfo(
			"MCPProxy[%s]: Initializing pooled downstream session %s in background (%d/%d still connecting)",
			p.name,
			session.downstreamSessionKey,
			summary.connectingCount,
			totalDownstreams,
		)
	case len(summary.connErrors) > 0:
		common.LogError("MCPProxy[%s]: All connections failed: %v", p.name, summary.connErrors)
	default:
		common.LogWarn("MCPProxy[%s]: No pooled downstream connections are connected for %s", p.name, session.downstreamSessionKey)
	}
}

type downstreamRegistrationSummary struct {
	connectedCount  int
	connectingCount int
	connErrors      []string
}

func (p *MCPProxy) collectDownstreamToolState(
	pool *DownstreamSessionPool,
	serverName string,
	conn DownstreamConnectionInterface,
	summary *downstreamRegistrationSummary,
) {
	if pool.connecting[serverName] {
		summary.connectingCount++
		return
	}
	status := conn.GetStatus()
	if conn.IsConnected() {
		summary.connectedCount++
		return
	}
	if status == StatusConnecting || status == StatusPending {
		summary.connectingCount++
		return
	}
	if status == StatusFailed && conn.GetError() != nil {
		summary.connErrors = append(summary.connErrors, fmt.Sprintf("%s: %v", serverName, conn.GetError()))
	}
}

// registerTool adds one downstream tool to one upstream-facing server instance.
func (p *MCPProxy) registerTool(session *UpstreamSession, serverName string, tool *mcp.Tool) {
	server := session.upstreamServer
	if session.registeredTools == nil {
		session.registeredTools = make(map[string]struct{})
	}

	clonedTool := deepCloneTool(tool)
	toolServerName := serverName
	if p.isAggregatedProxy {
		clonedTool.Name = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
		clonedTool.Description = fmt.Sprintf("[%s] %s", serverName, tool.Description)
	}

	if _, exists := session.registeredTools[clonedTool.Name]; exists {
		return
	}
	session.registeredTools[clonedTool.Name] = struct{}{}

	server.AddTool(clonedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleToolCall(ctx, session, toolServerName, req)
	})
}

// ProcessCall handles the request phase processing using handlers.
func (p *MCPProxy) ProcessCall(callCtx CallContext, direction common.McpEventDirection, msgType common.McpMessageType) error {
	if p.eventProcessor == nil {
		return nil
	}

	callCtx.SetDirection(direction)
	callCtx.SetMessageType(msgType)
	return p.eventProcessor.Process(callCtx)
}

func (p *MCPProxy) handleToolCall(ctx context.Context, session *UpstreamSession, serverName string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if session.protocolVersion == "" || session.rootsDirty {
		if err := p.syncUpstreamSessionState(ctx, session.id); err != nil {
			common.LogWarn("MCPProxy[%s]: failed to synchronize client state for session %s: %v", p.name, session.id, err)
		}
	}

	callCtx, err := NewToolCallContext(p, session, serverName, req)
	if err != nil {
		return nil, err
	}
	ctx = WithCallContext(ctx, callCtx)
	common.LogInfo("Tool called: %s :: %s", callCtx.GetServerName(), callCtx.GetToolName())

	// Call processing loop on request
	if err := p.ProcessCall(callCtx, common.DirectionClientToServer, common.MessageTypeRequest); err != nil {
		return nil, err
	}
	// If processing loop returned a result we short-circuit and send it immediately
	// back to the client without executing the actual request downstream
	if callCtx.HasResult() {
		return callCtx.GetResult(), nil
	}
	// Send the actual downstream request
	if err := callCtx.SendRequest(ctx); err != nil {
		p.invalidateDownstreamPool(session.downstreamSessionKey)
		return nil, err
	}
	// Call processing loop on result
	if err := p.ProcessCall(callCtx, common.DirectionServerToClient, common.MessageTypeResponse); err != nil {
		return nil, err
	}
	// Return result
	return callCtx.GetResult(), nil
}

func (p *MCPProxy) forwardSamplingRequest(ctx context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	session, serverSession, err := p.upstreamSessionFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if session.rootsDirty {
		if syncErr := p.syncUpstreamSessionState(ctx, session.id); syncErr != nil {
			common.LogWarn("MCPProxy[%s]: failed to refresh client state before sampling for session %s: %v", p.name, session.id, syncErr)
		}
	}
	return serverSession.CreateMessage(ctx, req.Params)
}

func (p *MCPProxy) forwardElicitationRequest(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	session, serverSession, err := p.upstreamSessionFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if session.rootsDirty {
		if syncErr := p.syncUpstreamSessionState(ctx, session.id); syncErr != nil {
			common.LogWarn("MCPProxy[%s]: failed to refresh client state before elicitation for session %s: %v", p.name, session.id, syncErr)
		}
	}
	return serverSession.Elicit(ctx, req.Params)
}

func (p *MCPProxy) upstreamSessionFromContext(ctx context.Context) (*UpstreamSession, *mcp.ServerSession, error) {
	callCtx, ok := CallContextFromContext(ctx)
	if !ok {
		return nil, nil, fmt.Errorf("missing call context")
	}

	toolCallCtx, ok := callCtx.(*ToolCallContext)
	if !ok || toolCallCtx.upstreamSession == nil {
		return nil, nil, fmt.Errorf("missing upstream session in call context")
	}

	serverSession := p.currentUpstreamServerSession(toolCallCtx.upstreamSession)
	if serverSession == nil {
		return nil, nil, fmt.Errorf("upstream server session not available")
	}
	return toolCallCtx.upstreamSession, serverSession, nil
}
