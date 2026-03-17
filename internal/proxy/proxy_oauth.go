package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (c *CentianServer) handleDownstreamAuthorizationRequired(binding centoauth.Binding, authURL string) {
	if c == nil {
		return
	}
	for _, endpoint := range c.Endpoints {
		if endpoint == nil {
			continue
		}
		endpoint.notifyOAuthRequired(binding, authURL)
	}
}

func (c *CentianServer) handleDownstreamAuthorized(binding centoauth.Binding) {
	if c == nil {
		return
	}
	for _, endpoint := range c.Endpoints {
		if endpoint == nil {
			continue
		}
		endpoint.handleOAuthAuthorized(binding)
	}
}

func (p *CentianEndpoint) notifyOAuthRequired(binding centoauth.Binding, authURL string) {
	if p == nil || binding.Gateway == "" || binding.Server == "" {
		return
	}
	endpointGateway := getGatewayFromPath(p.endpoint)
	if endpointGateway != binding.Gateway {
		return
	}
	message := fmt.Sprintf(
		"OAuth required for downstream %s/%s. Use %s or open %s",
		binding.Gateway,
		binding.Server,
		loginToolName(binding.Server),
		authURL,
	)

	p.mu.RLock()
	sessions := make([]*UpstreamSession, 0, len(p.upstreamSessions))
	for _, session := range p.upstreamSessions {
		if session != nil && session.identityKey == binding.PrincipalID {
			sessions = append(sessions, session)
		}
	}
	p.mu.RUnlock()

	for _, session := range sessions {
		if err := p.logUpstreamSession(context.Background(), session, &mcp.LoggingMessageParams{
			Level: logLevelInfo,
			Data:  message,
		}); err != nil {
			common.LogWarn("ProxyEndpoint[%s]: failed to notify OAuth requirement for session %s: %v", p.name, session.id, err)
		}
	}
}

func (p *CentianEndpoint) notifyOAuthAuthorized(session *UpstreamSession, serverName string, toolNames []string) {
	if p == nil || session == nil {
		return
	}

	toolList := strings.Join(toolNames, ", ")
	if toolList == "" {
		toolList = "(none)"
	}

	if err := p.logUpstreamSession(context.Background(), session, &mcp.LoggingMessageParams{
		Level: logLevelInfo,
		Data: fmt.Sprintf(
			"OAuth complete for downstream %s. Upstream tools now: %s",
			serverName,
			toolList,
		),
	}); err != nil {
		common.LogWarn("ProxyEndpoint[%s]: failed to notify OAuth success for session %s: %v", p.name, session.id, err)
	}
}

func (p *CentianEndpoint) handleOAuthAuthorized(binding centoauth.Binding) {
	if p == nil {
		return
	}
	if getGatewayFromPath(p.endpoint) != binding.Gateway {
		return
	}

	p.mu.Lock()
	type reconnectRequest struct {
		downstreamSessionKey string
		oldConn              DownstreamConnectionInterface
		conn                 DownstreamConnectionInterface
		options              *DownstreamConnectOptions
	}
	reconnects := make([]reconnectRequest, 0)

	for downstreamSessionKey, pool := range p.downstreamPools {
		if pool == nil || pool.identityKey != binding.PrincipalID {
			continue
		}
		conn, ok := pool.downstreamConns[binding.Server]
		if !ok {
			continue
		}
		serverConfig := p.GetActiveMCPServerConfigs()[binding.Server]
		if serverConfig == nil {
			continue
		}
		var session *UpstreamSession
		for _, candidate := range pool.upstreamSessions {
			if candidate == nil {
				continue
			}
			candidate.downstreamConns = pool.downstreamConns
			if session == nil {
				session = candidate
			}
		}
		if session == nil {
			continue
		}
		newConn := p.newDownstreamConnection(binding.Server, serverConfig)
		pool.downstreamConns[binding.Server] = newConn
		pool.connecting[binding.Server] = true
		reconnects = append(reconnects, reconnectRequest{
			downstreamSessionKey: downstreamSessionKey,
			oldConn:              conn,
			conn:                 newConn,
			options:              cloneDownstreamConnectOptions(p.buildDownstreamConnectOptions(session)),
		})
	}
	p.mu.Unlock()

	for _, reconnect := range reconnects {
		if reconnect.oldConn != nil {
			if err := reconnect.oldConn.Close(); err != nil {
				common.LogWarn(
					"ProxyEndpoint[%s]: failed to close replaced downstream %s: %v",
					p.name,
					reconnect.oldConn.GetServerName(),
					err,
				)
			}
		}
		go p.connectDownstreamPool(reconnect.downstreamSessionKey, reconnect.conn, reconnect.options)
	}
}
