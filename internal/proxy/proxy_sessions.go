package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file manages upstream session lifecycle, captured client state, and
// downstream pool reconciliation.

type downstreamPoolUpdate struct {
	pool         *DownstreamSessionPool
	reused       bool
	syncPool     bool
	closePool    *DownstreamSessionPool
	waitForReady bool
}

// GetOrCreateServerForRequest returns (or creates) the upstream-facing MCP server for the request.
func (p *CentianEndpoint) GetOrCreateServerForRequest(r *http.Request) *mcp.Server {
	sessionID := ""
	if r != nil {
		sessionID = r.Header.Get("Mcp-Session-Id")
	}
	if sessionID == "" && r != nil && r.Method == http.MethodPost {
		sessionID = "csid-" + getNewUUIDV7()
	}
	if sessionID == "" {
		return nil
	}

	common.LogInfo("ProxyEndpoint[%s]: getting server for session %s", p.name, sessionID)

	identityKey, err := p.resolveIdentityKey(r)
	if err != nil {
		common.LogError("ProxyEndpoint[%s]: failed to resolve request identity: %v", p.name, err)
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	session, exists := p.upstreamSessions[sessionID]
	if !exists {
		session = p.createUpstreamSession(sessionID, r, identityKey)
		session.upstreamServer = p.newUpstreamServer(sessionID)
		p.upstreamSessions[sessionID] = session
		return session.upstreamServer
	}
	if session.identityKey != identityKey {
		common.LogWarn(
			"ProxyEndpoint[%s]: session identity mismatch for session %s (existing=%s current=%s)",
			p.name,
			sessionID,
			session.identityKey,
			identityKey,
		)
		return nil
	}
	return session.upstreamServer
}

// GetServerForRequest is kept as a compatibility wrapper while callers migrate.
func (p *CentianEndpoint) GetServerForRequest(r *http.Request) *mcp.Server {
	return p.GetOrCreateServerForRequest(r)
}

func (p *CentianEndpoint) createUpstreamSession(id string, r *http.Request, identityKey string) *UpstreamSession {
	authData := getAuthData(r.Context())
	var clientHeaders http.Header
	if r != nil && r.Header != nil {
		clientHeaders = r.Header.Clone()
	}
	return &UpstreamSession{
		id:              id,
		downstreamConns: make(map[string]DownstreamConnectionInterface),
		registeredTools: make(map[string]struct{}),
		clientHeaders:   clientHeaders,
		identityKey:     identityKey,
		authData:        authData.Clone(),
		rootsDirty:      true,
	}
}

func (p *CentianEndpoint) resolveIdentityKey(r *http.Request) (string, error) {
	if p.server == nil || p.server.APIKeys == nil {
		return sharedLocalIdentity, nil
	}
	identity, ok := requestIdentityFromContext(r.Context())
	if !ok {
		return "", fmt.Errorf("authenticated request missing resolved API key identity")
	}
	return identity, nil
}

func (p *CentianEndpoint) currentUpstreamServerSession(session *UpstreamSession) *mcp.ServerSession {
	if session == nil || session.upstreamServer == nil {
		return nil
	}

	for serverSession := range session.upstreamServer.Sessions() {
		if serverSession == nil {
			continue
		}
		if serverSession.ID() == session.id || session.id == "" {
			return serverSession
		}
	}
	return nil
}

func (p *CentianEndpoint) syncUpstreamSessionState(ctx context.Context, sessionID string) {
	p.mu.RLock()
	session := p.upstreamSessions[sessionID]
	p.mu.RUnlock()
	if session == nil {
		return
	}

	serverSession := p.currentUpstreamServerSession(session)
	upstreamClientState, ok := p.readUpstreamClientState(ctx, session, serverSession)
	if !ok {
		return
	}

	mirroredClientState := deriveDownstreamClientState(upstreamClientState)
	session, update := p.reconcileUpstreamSessionState(sessionID, mirroredClientState, upstreamClientState.rootsDirty)
	if session == nil {
		return
	}
	p.finalizeDownstreamPoolUpdate(ctx, session, update)
}

type capturedUpstreamClientState struct {
	protocolVersion string
	capabilities    *mcp.ClientCapabilities
	roots           []*mcp.Root
	rootsDirty      bool
}

func (p *CentianEndpoint) readUpstreamClientState(
	ctx context.Context,
	session *UpstreamSession,
	serverSession *mcp.ServerSession,
) (*capturedUpstreamClientState, bool) {
	if session == nil || serverSession == nil || serverSession.InitializeParams() == nil {
		return nil, false
	}

	initializeParams := serverSession.InitializeParams()
	clientState := &capturedUpstreamClientState{
		protocolVersion: initializeParams.ProtocolVersion,
		capabilities:    initializeParams.Capabilities,
		roots:           session.roots,
		rootsDirty:      session.rootsDirty,
	}

	if !clientSupportsRoots(initializeParams.Capabilities) {
		return clientState, true
	}

	latestRoots, err := p.fetchUpstreamRoots(ctx, session, serverSession)
	if err != nil {
		common.LogDebug("ProxyEndpoint[%s]: failed to refresh roots for session %s: %v", p.name, session.id, err)
		return clientState, true
	}

	clientState.roots = latestRoots
	clientState.rootsDirty = false
	return clientState, true
}

func deriveDownstreamClientState(clientState *capturedUpstreamClientState) *DownstreamClientState {
	if clientState == nil {
		return &DownstreamClientState{}
	}

	return buildDownstreamClientState(
		clientState.protocolVersion,
		clientState.capabilities,
		clientState.roots,
	)
}

func (p *CentianEndpoint) reconcileUpstreamSessionState(
	sessionID string,
	state *DownstreamClientState,
	rootsDirty bool,
) (*UpstreamSession, downstreamPoolUpdate) {
	var update downstreamPoolUpdate

	p.mu.Lock()
	session := p.upstreamSessions[sessionID]
	if session != nil {
		update = p.applyClientStateLocked(session, state, rootsDirty)
	}
	p.mu.Unlock()

	return session, update
}

func (p *CentianEndpoint) fetchUpstreamRoots(ctx context.Context, session *UpstreamSession, serverSession *mcp.ServerSession) ([]*mcp.Root, error) {
	if session == nil || serverSession == nil {
		return nil, nil
	}
	result, err := serverSession.ListRoots(ctx, nil)
	if err != nil {
		return nil, err
	}
	return normalizeRoots(result.Roots), nil
}

func (p *CentianEndpoint) applyClientStateLocked(
	session *UpstreamSession,
	state *DownstreamClientState,
	rootsDirty bool,
) downstreamPoolUpdate {
	if state == nil {
		state = &DownstreamClientState{}
	}
	previousKey := session.downstreamSessionKey
	session.protocolVersion = state.ProtocolVersion
	session.clientCapabilities = state.ClientCapabilities
	session.capabilitiesFingerprint = state.CapabilitiesFingerprint
	session.roots = normalizeRoots(state.Roots)
	session.rootsFingerprint = state.RootsFingerprint
	session.rootsDirty = rootsDirty

	session.downstreamSessionKey = p.buildDownstreamSessionKey(
		session.identityKey,
		session.GetAuthHeaders(p.excludedClientAuthHeader()),
		state,
	)
	if session.downstreamSessionKey == "" {
		return downstreamPoolUpdate{}
	}

	if previousKey == "" {
		pool, reused := p.attachUpstreamSessionToPoolLocked(session)
		return downstreamPoolUpdate{pool: pool, reused: reused, waitForReady: !reused}
	}

	if previousKey == session.downstreamSessionKey {
		if pool, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
			return downstreamPoolUpdate{pool: pool, reused: true, syncPool: len(pool.upstreamSessions) == 1 && !rootsDirty}
		}
		pool, reused := p.attachUpstreamSessionToPoolLocked(session)
		return downstreamPoolUpdate{pool: pool, reused: reused, waitForReady: !reused}
	}

	currentPool := p.downstreamPools[previousKey]
	if currentPool != nil && len(currentPool.upstreamSessions) == 1 {
		if existingTarget, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
			delete(currentPool.upstreamSessions, session.id)
			delete(p.downstreamPools, previousKey)
			existingTarget.upstreamSessions[session.id] = session
			session.downstreamConns = existingTarget.downstreamConns
			p.ensureDownstreamConnectionsLocked(existingTarget, p.buildDownstreamConnectOptions(session))
			return downstreamPoolUpdate{pool: existingTarget, reused: true, closePool: currentPool}
		}

		delete(p.downstreamPools, previousKey)
		currentPool.downstreamSessionKey = session.downstreamSessionKey
		currentPool.lastUsed = time.Now()
		p.downstreamPools[session.downstreamSessionKey] = currentPool
		session.downstreamConns = currentPool.downstreamConns
		return downstreamPoolUpdate{pool: currentPool, reused: true, syncPool: !rootsDirty}
	}

	var closePool *DownstreamSessionPool
	if currentPool != nil {
		delete(currentPool.upstreamSessions, session.id)
		if len(currentPool.upstreamSessions) == 0 {
			delete(p.downstreamPools, previousKey)
			closePool = currentPool
		}
	}

	pool, reused := p.attachUpstreamSessionToPoolLocked(session)
	return downstreamPoolUpdate{pool: pool, reused: reused, closePool: closePool, waitForReady: !reused}
}

func (p *CentianEndpoint) finalizeDownstreamPoolUpdate(ctx context.Context, session *UpstreamSession, update downstreamPoolUpdate) {
	if update.closePool != nil {
		p.closeDownstreamSessionPool(update.closePool)
	}
	if update.pool == nil {
		return
	}
	if update.syncPool {
		p.syncPoolClientState(ctx, update.pool, session.downstreamClientState())
	}
	if update.waitForReady {
		p.waitForFirstUsableDownstream(update.pool, initialDownstreamReadyWait)
	}
	p.registerAvailableTools(session)
}

func (p *CentianEndpoint) syncPoolClientState(ctx context.Context, pool *DownstreamSessionPool, state *DownstreamClientState) {
	for _, conn := range pool.downstreamConns {
		if err := conn.SyncClientState(ctx, state); err != nil {
			common.LogWarn("ProxyEndpoint[%s]: failed to sync downstream client state for %s: %v", p.name, conn.GetServerName(), err)
		}
	}
}

func (p *CentianEndpoint) markUpstreamSessionRootsDirty(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if session, ok := p.upstreamSessions[sessionID]; ok {
		session.rootsDirty = true
	}
}

func (p *CentianEndpoint) invalidateDownstreamPool(downstreamSessionKey string) {
	if downstreamSessionKey == "" {
		return
	}

	p.mu.Lock()
	pool, ok := p.downstreamPools[downstreamSessionKey]
	if ok {
		delete(p.downstreamPools, downstreamSessionKey)
	}
	p.mu.Unlock()

	if !ok {
		return
	}
	common.LogWarn("ProxyEndpoint[%s]: invalidating pooled downstream session %s", p.name, downstreamSessionKey)
	p.closeDownstreamSessionPool(pool)
}

// invalidatePooledDownstream is kept as a compatibility wrapper while callers migrate.
func (p *CentianEndpoint) invalidatePooledDownstream(downstreamSessionKey string) {
	p.invalidateDownstreamPool(downstreamSessionKey)
}

// Close terminates all sessions and their downstream connections.
func (p *CentianEndpoint) Close() []error {
	p.mu.Lock()
	pools := make([]*DownstreamSessionPool, 0, len(p.downstreamPools))
	for key, pool := range p.downstreamPools {
		pools = append(pools, pool)
		delete(p.downstreamPools, key)
	}
	p.mu.Unlock()

	errs := make([]error, 0)
	for _, pool := range pools {
		errs = append(errs, p.closeDownstreamSessionPool(pool)...)
	}
	return errs
}

func (p *CentianEndpoint) closeDownstreamSessionPool(pool *DownstreamSessionPool) []error {
	if pool == nil {
		return nil
	}
	errs := make([]error, 0)
	for _, conn := range pool.downstreamConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// closePoolEntryLocked is kept as a compatibility wrapper while callers migrate.
func (p *CentianEndpoint) closePoolEntryLocked(pool *DownstreamSessionPool) []error {
	return p.closeDownstreamSessionPool(pool)
}
