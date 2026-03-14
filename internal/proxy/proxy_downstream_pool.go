package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
)

// This file manages reusable downstream connection pools keyed by effective
// upstream client state.

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

func (p *CentianEndpoint) newDownstreamConnection(serverName string, cfg *config.MCPServerConfig) DownstreamConnectionInterface {
	if p.connectionFactory != nil {
		return p.connectionFactory(serverName, cfg)
	}
	return NewDownstreamConnection(serverName, cfg)
}

func (p *CentianEndpoint) buildDownstreamConnectOptions(session *UpstreamSession) *DownstreamConnectOptions {
	clientState := session.downstreamClientState()
	if clientState == nil {
		clientState = &DownstreamClientState{}
	}

	return &DownstreamConnectOptions{
		ForwardedHeaders:   session.GetAuthHeaders(p.excludedClientAuthHeader()),
		ClientState:        *clientState,
		SamplingHandler:    p.forwardSamplingRequest,
		ElicitationHandler: p.forwardElicitationRequest,
	}
}

// attachUpstreamSessionToPoolLocked attaches the upstream session to a matching downstream pool.
// Caller must hold p.mu.
func (p *CentianEndpoint) attachUpstreamSessionToPoolLocked(session *UpstreamSession) (*DownstreamSessionPool, bool) {
	if session.downstreamSessionKey == "" {
		return nil, false
	}

	connectOptions := p.buildDownstreamConnectOptions(session)
	if existing, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
		existing.lastUsed = time.Now()
		existing.upstreamSessions[session.id] = session
		session.downstreamConns = existing.downstreamConns
		p.ensureDownstreamConnectionsLocked(existing, connectOptions)
		return existing, true
	}

	pool := &DownstreamSessionPool{
		identityKey:          session.identityKey,
		downstreamSessionKey: session.downstreamSessionKey,
		downstreamConns:      make(map[string]DownstreamConnectionInterface, len(p.downstreams)),
		upstreamSessions:     map[string]*UpstreamSession{session.id: session},
		connecting:           make(map[string]bool),
		lastUsed:             time.Now(),
	}
	p.downstreamPools[session.downstreamSessionKey] = pool
	session.downstreamConns = pool.downstreamConns
	p.ensureDownstreamConnectionsLocked(pool, connectOptions)
	return pool, false
}

func (p *CentianEndpoint) ensureDownstreamConnectionsLocked(pool *DownstreamSessionPool, connectOptions *DownstreamConnectOptions) {
	for serverName, template := range p.downstreams {
		conn, err := pool.GetConnectionByServerName(serverName)
		if err != nil || conn.GetStatus().IsFailed() {
			conn = p.newDownstreamConnection(serverName, template.config)
			pool.downstreamConns[serverName] = conn
		}
		if pool.connecting[serverName] || conn.IsConnected() {
			continue
		}
		pool.connecting[serverName] = true
		go p.connectDownstreamPool(pool.downstreamSessionKey, conn, cloneDownstreamConnectOptions(connectOptions))
	}
}

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
	if err := conn.Connect(context.Background(), connectOptions); err != nil {
		p.mu.Lock()
		if pool, ok := p.downstreamPools[downstreamSessionKey]; ok {
			if current, exists := pool.downstreamConns[serverName]; exists && current == conn {
				delete(pool.connecting, serverName)
			}
		}
		p.mu.Unlock()
		common.LogWarn("ProxyEndpoint[%s]: failed to connect pooled downstream %s: %v", p.name, serverName, err)
		return
	}

	p.mu.Lock()
	var sessions []*UpstreamSession
	if pool, ok := p.downstreamPools[downstreamSessionKey]; ok {
		if current, exists := pool.downstreamConns[serverName]; exists && current == conn {
			delete(pool.connecting, serverName)
			pool.lastUsed = time.Now()
			for _, session := range pool.upstreamSessions {
				sessions = append(sessions, session)
			}
		}
	}
	p.mu.Unlock()

	p.toolRegMu.Lock()
	for _, session := range sessions {
		p.syncAvailableTools(session)
	}
	p.toolRegMu.Unlock()

	common.LogInfo("ProxyEndpoint[%s]: connected pooled downstream %s with %d tools", p.name, sanitizeLogValue(serverName), len(conn.Tools()))
}
