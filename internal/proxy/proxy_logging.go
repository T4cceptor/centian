package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file forwards downstream logging notifications to the live upstream
// sessions attached to the same pooled downstream session.

func (p *CentianEndpoint) newPoolLoggingHandler(
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
) DownstreamLoggingHandler {
	return func(ctx context.Context, req *mcp.LoggingMessageRequest) {
		if req == nil || req.Params == nil {
			return
		}
		p.forwardDownstreamLog(ctx, downstreamSessionKey, serverName, conn, req.Params)
	}
}

func (p *CentianEndpoint) forwardDownstreamLog(
	ctx context.Context,
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
	params *mcp.LoggingMessageParams,
) {
	if params == nil || downstreamSessionKey == "" {
		return
	}

	sessions := p.snapshotPoolUpstreamSessions(downstreamSessionKey, conn)
	for _, session := range sessions {
		serverSession := p.currentUpstreamServerSession(session)
		if serverSession == nil {
			continue
		}
		if err := serverSession.Log(ctx, params); err != nil {
			common.LogWarn(
				"ProxyEndpoint[%s]: failed to forward downstream log from %s to upstream session %s: %v",
				p.name,
				sanitizeLogValue(serverName),
				sanitizeLogValue(session.id),
				err,
			)
		}
	}
}

func (p *CentianEndpoint) snapshotPoolUpstreamSessions(
	downstreamSessionKey string,
	conn DownstreamConnectionInterface,
) []*UpstreamSession {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool != nil && (conn == nil || poolContainsConnection(pool, conn)) {
		return clonePoolUpstreamSessions(pool)
	}

	if conn == nil {
		return nil
	}
	for _, pool = range p.downstreamPools {
		if poolContainsConnection(pool, conn) {
			return clonePoolUpstreamSessions(pool)
		}
	}
	return nil
}

func clonePoolUpstreamSessions(pool *DownstreamSessionPool) []*UpstreamSession {
	if pool == nil {
		return nil
	}

	sessions := make([]*UpstreamSession, 0, len(pool.upstreamSessions))
	for _, session := range pool.upstreamSessions {
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

func poolContainsConnection(pool *DownstreamSessionPool, conn DownstreamConnectionInterface) bool {
	if pool == nil || conn == nil {
		return false
	}
	current, ok := pool.downstreamConns[conn.GetServerName()]
	return ok && current == conn
}

func (p *CentianEndpoint) syncUpstreamLoggingLevel(sessionID string, level mcp.LoggingLevel) {
	if sessionID == "" || level == "" {
		return
	}

	p.mu.Lock()
	session := p.upstreamSessions[sessionID]
	if session != nil {
		session.logLevel = level
	}
	p.mu.Unlock()

	if session == nil || session.downstreamSessionKey == "" {
		return
	}
	p.syncPoolLoggingLevel(session.downstreamSessionKey)
}

func (p *CentianEndpoint) syncPoolLoggingLevel(downstreamSessionKey string) {
	level, conns := p.poolLoggingState(downstreamSessionKey)
	if level == "" {
		return
	}

	for _, conn := range conns {
		if err := conn.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: level}); err != nil {
			common.LogWarn(
				"ProxyEndpoint[%s]: failed to sync downstream logging level for %s: %v",
				p.name,
				sanitizeLogValue(conn.GetServerName()),
				err,
			)
		}
	}
}

func (p *CentianEndpoint) poolLoggingState(downstreamSessionKey string) (mcp.LoggingLevel, []DownstreamConnectionInterface) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pool := p.downstreamPools[downstreamSessionKey]
	if pool == nil {
		return "", nil
	}

	level := mostVerbosePoolLogLevel(pool)
	if level == "" {
		return "", nil
	}

	conns := make([]DownstreamConnectionInterface, 0, len(pool.downstreamConns))
	for _, conn := range pool.downstreamConns {
		if conn != nil && conn.IsConnected() {
			conns = append(conns, conn)
		}
	}
	return level, conns
}

func mostVerbosePoolLogLevel(pool *DownstreamSessionPool) mcp.LoggingLevel {
	if pool == nil {
		return ""
	}

	var level mcp.LoggingLevel
	for _, session := range pool.upstreamSessions {
		if session == nil || session.logLevel == "" {
			continue
		}
		if level == "" || loggingLevelRank(session.logLevel) < loggingLevelRank(level) {
			level = session.logLevel
		}
	}
	return level
}

func loggingLevelRank(level mcp.LoggingLevel) int {
	switch level {
	case "debug":
		return 0
	case "info":
		return 1
	case "notice":
		return 2
	case "warning":
		return 3
	case "error":
		return 4
	case "critical":
		return 5
	case "alert":
		return 6
	case "emergency":
		return 7
	default:
		return 100
	}
}
