package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/identifiers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file manages upstream session lifecycle, captured client state, and
// downstream pool reconciliation.

const (
	initialRootsBootstrapTimeout  = 2 * time.Second
	initialRootsBootstrapInterval = 25 * time.Millisecond
)

// downstreamPoolUpdate describes the side effects needed after pool reconciliation.
type downstreamPoolUpdate struct {
	pool         *DownstreamSessionPool
	reused       bool
	syncPool     bool
	closePool    *DownstreamSessionPool
	waitForReady bool
}

type namedDownstreamConnection struct {
	serverName string
	conn       DownstreamConnectionInterface
}

// getSessionID returns the MCP session ID from the request or synthesizes one for new POST sessions.
func getSessionID(r *http.Request) string {
	sessionID := ""
	if r != nil {
		sessionID = r.Header.Get("Mcp-Session-Id")
	}
	if sessionID == "" && r != nil && r.Method == http.MethodPost {
		sessionID = identifiers.New(identifiers.KindSession)
	}
	return sessionID
}

// GetOrCreateServerForRequest returns (or creates) the upstream-facing MCP server for the request.
func (p *CentianEndpoint) GetOrCreateServerForRequest(r *http.Request) *mcp.Server {
	sessionID := getSessionID(r)
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
		session.upstreamServer = p.newUpstreamServer(session)
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

// createUpstreamSession snapshots request-scoped identity, headers, and auth data for one session.
func (p *CentianEndpoint) createUpstreamSession(id string, r *http.Request, identityKey string) *UpstreamSession {
	authData := getAuthData(r.Context())
	var clientHeaders http.Header
	if r != nil && r.Header != nil {
		clientHeaders = r.Header.Clone()
	}
	return &UpstreamSession{
		id:                          id,
		downstreamConns:             make(map[string]DownstreamConnectionInterface),
		registeredTools:             make(map[string]struct{}),
		registeredToolFingerprints:  make(map[string]string),
		toolRoutes:                  make(map[string]toolRoute),
		registeredStaticTools:       make(map[string]struct{}),
		registeredResources:         make(map[string]struct{}),
		registeredResourceTemplates: make(map[string]struct{}),
		registeredPrompts:           make(map[string]struct{}),
		clientHeaders:               clientHeaders,
		identityKey:                 identityKey,
		authData:                    authData.Clone(),
		rootsDirty:                  true,
	}
}

// resolveIdentityKey maps a request to the pool identity used for downstream reuse decisions.
func (p *CentianEndpoint) resolveIdentityKey(r *http.Request) (string, error) {
	if p.server == nil || p.server.Principals == nil {
		return sharedLocalIdentity, nil
	}
	identity, ok := requestIdentityFromContext(r.Context())
	if !ok {
		return "", fmt.Errorf("authenticated request missing resolved principal identity")
	}
	return identity, nil
}

// currentUpstreamServerSession finds the live SDK session that corresponds to the stored upstream session.
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

// syncUpstreamSessionState captures the latest upstream client state and reconciles the downstream pool.
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
	if upstreamClientState.clientName != "" && session.clientName == "" {
		p.mu.Lock()
		session.clientName = upstreamClientState.clientName
		session.clientVersion = upstreamClientState.clientVersion
		p.mu.Unlock()
	}
	p.finalizeDownstreamPoolUpdate(ctx, session, update)
}

// bootstrapUpstreamSessionState retries the initial roots-dependent sync until
// the first downstream pool can be established or the timeout is hit.
func (p *CentianEndpoint) bootstrapUpstreamSessionState(ctx context.Context, sessionID string) {
	deadline := time.Now().Add(initialRootsBootstrapTimeout)
	for {
		p.syncUpstreamSessionState(ctx, sessionID)
		if !p.sessionNeedsInitialRootsBootstrap(sessionID) || time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialRootsBootstrapInterval):
		}
	}
}

// capturedUpstreamClientState is the upstream-facing client state observed from the SDK session.
type capturedUpstreamClientState struct {
	protocolVersion string
	capabilities    *mcp.ClientCapabilities
	roots           []*mcp.Root
	rootsDirty      bool
	clientName      string
	clientVersion   string
}

// readUpstreamClientState reads the current client state from the upstream SDK session.
func (p *CentianEndpoint) readUpstreamClientState(
	ctx context.Context,
	session *UpstreamSession,
	serverSession *mcp.ServerSession,
) (*capturedUpstreamClientState, bool) {
	if session == nil || serverSession == nil || serverSession.InitializeParams() == nil {
		return nil, false
	}

	initializeParams := serverSession.InitializeParams()
	p.mu.RLock()
	sessionRoots := normalizeRoots(session.roots)
	sessionRootsDirty := session.rootsDirty
	p.mu.RUnlock()
	clientState := &capturedUpstreamClientState{
		protocolVersion: initializeParams.ProtocolVersion,
		capabilities:    initializeParams.Capabilities,
		roots:           sessionRoots,
		rootsDirty:      sessionRootsDirty,
		clientName:      initializeParams.ClientInfo.Name,
		clientVersion:   initializeParams.ClientInfo.Version,
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

// deriveDownstreamClientState converts captured upstream client state into the mirrored downstream shape.
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

// reconcileUpstreamSessionState updates the stored session state and selects the target downstream pool.
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

// fetchUpstreamRoots asks the upstream SDK session for the latest roots snapshot.
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

// syncUpstreamSessionToDownstreamState copies mirrored downstream-facing client state onto the upstream session.
func (p *CentianEndpoint) syncUpstreamSessionToDownstreamState(session *UpstreamSession, state *DownstreamClientState) {
	p.storeMirroredClientState(session, state)
	session.downstreamSessionKey = p.buildDownstreamSessionKey(
		session.identityKey,
		session.GetAuthHeaders(p.excludedClientAuthHeader()),
		state,
	)
}

func (p *CentianEndpoint) storeMirroredClientState(session *UpstreamSession, state *DownstreamClientState) {
	session.protocolVersion = state.ProtocolVersion
	session.clientCapabilities = state.ClientCapabilities
	session.capabilitiesFingerprint = state.CapabilitiesFingerprint
	session.roots = normalizeRoots(state.Roots)
	session.rootsFingerprint = state.RootsFingerprint
}

// applyClientStateLocked stores mirrored client state on the session and plans any required pool transition.
// Caller must hold p.mu.
func (p *CentianEndpoint) applyClientStateLocked(
	session *UpstreamSession,
	state *DownstreamClientState,
	rootsDirty bool,
) downstreamPoolUpdate {
	if state == nil {
		state = &DownstreamClientState{}
	}

	// Keep the previous pool key so the transition logic can decide whether this
	// session stays on its current pool, retargets an exclusive pool, or moves to
	// a different shared pool.
	previousKey := session.downstreamSessionKey

	// Persist the newly mirrored client state before making any pool decision.
	// The downstream session key is derived from this snapshot.
	p.syncUpstreamSessionToDownstreamState(session, state)
	session.rootsDirty = rootsDirty
	if session.downstreamSessionKey == "" {
		return downstreamPoolUpdate{}
	}

	if previousKey == "" {
		// This session has not been attached to any pool yet, so create or reuse
		// the first matching pool and wait briefly for an initial usable downstream.
		pool, reused := p.getOrCreateSessionPoolLocked(session)
		return downstreamPoolUpdate{pool: pool, reused: reused, waitForReady: !reused}
	}

	if previousKey == session.downstreamSessionKey {
		if pool, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
			// The session still belongs to the same pool. When the pool is dedicated
			// to this single session, client-state changes can be pushed into the
			// existing downstream connections instead of reconnecting.
			return downstreamPoolUpdate{pool: pool, reused: true, syncPool: len(pool.upstreamSessions) == 1 && !rootsDirty}
		}

		// The key is unchanged but the pool entry is gone, so rebuild the expected
		// pool attachment from the current session state.
		pool, reused := p.getOrCreateSessionPoolLocked(session)
		return downstreamPoolUpdate{pool: pool, reused: reused, waitForReady: !reused}
	}

	currentPool := p.downstreamPools[previousKey]
	if currentPool != nil && len(currentPool.upstreamSessions) == 1 {
		if existingTarget, ok := p.downstreamPools[session.downstreamSessionKey]; ok {
			// The session exclusively owns its current pool, but another pool already
			// exists for the new key. Move the session there and close the old pool.
			delete(currentPool.upstreamSessions, session.id)
			delete(p.downstreamPools, previousKey)
			p.bindSessionToPoolLocked(session, existingTarget)
			p.startMissingPoolConnectionsLocked(existingTarget, p.buildDownstreamConnectOptions(session))
			return downstreamPoolUpdate{pool: existingTarget, reused: true, closePool: currentPool}
		}

		// The session exclusively owns its current pool and no target pool exists, so
		// retarget the current pool in place instead of replacing all downstreams.
		delete(p.downstreamPools, previousKey)
		currentPool.downstreamSessionKey = session.downstreamSessionKey
		currentPool.lastUsed = time.Now()
		p.downstreamPools[session.downstreamSessionKey] = currentPool
		session.downstreamConns = currentPool.downstreamConns
		return downstreamPoolUpdate{pool: currentPool, reused: true, syncPool: !rootsDirty}
	}

	var closePool *DownstreamSessionPool
	if currentPool != nil {
		// Shared pools cannot be retargeted in place because other sessions still
		// depend on the old key. Detach this session and close the old pool only if
		// it becomes empty afterwards.
		delete(currentPool.upstreamSessions, session.id)
		if len(currentPool.upstreamSessions) == 0 {
			delete(p.downstreamPools, previousKey)
			closePool = currentPool
		}
	}

	// Reuse an existing pool for the new key when available, otherwise create one.
	pool, reused := p.getOrCreateSessionPoolLocked(session)
	return downstreamPoolUpdate{
		pool:         pool,
		reused:       reused,
		closePool:    closePool,
		waitForReady: !reused,
	}
}

func (p *CentianEndpoint) sessionNeedsInitialRootsBootstrap(sessionID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	session := p.upstreamSessions[sessionID]
	if session == nil {
		return false
	}
	return session.rootsDirty &&
		clientSupportsRoots(session.clientCapabilities)
}

// finalizeDownstreamPoolUpdate executes the pool actions planned during reconciliation outside the mutex.
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
	p.registerAvailableResources(session)
	p.registerAvailableResourceTemplates(session)
	p.registerAvailablePrompts(session)
}

// syncPoolClientState pushes mirrored client state changes into every downstream connection in the pool.
func (p *CentianEndpoint) syncPoolClientState(ctx context.Context, pool *DownstreamSessionPool, state *DownstreamClientState) {
	for _, conn := range pool.downstreamConns {
		if err := conn.SyncClientState(ctx, state); err != nil {
			common.LogWarn("ProxyEndpoint[%s]: failed to sync downstream client state for %s: %v", p.name, conn.GetServerName(), err)
		}
	}
}

// markUpstreamSessionRootsDirty records that the cached roots snapshot must be refreshed on the next sync.
func (p *CentianEndpoint) markUpstreamSessionRootsDirty(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if session, ok := p.upstreamSessions[sessionID]; ok {
		session.rootsDirty = true
	}
}

// invalidateDownstreamPool removes and closes one downstream pool by session key.
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

func (p *CentianEndpoint) sessionNeedsSync(session *UpstreamSession) bool {
	if p == nil || session == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return session.protocolVersion == "" || session.rootsDirty
}

func (p *CentianEndpoint) sessionRootsDirty(session *UpstreamSession) bool {
	if p == nil || session == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return session.rootsDirty
}

func (p *CentianEndpoint) sessionDownstreamSessionKey(session *UpstreamSession) string {
	if p == nil || session == nil {
		return ""
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return session.downstreamSessionKey
}

func (p *CentianEndpoint) sessionConnection(session *UpstreamSession, serverName string) (DownstreamConnectionInterface, error) {
	if p == nil || session == nil {
		return nil, fmt.Errorf("upstream session is not available")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return connectionByServerName(session.downstreamConns, serverName)
}

func (p *CentianEndpoint) sessionConnectionSnapshot(session *UpstreamSession) []namedDownstreamConnection {
	if p == nil || session == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(session.downstreamConns) == 0 {
		return nil
	}

	serverNames := make([]string, 0, len(session.downstreamConns))
	for serverName := range session.downstreamConns {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)

	snapshot := make([]namedDownstreamConnection, 0, len(serverNames))
	for _, serverName := range serverNames {
		snapshot = append(snapshot, namedDownstreamConnection{
			serverName: serverName,
			conn:       session.downstreamConns[serverName],
		})
	}
	return snapshot
}

// Close terminates all sessions and their downstream connections.
func (p *CentianEndpoint) Close() []error {
	p.mu.Lock()
	pools := make([]*DownstreamSessionPool, 0, len(p.downstreamPools))
	for key, pool := range p.downstreamPools {
		pools = append(pools, pool)
		delete(p.downstreamPools, key)
	}
	sessions := make([]*UpstreamSession, 0, len(p.upstreamSessions))
	for _, session := range p.upstreamSessions {
		sessions = append(sessions, session)
	}
	p.mu.Unlock()

	for _, session := range sessions {
		session.taskMu.Lock()
		p.cancelTaskTimeoutLocked(session)
		session.taskMu.Unlock()
	}

	errs := make([]error, 0)
	for _, pool := range pools {
		errs = append(errs, p.closeDownstreamSessionPool(pool)...)
	}
	return errs
}

// closeDownstreamSessionPool closes every live downstream connection owned by the pool.
func (p *CentianEndpoint) closeDownstreamSessionPool(pool *DownstreamSessionPool) []error {
	if pool == nil {
		return nil
	}
	p.cancelPoolRetryWorkers(pool)
	errs := make([]error, 0)
	for _, conn := range pool.downstreamConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
