package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
)

// This file manages reusable downstream connection pools keyed by effective
// upstream client state.

// buildDownstreamSessionKey creates the reuse key for a downstream session pool.
func (p *CentianEndpoint) buildDownstreamSessionKey(identityKey string, forwardedHeaders map[string]string, state *DownstreamClientState) string {
	if state == nil {
		state = &DownstreamClientState{}
	}
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s",
		p.endpoint,
		identityKey,
		authHeadersFingerprint(forwardedHeaders),
		state.CapabilitiesFingerprint,
		state.RootsFingerprint,
	)
}

// newDownstreamConnection creates one runtime downstream connection from config.
func (p *CentianEndpoint) newDownstreamConnection(serverName string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
	if p.connectionFactory != nil {
		return p.connectionFactory(serverName, cfg)
	}
	conn := NewDownstreamConnection(serverName, cfg)
	if p.server != nil {
		conn.oauthManager = p.server.OAuth
	}
	return conn
}

// buildDownstreamConnectOptions derives the connect options for one upstream session.
func (p *CentianEndpoint) buildDownstreamConnectOptions(session *UpstreamSession) *DownstreamConnectOptions {
	clientState := session.downstreamClientState()
	if clientState == nil {
		clientState = &DownstreamClientState{}
	}

	return &DownstreamConnectOptions{
		ForwardedHeaders:   session.GetAuthHeaders(p.excludedClientAuthHeader()),
		ClientState:        *clientState,
		IdentityKey:        session.identityKey,
		GatewayName:        getGatewayFromPath(p.endpoint),
		SamplingHandler:    p.forwardSamplingRequest,
		ElicitationHandler: p.forwardElicitationRequest,
	}
}

// bindSessionToPoolLocked wires one upstream session to the pool's shared connection set.
// Caller must hold p.mu.
func (p *CentianEndpoint) bindSessionToPoolLocked(session *UpstreamSession, pool *DownstreamSessionPool) {
	if session == nil || pool == nil {
		return
	}
	pool.lastUsed = time.Now()
	pool.upstreamSessions[session.id] = session
	session.downstreamConns = pool.downstreamConns
}

// getOrCreateSessionPoolLocked returns the matching downstream pool for the session,
// creating it when needed and starting any missing downstream connections.
// Caller must hold p.mu.
func (p *CentianEndpoint) getOrCreateSessionPoolLocked(session *UpstreamSession) (*DownstreamSessionPool, bool) {
	if session.downstreamSessionKey == "" {
		return nil, false
	}

	connectOptions := p.buildDownstreamConnectOptions(session)
	if existing, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
		p.bindSessionToPoolLocked(session, existing)
		p.startMissingPoolConnectionsLocked(existing, connectOptions)
		return existing, true
	}

	activeServerConfigs := p.GetActiveMCPServerConfigs()
	pool := &DownstreamSessionPool{
		identityKey:          session.identityKey,
		downstreamSessionKey: session.downstreamSessionKey,
		downstreamConns:      make(map[string]DownstreamConnectionInterface, len(activeServerConfigs)),
		upstreamSessions:     map[string]*UpstreamSession{session.id: session},
		connecting:           make(map[string]bool),
		lastUsed:             time.Now(),
	}
	p.downstreamPools[session.downstreamSessionKey] = pool
	p.bindSessionToPoolLocked(session, pool)
	p.startMissingPoolConnectionsLocked(pool, connectOptions)
	return pool, false
}

// startMissingPoolConnectionsLocked ensures the pool has connection objects for
// each active downstream server and starts any missing async connect attempts.
// Caller must hold p.mu.
func (p *CentianEndpoint) startMissingPoolConnectionsLocked(pool *DownstreamSessionPool, connectOptions *DownstreamConnectOptions) {
	for serverName, serverConfig := range p.GetActiveMCPServerConfigs() {
		conn, err := pool.GetConnectionByServerName(serverName)
		if err != nil || conn.GetStatus().IsFailed() || conn.GetStatus().IsAuthRequired() || conn.GetStatus().IsRefreshFailed() {
			conn = p.newDownstreamConnection(serverName, serverConfig)
			pool.SetConnection(serverName, conn)
		}
		isConnecting, _ := pool.IsConnecting(serverName)
		if isConnecting || conn.IsConnected() {
			continue
		}
		pool.connecting[serverName] = true
		go p.connectDownstreamPool(pool.downstreamSessionKey, conn, cloneDownstreamConnectOptions(connectOptions))
	}
}

// waitForFirstUsableDownstream waits briefly for at least one connected downstream with tools.
// A short poll loop is used here because downstream connections are established on goroutines
// that do not signal completion via a channel or condition variable. The timeout is kept small
// (500ms) so the upstream initialize response is not held up for long. A future improvement
// would be to signal readiness from connectDownstreamPool directly.
func (p *CentianEndpoint) waitForFirstUsableDownstream(pool *DownstreamSessionPool, timeout time.Duration) {
	if pool == nil {
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.downstreamPoolHasUsableConnection(pool) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// downstreamPoolHasUsableConnection reports whether the pool has at least one ready downstream.
func (p *CentianEndpoint) downstreamPoolHasUsableConnection(pool *DownstreamSessionPool) bool {
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

// connectDownstreamPool establishes one downstream connection owned by a reusable pool.
func (p *CentianEndpoint) connectDownstreamPool(
	downstreamSessionKey string,
	conn DownstreamConnectionInterface,
	connectOptions *DownstreamConnectOptions,
) {
	serverName := conn.GetServerName()
	options := p.poolConnectOptions(downstreamSessionKey, serverName, conn, connectOptions)
	if err := conn.Connect(context.Background(), options); err != nil {
		p.handlePoolConnectError(downstreamSessionKey, serverName, conn, err)
		return
	}

	p.syncPoolLoggingLevel(downstreamSessionKey)
	sessions, oauthEnabled := p.markPoolConnectionReady(downstreamSessionKey, serverName, conn)
	p.discoverDownstreamArtifacts(serverName, conn)
	p.syncPoolSessions(serverName, sessions, oauthEnabled)
	common.LogInfo("ProxyEndpoint[%s]: connected pooled downstream %s with %d tools, %d resources, %d resource templates, %d prompts",
		p.name, sanitizeLogValue(serverName), len(conn.Tools()), len(conn.Resources()), len(conn.ResourceTemplates()), len(conn.Prompts()))
}

func (p *CentianEndpoint) poolConnectOptions(
	downstreamSessionKey, serverName string,
	conn DownstreamConnectionInterface,
	connectOptions *DownstreamConnectOptions,
) *DownstreamConnectOptions {
	options := cloneDownstreamConnectOptions(connectOptions)
	options.LoggingHandler = p.newPoolLoggingHandler(downstreamSessionKey, serverName, conn)
	options.ResourceListChangedHandler = p.newPoolResourceListChangedHandler(downstreamSessionKey, serverName, conn)
	options.ResourceUpdatedHandler = p.newPoolResourceUpdatedHandler(downstreamSessionKey, serverName, conn)
	return options
}

func (p *CentianEndpoint) handlePoolConnectError(
	downstreamSessionKey, serverName string,
	conn DownstreamConnectionInterface,
	err error,
) {
	sessions := p.releasePoolConnection(downstreamSessionKey, serverName, conn)
	if authErr, ok := centoauth.IsAuthorizationRequired(err); ok {
		p.toolRegMu.Lock()
		for _, session := range sessions {
			p.syncAvailableTools(session)
		}
		p.toolRegMu.Unlock()
		common.LogInfo(
			"ProxyEndpoint[%s]: downstream %s requires OAuth authorization for %s via %s",
			p.name,
			serverName,
			authErr.Binding.PrincipalID,
			authErr.AuthURL,
		)
		return
	}
	common.LogWarn("ProxyEndpoint[%s]: failed to connect pooled downstream %s: %v", p.name, serverName, err)
}

func (p *CentianEndpoint) releasePoolConnection(
	downstreamSessionKey, serverName string,
	conn DownstreamConnectionInterface,
) []*UpstreamSession {
	var sessions []*UpstreamSession
	p.mu.Lock()
	defer p.mu.Unlock()

	if pool, ok := p.downstreamPools[downstreamSessionKey]; ok {
		if current, exists := pool.downstreamConns[serverName]; exists && current == conn {
			delete(pool.connecting, serverName)
			for _, session := range pool.upstreamSessions {
				sessions = append(sessions, session)
			}
		}
	}
	return sessions
}

func (p *CentianEndpoint) markPoolConnectionReady(
	downstreamSessionKey, serverName string,
	conn DownstreamConnectionInterface,
) ([]*UpstreamSession, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var sessions []*UpstreamSession
	oauthEnabled := false
	if pool, ok := p.downstreamPools[downstreamSessionKey]; ok {
		if current, exists := pool.downstreamConns[serverName]; exists && current == conn {
			delete(pool.connecting, serverName)
			pool.lastUsed = time.Now()
			for _, session := range pool.upstreamSessions {
				sessions = append(sessions, session)
			}
			if serverConfig := p.GetActiveMCPServerConfigs()[serverName]; serverConfig != nil {
				oauthEnabled = serverConfig.OAuthEnabled()
			}
		}
	}
	return sessions, oauthEnabled
}

func (p *CentianEndpoint) discoverDownstreamArtifacts(serverName string, conn DownstreamConnectionInterface) {
	if err := conn.DiscoverResources(context.Background()); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: resource discovery failed for %s: %v", p.name, sanitizeLogValue(serverName), err)
	}
	if err := conn.DiscoverResourceTemplates(context.Background()); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: resource template discovery failed for %s: %v", p.name, sanitizeLogValue(serverName), err)
	}
	if err := conn.DiscoverPrompts(context.Background()); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: prompt discovery failed for %s: %v", p.name, sanitizeLogValue(serverName), err)
	}
}

func (p *CentianEndpoint) syncPoolSessions(serverName string, sessions []*UpstreamSession, oauthEnabled bool) {
	p.toolRegMu.Lock()
	toolNamesBySession := make(map[*UpstreamSession][]string, len(sessions))
	for _, session := range sessions {
		p.syncAvailableTools(session)
		p.syncAvailableResources(session)
		p.syncAvailableResourceTemplates(session)
		p.syncAvailablePrompts(session)
		if oauthEnabled {
			toolNamesBySession[session] = p.currentUpstreamToolNames(session)
		}
	}
	p.toolRegMu.Unlock()

	if !oauthEnabled {
		return
	}
	for _, session := range sessions {
		p.notifyOAuthAuthorized(session, serverName, toolNamesBySession[session])
	}
}
