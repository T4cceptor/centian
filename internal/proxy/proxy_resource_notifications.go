package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file forwards downstream resource notifications to the live upstream
// sessions attached to the same pooled downstream session.

func (p *CentianEndpoint) newPoolResourceListChangedHandler(
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
) DownstreamResourceListChangedHandler {
	return func(ctx context.Context, req *mcp.ResourceListChangedRequest) {
		p.refreshDownstreamResources(ctx, downstreamSessionKey, serverName, conn, req)
	}
}

func (p *CentianEndpoint) newPoolResourceUpdatedHandler(
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
) DownstreamResourceUpdatedHandler {
	return func(ctx context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
		if req == nil || req.Params == nil {
			return
		}
		p.forwardDownstreamResourceUpdated(ctx, downstreamSessionKey, serverName, conn, req.Params)
	}
}

func (p *CentianEndpoint) refreshDownstreamResources(
	ctx context.Context,
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
	req *mcp.ResourceListChangedRequest,
) {
	if req == nil || downstreamSessionKey == "" || conn == nil {
		return
	}

	if err := conn.DiscoverResources(ctx); err != nil {
		common.LogWarn(
			"ProxyEndpoint[%s]: failed to refresh downstream resources for %s: %v",
			p.name,
			sanitizeLogValue(serverName),
			err,
		)
	}
	if err := conn.DiscoverResourceTemplates(ctx); err != nil {
		common.LogWarn(
			"ProxyEndpoint[%s]: failed to refresh downstream resource templates for %s: %v",
			p.name,
			sanitizeLogValue(serverName),
			err,
		)
	}

	sessions := p.snapshotPoolUpstreamSessions(downstreamSessionKey, conn)
	p.toolRegMu.Lock()
	defer p.toolRegMu.Unlock()
	for _, session := range sessions {
		p.syncAvailableResources(session)
		p.syncAvailableResourceTemplates(session)
	}
}

func (p *CentianEndpoint) forwardDownstreamResourceUpdated(
	ctx context.Context,
	downstreamSessionKey string,
	serverName string,
	conn DownstreamConnectionInterface,
	params *mcp.ResourceUpdatedNotificationParams,
) {
	if params == nil || downstreamSessionKey == "" {
		return
	}

	sessions := p.snapshotPoolUpstreamSessions(downstreamSessionKey, conn)
	for _, session := range sessions {
		if p.currentUpstreamServerSession(session) == nil {
			continue
		}
		if err := session.upstreamServer.ResourceUpdated(ctx, params); err != nil {
			common.LogWarn(
				"ProxyEndpoint[%s]: failed to forward downstream resource update from %s to upstream session %s: %v",
				p.name,
				sanitizeLogValue(serverName),
				sanitizeLogValue(session.id),
				err,
			)
		}
	}
}
