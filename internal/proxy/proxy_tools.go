package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/T4cceptor/centian/internal/taskverification"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file registers upstream tool surfaces and routes proxied tool calls to
// downstream servers.

type toolRoute struct {
	serverName   string
	originalName string
	exposedName  string
}

type toolStateEntry struct {
	tool                  *mcp.Tool
	serverName            string
	originalName          string
	defaultExposedName    string
	definitionFingerprint string
}

// newUpstreamServer returns a new upstream-facing MCP server for the given session.
// The session is captured by the handler closures so that forwarding functions can
// look up downstream connections at call time.
func (p *CentianEndpoint) newUpstreamServer(session *UpstreamSession) *mcp.Server {
	serverName := "centian-proxy-" + p.name
	if p.isAggregatedProxy {
		serverName = "centian-gateway-" + p.name
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: p.server.Config.Version,
	}, &mcp.ServerOptions{
		// HasResources and HasPrompts tell the SDK to advertise these surfaces even
		// before any individual resources or prompts have been registered (they are
		// registered dynamically after downstream connections are established).
		HasResources: true,
		HasPrompts:   true,
		Capabilities: &mcp.ServerCapabilities{
			Tools:       &mcp.ToolCapabilities{ListChanged: true},
			Logging:     &mcp.LoggingCapabilities{},
			Completions: &mcp.CompletionCapabilities{},
		},
		// SubscribeHandler non-nil causes the SDK to add resources.subscribe to capabilities.
		// Both Subscribe and Unsubscribe must be set or unset together.
		SubscribeHandler: func(ctx context.Context, req *mcp.SubscribeRequest) error {
			return p.forwardSubscribe(ctx, session, req)
		},
		UnsubscribeHandler: func(ctx context.Context, req *mcp.UnsubscribeRequest) error {
			return p.forwardUnsubscribe(ctx, session, req)
		},
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return p.forwardCompletion(ctx, session, req)
		},
		GetSessionID: func() string {
			return session.id
		},
	})
	// TODO: refactor registerStaticProxyTools into 2 functions:
	// 1. for auth tools
	// 2. for test notifications (they should be separated)
	// then call both from here
	// and move registerStaticProxyTools away from CentianEndpoint
	// it should be a session.upstreamServer function actually
	p.registerStaticProxyTools(session, server)
	return server
}

func (p *CentianEndpoint) syncAvailableTools(session *UpstreamSession) {
	if err := p.syncAvailableToolsChecked(session); err != nil {
		common.LogError("ProxyEndpoint[%s]: failed to sync tool surface for session %s: %v", p.name, sanitizeLogValue(sessionIDForLog(session)), err)
	}
}

func (p *CentianEndpoint) syncAvailableToolsChecked(session *UpstreamSession) error {
	if session == nil || session.upstreamServer == nil {
		return nil
	}
	if session.registeredTools == nil {
		session.registeredTools = make(map[string]struct{})
	}
	if session.registeredToolFingerprints == nil {
		session.registeredToolFingerprints = make(map[string]string)
	}
	if session.toolRoutes == nil {
		session.toolRoutes = make(map[string]toolRoute)
	}

	desiredTools, err := p.desiredToolState(session)
	if err != nil {
		return err
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
			delete(session.registeredToolFingerprints, toolName)
			delete(session.toolRoutes, toolName)
		}
	}

	for upstreamName, entry := range desiredTools {
		nextFingerprint := fingerprintRegisteredTool(entry.tool)
		if _, ok := session.registeredTools[upstreamName]; ok && session.registeredToolFingerprints[upstreamName] == nextFingerprint {
			continue
		}
		if strings.HasPrefix(upstreamName, loginToolPrefix) {
			p.registerLoginTool(session, entry.serverName)
			session.registeredToolFingerprints[upstreamName] = nextFingerprint
			continue
		}
		if _, ok := session.registeredTools[upstreamName]; ok {
			session.upstreamServer.RemoveTools(upstreamName)
			delete(session.registeredTools, upstreamName)
			delete(session.registeredToolFingerprints, upstreamName)
			delete(session.toolRoutes, upstreamName)
		}
		p.registerTool(session, entry)
		session.registeredToolFingerprints[upstreamName] = nextFingerprint
	}
	return nil
}

func (p *CentianEndpoint) registerAvailableTools(session *UpstreamSession) {
	if session == nil {
		return
	}
	downstreamSessionKey := p.sessionDownstreamSessionKey(session)

	p.mu.RLock()
	pool := p.downstreamPools[downstreamSessionKey]
	p.mu.RUnlock()

	if pool == nil {
		return
	}

	totalDownstreams := len(pool.downstreamConns)
	if totalDownstreams == 0 {
		common.LogWarn("ProxyEndpoint[%s]: no downstream servers available for session %s", p.name, downstreamSessionKey)
		return
	}

	summary := downstreamRegistrationSummary{}
	p.mu.RLock()
	for serverName, conn := range pool.downstreamConns {
		p.collectDownstreamToolState(pool, serverName, conn, &summary)
	}
	p.mu.RUnlock()

	p.toolRegMu.Lock()
	if err := p.syncAvailableToolsChecked(session); err != nil {
		common.LogError("ProxyEndpoint[%s]: failed to sync tool surface for session %s: %v", p.name, sanitizeLogValue(session.id), err)
	}
	p.toolRegMu.Unlock()
	p.notifySessionOAuthRequirements(session, pool)

	switch {
	case summary.connectedCount > 0:
		common.LogInfo(
			"ProxyEndpoint[%s]: upstream session %s using pooled downstream session %s with %d/%d connected servers",
			p.name,
			session.id,
			downstreamSessionKey,
			summary.connectedCount,
			totalDownstreams,
		)
	case summary.connectingCount > 0:
		common.LogInfo(
			"ProxyEndpoint[%s]: initializing pooled downstream session %s in background (%d/%d still connecting)",
			p.name,
			downstreamSessionKey,
			summary.connectingCount,
			totalDownstreams,
		)
	case len(summary.connErrors) > 0:
		common.LogError("ProxyEndpoint[%s]: all connections failed: %v", p.name, summary.connErrors)
	default:
		common.LogWarn("ProxyEndpoint[%s]: no pooled downstream connections are connected for %s", p.name, downstreamSessionKey)
	}
}

func (p *CentianEndpoint) notifySessionOAuthRequirements(session *UpstreamSession, pool *DownstreamSessionPool) {
	if session == nil || pool == nil {
		return
	}

	for _, conn := range pool.downstreamConns {
		if conn == nil || !conn.GetStatus().IsAuthRequired() {
			continue
		}
		authErr, ok := centoauth.IsAuthorizationRequired(conn.GetError())
		if !ok || authErr.Binding.PrincipalID != session.identityKey {
			continue
		}
		if err := p.logUpstreamSession(context.Background(), session, &mcp.LoggingMessageParams{
			Level: logLevelInfo,
			Data: fmt.Sprintf(
				"OAuth required for downstream %s/%s. Use %s or open %s",
				authErr.Binding.Gateway,
				authErr.Binding.Server,
				loginToolName(authErr.Binding.Server),
				authErr.AuthURL,
			),
		}); err != nil {
			common.LogWarn("ProxyEndpoint[%s]: failed to log downstream oauth requirement for session %s: %v", p.name, session.id, err)
		}
	}
}

type downstreamRegistrationSummary struct {
	connectedCount  int
	connectingCount int
	connErrors      []string
}

func (p *CentianEndpoint) collectDownstreamToolState(
	pool *DownstreamSessionPool,
	serverName string,
	conn DownstreamConnectionInterface,
	summary *downstreamRegistrationSummary,
) {
	if pool.HasActiveConnectWorker(serverName) {
		summary.connectingCount++
		return
	}
	if conn.IsConnected() {
		summary.connectedCount++
		return
	}
	if conn.IsConnecting() || conn.IsPending() {
		summary.connectingCount++
		return
	}
	if conn.IsFailed() && conn.GetError() != nil {
		summary.connErrors = append(summary.connErrors, fmt.Sprintf("%s: %v", serverName, conn.GetError()))
	}
}

// registerTool adds one downstream tool to one upstream-facing server instance.
func (p *CentianEndpoint) registerTool(session *UpstreamSession, entry toolStateEntry) {
	server := session.upstreamServer
	if session.registeredTools == nil {
		session.registeredTools = make(map[string]struct{})
	}
	if session.toolRoutes == nil {
		session.toolRoutes = make(map[string]toolRoute)
	}
	tool := entry.tool
	if tool == nil {
		return
	}
	if isProxyToolName(tool.Name) {
		common.LogWarn("ProxyEndpoint[%s]: skipping downstream tool %q on %s; centian.* is reserved", p.name, tool.Name, entry.serverName)
		return
	}

	clonedTool := copyToolForRegistration(tool)
	applyConfiguredToolHintOverrides(clonedTool, p.config)

	if _, exists := session.registeredTools[clonedTool.Name]; exists {
		return
	}
	session.registeredTools[clonedTool.Name] = struct{}{}
	session.toolRoutes[clonedTool.Name] = toolRoute{
		serverName:   entry.serverName,
		originalName: entry.originalName,
		exposedName:  clonedTool.Name,
	}

	server.AddTool(clonedTool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleToolCall(ctx, session, entry.serverName, req)
	})
}

func sessionIDForLog(session *UpstreamSession) string {
	if session == nil {
		return ""
	}
	return session.id
}

func (p *CentianEndpoint) logToolSurfaceAnnotations(session *UpstreamSession, serverName string, annotations []common.EventAnnotation) {
	if len(annotations) == 0 {
		return
	}
	logger := p.projectLogger()
	if logger == nil {
		return
	}
	meta := common.NewMetaContext("surface", common.DirectionSystem, common.MessageTypeSystem).
		WithRequestID(getNewUUIDV7()).
		WithSessionID(sessionIDForLog(session))
	if p.server != nil {
		meta.WithServerID(p.server.ServerID)
	}
	entry := &common.LogEntry{
		BaseMcpEvent: meta.BaseMcpEvent,
		Routing: common.RoutingContext{
			Gateway:    getGatewayFromPath(p.endpoint),
			ServerName: serverName,
			Endpoint:   p.endpoint,
		},
		Annotations: annotations,
	}
	entry.Timestamp = time.Now()
	entry.Success = true
	if err := logger.LogMcpEvent(entry); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: failed to log tool surface annotations: %v", p.name, err)
	}
}

// ProcessCall handles the request phase processing using handlers.
func (p *CentianEndpoint) ProcessCall(callCtx CallContext, direction common.McpEventDirection, msgType common.McpMessageType) error {
	if p.eventProcessor == nil {
		return nil
	}
	callCtx.SetDirection(direction)
	callCtx.SetMessageType(msgType)
	return p.eventProcessor.Process(callCtx)
}

func (p *CentianEndpoint) handleToolCall(ctx context.Context, session *UpstreamSession, serverName string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if p.sessionNeedsSync(session) {
		p.syncUpstreamSessionState(ctx, session.id)
	}

	callCtx, err := NewToolCallContext(p, session, serverName, req)
	if err != nil {
		return nil, err
	}
	ctx = WithCallContext(ctx, callCtx)
	common.LogInfo("Tool called: %s :: %s", callCtx.GetServerName(), callCtx.GetToolName())
	session.taskMu.Lock()
	p.maybeExpireActiveTaskLocked(session, callCtx.GetRequestID())
	activeRun := snapshotTaskRun(session.taskRun)
	if activeRun.Status == taskverification.TaskStatusActive {
		p.cancelTaskTimeoutLocked(session)
	}
	session.taskMu.Unlock()
	if activeRun.Status == taskverification.TaskStatusActive {
		p.recordTaskActionContext(activeRun.RunID, callCtx.GetRequestID(), activeRun.Phase, activeRun.NodeKind)
	}
	defer func() {
		session.taskMu.Lock()
		defer session.taskMu.Unlock()
		switch {
		case session.taskRun == nil:
			p.cancelTaskTimeoutLocked(session)
		case session.taskRun.Status == taskverification.TaskStatusActive:
			p.refreshTaskActivityLocked(session)
		default:
			p.cancelTaskTimeoutLocked(session)
			if session.taskRun.Status != taskverification.TaskStatusTimedOut {
				session.taskRun.ExpiresAt = 0
			}
		}
	}()
	if denied, blocked := p.enforceWorkflowNodeToolGovernance(session, callCtx); blocked {
		return denied, nil
	}

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
		if authErr, ok := centoauth.IsAuthorizationRequired(err); ok {
			return nil, p.handleDownstreamToolAuthorizationRequired(ctx, session, serverName, authErr)
		}
		p.invalidateDownstreamPool(p.sessionDownstreamSessionKey(session))
		return nil, err
	}
	// Call processing loop on result
	if err := p.ProcessCall(callCtx, common.DirectionServerToClient, common.MessageTypeResponse); err != nil {
		return nil, err
	}
	// Return result
	return callCtx.GetResult(), nil
}

func (p *CentianEndpoint) getSyncedSession(ctx context.Context) (*mcp.ServerSession, error) {
	session, serverSession, err := p.upstreamSessionFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if p.sessionRootsDirty(session) {
		p.syncUpstreamSessionState(ctx, session.id)
	}
	return serverSession, nil
}

func (p *CentianEndpoint) forwardSamplingRequest(ctx context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	serverSession, err := p.getSyncedSession(ctx)
	if err != nil {
		return nil, err
	}
	return serverSession.CreateMessage(ctx, req.Params)
}

func (p *CentianEndpoint) forwardElicitationRequest(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	serverSession, err := p.getSyncedSession(ctx)
	if err != nil {
		return nil, err
	}
	return serverSession.Elicit(ctx, req.Params)
}

func (p *CentianEndpoint) upstreamSessionFromContext(ctx context.Context) (*UpstreamSession, *mcp.ServerSession, error) {
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
