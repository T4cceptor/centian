package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	proxyToolNamespace     = "centian."
	authStatusToolName     = "centian.auth_status"
	loginToolPrefix        = "centian.login."
	testNotificationsTool  = "centian.test_notifications"
	authStateConnected     = "connected"
	authStateRequired      = "auth_required"
	authStateInProgress    = "auth_in_progress"
	authStateRefreshFailed = "refresh_failed"
	authStateUnavailable   = "unavailable"
)

type authToolState struct {
	Server    string `json:"server"`
	State     string `json:"state"`
	Message   string `json:"message"`
	LoginTool string `json:"loginTool,omitempty"`
	StartURL  string `json:"startUrl,omitempty"`
	StatusURL string `json:"statusUrl,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

func (p *CentianEndpoint) hasOAuthDownstreams() bool {
	for _, cfg := range p.GetActiveMCPServerConfigs() {
		if cfg != nil && cfg.OAuthEnabled() {
			return true
		}
	}
	return false
}

func loginToolName(serverName string) string {
	return loginToolPrefix + serverName
}

func isProxyToolName(name string) bool {
	return strings.HasPrefix(name, proxyToolNamespace)
}

func (p *CentianEndpoint) testToolsEnabled() bool {
	// TODO: this method should be moved to the server level -> makes MUCH more sense
	return p != nil &&
		p.server != nil &&
		p.server.Config != nil &&
		p.server.Config.Proxy != nil &&
		p.server.Config.Proxy.EnableTestTools
}

func (p *CentianEndpoint) registerStaticProxyTools(session *UpstreamSession, server *mcp.Server) {
	if session == nil || server == nil {
		return
	}
	if session.registeredStaticTools == nil {
		session.registeredStaticTools = make(map[string]struct{})
	}
	if p.hasOAuthDownstreams() {
		if _, exists := session.registeredStaticTools[authStatusToolName]; !exists {
			server.AddTool(&mcp.Tool{
				Name:        authStatusToolName,
				Description: "Show downstream OAuth connection state for this Centian endpoint.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return p.handleAuthStatusTool(ctx, session, req)
			})
			session.registeredStaticTools[authStatusToolName] = struct{}{}
		}
	}
	if !p.testToolsEnabled() {
		return
	}
	if _, exists := session.registeredStaticTools[testNotificationsTool]; exists {
		return
	}
	server.AddTool(&mcp.Tool{
		Name:        testNotificationsTool,
		Description: "Emit test log notifications on a timer for this session.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intervalSeconds": map[string]any{
					"type":        "number",
					"description": "Seconds between notifications. Defaults to 10.",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of notifications to emit. Defaults to 6.",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleTestNotificationsTool(ctx, session, req)
	})
	session.registeredStaticTools[testNotificationsTool] = struct{}{}
}

func (p *CentianEndpoint) desiredToolState(session *UpstreamSession) (map[string]*mcp.Tool, map[string]string) {
	desired := make(map[string]*mcp.Tool)
	toolServers := make(map[string]string)
	authStates := p.buildAuthToolStates(session)

	for serverName, conn := range session.downstreamConns {
		if !conn.IsConnected() {
			continue
		}
		for _, tool := range conn.Tools() {
			if tool == nil {
				continue
			}
			if isProxyToolName(tool.Name) {
				common.LogWarn("ProxyEndpoint[%s]: skipping downstream tool %q on %s; centian.* is reserved", p.name, tool.Name, serverName)
				continue
			}
			upstreamName := tool.Name
			if p.isAggregatedProxy {
				upstreamName = fmt.Sprintf("%s%s%s", serverName, NamespaceSeparator, tool.Name)
			}
			desired[upstreamName] = tool
			toolServers[upstreamName] = serverName
		}
	}

	for _, state := range authStates {
		if state.LoginTool == "" {
			continue
		}
		desired[state.LoginTool] = &mcp.Tool{
			Name:        state.LoginTool,
			Description: fmt.Sprintf("Start or resume login for downstream %s.", state.Server),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		}
		toolServers[state.LoginTool] = state.Server
	}

	return desired, toolServers
}

func (p *CentianEndpoint) currentUpstreamToolNames(session *UpstreamSession) []string {
	if session == nil {
		return nil
	}

	names := make([]string, 0, len(session.registeredStaticTools)+len(session.registeredTools))
	for name := range session.registeredStaticTools {
		names = append(names, name)
	}
	for name := range session.registeredTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *CentianEndpoint) buildAuthToolStates(session *UpstreamSession) []authToolState {
	serverNames := make([]string, 0)
	for serverName, cfg := range p.GetActiveMCPServerConfigs() {
		if cfg != nil && cfg.OAuthEnabled() {
			serverNames = append(serverNames, serverName)
		}
	}
	sort.Strings(serverNames)

	states := make([]authToolState, 0, len(serverNames))
	gateway := getGatewayFromPath(p.endpoint)
	var pool *DownstreamSessionPool

	p.mu.RLock()
	if session != nil && session.downstreamSessionKey != "" {
		pool = p.downstreamPools[session.downstreamSessionKey]
	}
	p.mu.RUnlock()

	for _, serverName := range serverNames {
		binding := centoauth.Binding{
			PrincipalID: session.identityKey,
			Gateway:     gateway,
			Server:      serverName,
		}
		var pending *centoauth.PendingAuthorization
		if p.server != nil && p.server.OAuth != nil {
			pending = p.server.OAuth.PendingForBinding(binding)
		}

		var conn DownstreamConnectionInterface
		if pool != nil {
			conn = pool.downstreamConns[serverName]
		}
		states = append(states, p.authStateForServer(serverName, conn, pending))
	}

	return states
}

func (p *CentianEndpoint) authStateForServer(
	serverName string,
	conn DownstreamConnectionInterface,
	pending *centoauth.PendingAuthorization,
) authToolState {
	state := p.baseAuthToolState(serverName, conn, pending)
	if connectedAuthToolState(&state, conn) {
		return state
	}
	applyPendingAuthToolState(&state, pending)
	applyConnectionAuthToolState(&state, conn)
	if state.State == authStateUnavailable {
		state.Message = fmt.Sprintf("Downstream %s is currently unavailable.", serverName)
	}
	return state
}

func (p *CentianEndpoint) baseAuthToolState(
	serverName string,
	conn DownstreamConnectionInterface,
	pending *centoauth.PendingAuthorization,
) authToolState {
	state := authToolState{
		Server: serverName,
		State:  authStateUnavailable,
	}
	if pending != nil {
		state.StartURL = p.server.OAuth.StartURL(pending.ID)
		state.StatusURL = p.server.OAuth.StatusURL(pending.ID)
		state.LastError = pending.LastError
	}
	if conn != nil && conn.GetError() != nil && state.LastError == "" {
		state.LastError = conn.GetError().Error()
	}
	return state
}

func connectedAuthToolState(state *authToolState, conn DownstreamConnectionInterface) bool {
	if state == nil || conn == nil || !conn.GetStatus().IsConnected() {
		return false
	}
	state.State = authStateConnected
	state.Message = fmt.Sprintf("Downstream %s is connected.", state.Server)
	return true
}

func applyPendingAuthToolState(state *authToolState, pending *centoauth.PendingAuthorization) {
	if state == nil || pending == nil {
		return
	}
	switch pending.Status {
	case centoauth.PendingStatusInProgress, centoauth.PendingStatusCompleted:
		state.State = authStateInProgress
		state.LoginTool = loginToolName(state.Server)
		state.Message = fmt.Sprintf("Login for downstream %s is in progress.", state.Server)
	case centoauth.PendingStatusReady:
		state.State = authStateRequired
		state.LoginTool = loginToolName(state.Server)
		state.Message = fmt.Sprintf("Downstream %s requires login before its tools can be used.", state.Server)
	}
}

func applyConnectionAuthToolState(state *authToolState, conn DownstreamConnectionInterface) {
	if state == nil || conn == nil || state.State != authStateUnavailable {
		return
	}
	switch {
	case conn.GetStatus().IsRefreshFailed():
		state.State = authStateRefreshFailed
		state.LoginTool = loginToolName(state.Server)
		state.Message = fmt.Sprintf("Downstream %s needs login again because token refresh failed.", state.Server)
	case conn.GetStatus().IsAuthRequired():
		state.State = authStateRequired
		state.LoginTool = loginToolName(state.Server)
		state.Message = fmt.Sprintf("Downstream %s requires login before its tools can be used.", state.Server)
	}
}

func (p *CentianEndpoint) handleAuthStatusTool(ctx context.Context, session *UpstreamSession, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if session != nil && session.rootsDirty {
		p.syncUpstreamSessionState(ctx, session.id)
	}
	states := p.buildAuthToolStates(session)
	structured := make([]map[string]any, 0, len(states))
	lines := make([]string, 0, len(states))
	for _, state := range states {
		structured = append(structured, map[string]any{
			"server":    state.Server,
			"state":     state.State,
			"message":   state.Message,
			"loginTool": state.LoginTool,
			"startUrl":  state.StartURL,
			"statusUrl": state.StatusURL,
			"lastError": state.LastError,
		})
		line := fmt.Sprintf("%s: %s", state.Server, state.State)
		if state.LoginTool != "" {
			line += " via " + state.LoginTool
		}
		lines = append(lines, line)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: strings.Join(lines, "\n")},
		},
		StructuredContent: map[string]any{"servers": structured},
	}, nil
}

func (p *CentianEndpoint) registerLoginTool(session *UpstreamSession, serverName string) {
	if session == nil || session.upstreamServer == nil {
		return
	}
	name := loginToolName(serverName)
	if _, exists := session.registeredTools[name]; exists {
		return
	}
	session.registeredTools[name] = struct{}{}
	session.upstreamServer.AddTool(&mcp.Tool{
		Name:        name,
		Description: fmt.Sprintf("Start or resume login for downstream %s.", serverName),
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return p.handleLoginTool(ctx, session, serverName, req)
	})
}

func (p *CentianEndpoint) ensurePendingAuthFlow(session *UpstreamSession, serverName string) (*centoauth.PendingAuthorization, error) {
	if p == nil || p.server == nil || p.server.OAuth == nil || session == nil {
		return nil, fmt.Errorf("oauth manager not available")
	}
	binding := centoauth.Binding{
		PrincipalID: session.identityKey,
		Gateway:     getGatewayFromPath(p.endpoint),
		Server:      serverName,
	}
	if pending := p.server.OAuth.PendingForBinding(binding); pending != nil {
		if pending.Status == centoauth.PendingStatusFailed {
			refreshed, _, err := p.server.OAuth.EnsurePending(
				binding,
				pending.ClientID,
				pending.ClientSecret,
				&pending.Metadata,
			)
			return refreshed, err
		}
		return pending, nil
	}
	return nil, fmt.Errorf("oauth flow for %s is not ready yet; wait for the downstream challenge and retry", serverName)
}

func (p *CentianEndpoint) promptForLogin(ctx context.Context, session *UpstreamSession, pending *centoauth.PendingAuthorization) (bool, string) {
	if session == nil || pending == nil {
		return false, ""
	}
	serverSession := p.currentUpstreamServerSession(session)
	if !supportsURLElicitation(serverSession) {
		return false, ""
	}
	result, err := serverSession.Elicit(ctx, &mcp.ElicitParams{
		Mode:          "url",
		Message:       fmt.Sprintf("Login is required for downstream %s. Open the Centian login page to continue.", pending.Binding.Server),
		URL:           p.server.OAuth.StartURL(pending.ID),
		ElicitationID: pending.ID,
	})
	if err != nil {
		common.LogWarn("ProxyEndpoint[%s]: failed to send URL elicitation for %s: %v", p.name, pending.Binding.Server, err)
		return false, ""
	}
	if result == nil {
		return true, ""
	}
	return true, result.Action
}

func supportsURLElicitation(serverSession *mcp.ServerSession) bool {
	if serverSession == nil || serverSession.InitializeParams() == nil || serverSession.InitializeParams().Capabilities == nil {
		return false
	}
	caps := serverSession.InitializeParams().Capabilities.Elicitation
	return caps != nil && caps.URL != nil
}

func (p *CentianEndpoint) handleLoginTool(
	ctx context.Context,
	session *UpstreamSession,
	serverName string,
	_ *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if session != nil && session.rootsDirty {
		p.syncUpstreamSessionState(ctx, session.id)
	}

	pending, err := p.ensurePendingAuthFlow(session, serverName)
	if err != nil {
		return nil, err
	}
	prompted, action := p.promptForLogin(ctx, session, pending)
	startURL := p.server.OAuth.StartURL(pending.ID)
	statusURL := p.server.OAuth.StatusURL(pending.ID)
	message := fmt.Sprintf("Open %s to log in to downstream %s.", startURL, serverName)
	if prompted {
		message = fmt.Sprintf("Login prompt sent for downstream %s. If your client did not open it, visit %s.", serverName, startURL)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		StructuredContent: map[string]any{
			"server":    serverName,
			"state":     pending.Status,
			"startUrl":  startURL,
			"statusUrl": statusURL,
			"prompted":  prompted,
			"action":    action,
		},
	}, nil
}

func (p *CentianEndpoint) formatAuthRequiredMessage(serverName string, authErr *centoauth.AuthorizationRequiredError) string {
	if authErr == nil {
		return fmt.Sprintf("Downstream %s requires login. Use %s.", serverName, loginToolName(serverName))
	}
	return fmt.Sprintf(
		"Downstream %s requires login. Use %s or open %s.",
		serverName,
		loginToolName(serverName),
		authErr.AuthURL,
	)
}

type testNotificationArgs struct {
	IntervalSeconds float64 `json:"intervalSeconds"`
	Count           int     `json:"count"`
}

func (p *CentianEndpoint) handleTestNotificationsTool(
	_ context.Context,
	session *UpstreamSession,
	req *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if session == nil {
		return nil, fmt.Errorf("upstream session not available")
	}
	serverSession := p.currentUpstreamServerSession(session)
	if serverSession == nil {
		return nil, fmt.Errorf("upstream server session not available")
	}

	args := testNotificationArgs{
		IntervalSeconds: 10,
		Count:           6,
	}
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.IntervalSeconds <= 0 {
		return nil, fmt.Errorf("intervalSeconds must be greater than zero")
	}
	if args.Count <= 0 {
		return nil, fmt.Errorf("count must be greater than zero")
	}

	sessionKey := serverSession.ID()
	if sessionKey == "" {
		sessionKey = session.id
	}
	p.startTestNotifications(sessionKey, session, time.Duration(args.IntervalSeconds*float64(time.Second)), args.Count)

	message := fmt.Sprintf("Started %d test notifications every %.2f seconds for this session.", args.Count, args.IntervalSeconds)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		StructuredContent: map[string]any{
			"started":         true,
			"intervalSeconds": args.IntervalSeconds,
			"count":           args.Count,
		},
	}, nil
}

func (p *CentianEndpoint) startTestNotifications(
	sessionKey string,
	session *UpstreamSession,
	interval time.Duration,
	count int,
) {
	if p == nil || session == nil {
		return
	}

	runCtx, job := p.swapNotificationJob(sessionKey)
	go p.runNotificationJob(runCtx, sessionKey, session, interval, count, job.id, job.cancel)
}

func (p *CentianEndpoint) swapNotificationJob(sessionKey string) (context.Context, notificationJob) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.notificationJobs == nil {
		p.notificationJobs = make(map[string]notificationJob)
	}
	if job, ok := p.notificationJobs[sessionKey]; ok && job.cancel != nil {
		job.cancel()
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	runCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // The cancel func is stored in the active job map and is called on replacement or goroutine exit.
	job := notificationJob{id: jobID, cancel: cancel}
	p.notificationJobs[sessionKey] = job
	return runCtx, job
}

func (p *CentianEndpoint) runNotificationJob(
	runCtx context.Context,
	sessionKey string,
	session *UpstreamSession,
	interval time.Duration,
	count int,
	jobID string,
	cancel context.CancelFunc,
) {
	defer cancel()
	defer p.clearNotificationJob(sessionKey, jobID)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for i := 1; i <= count; i++ {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			if err := p.logUpstreamSession(runCtx, session, &mcp.LoggingMessageParams{
				Level: logLevelInfo,
				Data:  fmt.Sprintf("centian test notification %d/%d", i, count),
			}); err != nil {
				common.LogWarn("ProxyEndpoint[%s]: failed to emit test notification for session %s: %v", p.name, sanitizeLogValue(sessionKey), err)
				return
			}
		}
	}
}

func (p *CentianEndpoint) clearNotificationJob(sessionKey, jobID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if current, ok := p.notificationJobs[sessionKey]; ok && current.id == jobID {
		delete(p.notificationJobs, sessionKey)
	}
}

func (p *CentianEndpoint) handleDownstreamToolAuthorizationRequired(
	ctx context.Context,
	session *UpstreamSession,
	serverName string,
	authErr *centoauth.AuthorizationRequiredError,
) error {
	if session == nil {
		return authErr
	}

	p.toolRegMu.Lock()
	p.syncAvailableTools(session)
	p.toolRegMu.Unlock()

	if authErr != nil {
		p.notifyOAuthRequired(authErr.Binding, authErr.AuthURL)
		if pending := p.server.OAuth.PendingForBinding(authErr.Binding); pending != nil {
			p.promptForLogin(ctx, session, pending)
		}
	}

	return fmt.Errorf("%s", p.formatAuthRequiredMessage(serverName, authErr))
}
