package proxy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"gotest.tools/assert"
)

func TestHandleDownstreamAuthorizationRequired_NotifiesMatchingEndpoints(t *testing.T) {
	var nilServer *CentianServer
	nilServer.handleDownstreamAuthorizationRequired(centoauth.Binding{}, "")

	matchingEndpoint, matchingSession, binding := newOAuthToolTestProxy(t, false)
	matchingRecorder, cleanupMatching := connectLoggingClient(t, matchingSession)
	defer cleanupMatching()
	matchingSession.identityKey = binding.PrincipalID

	otherEndpoint := newLoggingTestProxy()
	otherEndpoint.name = "other"
	otherEndpoint.endpoint = "/mcp/other/protected"
	otherSession := newLoggingTestSession(otherEndpoint, "session-2")
	otherSession.identityKey = binding.PrincipalID
	otherRecorder, cleanupOther := connectLoggingClient(t, otherSession)
	defer cleanupOther()

	server := matchingEndpoint.server
	otherEndpoint.server = server
	server.Endpoints = []*CentianEndpoint{matchingEndpoint, nil, otherEndpoint}

	server.handleDownstreamAuthorizationRequired(binding, "http://127.0.0.1:9666/oauth/start?id=test")

	messages := waitForSingleLog(t, matchingRecorder)
	assert.Assert(t, len(messages) == 1)
	assert.Equal(t, messages[0].Data.(string), "OAuth required for downstream gateway/protected. Use centian.login.protected or open http://127.0.0.1:9666/oauth/start?id=test")

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, len(otherRecorder.snapshot()), 0)
}

func TestHandleOAuthAuthorizedClosesReplacedConnection(t *testing.T) {
	endpoint, session, binding := newOAuthToolTestProxy(t, false)

	oldConn := &MockDownstreamConnection{
		serverName: binding.Server,
		Status:     StatusRefreshFailed,
	}
	replacement := &MockDownstreamConnection{
		serverName: binding.Server,
		ConnectFunc: func(context.Context, *DownstreamConnectOptions) error {
			return fmt.Errorf("stop after replacement")
		},
	}

	endpoint.connectionFactory = func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
		return replacement
	}
	endpoint.downstreamPools[session.downstreamSessionKey] = &DownstreamSessionPool{
		identityKey:          session.identityKey,
		downstreamSessionKey: session.downstreamSessionKey,
		downstreamConns: map[string]DownstreamConnectionInterface{
			binding.Server: oldConn,
		},
		upstreamSessions: map[string]*UpstreamSession{
			session.id: session,
		},
		connecting: make(map[string]bool),
	}

	endpoint.handleOAuthAuthorized(binding)

	assert.Equal(t, oldConn.CloseCalls, 1)
	assert.Assert(t, endpoint.downstreamPools[session.downstreamSessionKey].downstreamConns[binding.Server] == replacement)
}

func TestHandleDownstreamAuthorized_ReconnectsMatchingEndpointOnly(t *testing.T) {
	var nilServer *CentianServer
	nilServer.handleDownstreamAuthorized(centoauth.Binding{})

	matchingEndpoint, session, binding := newOAuthToolTestProxy(t, false)
	matchingOldConn := &MockDownstreamConnection{
		serverName: binding.Server,
		Status:     StatusRefreshFailed,
	}
	matchingReplacement := &MockDownstreamConnection{
		serverName: binding.Server,
		ConnectFunc: func(context.Context, *DownstreamConnectOptions) error {
			return fmt.Errorf("stop after replacement")
		},
	}
	matchingEndpoint.connectionFactory = func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
		return matchingReplacement
	}
	matchingEndpoint.downstreamPools[session.downstreamSessionKey] = &DownstreamSessionPool{
		identityKey:          session.identityKey,
		downstreamSessionKey: session.downstreamSessionKey,
		downstreamConns: map[string]DownstreamConnectionInterface{
			binding.Server: matchingOldConn,
		},
		upstreamSessions: map[string]*UpstreamSession{
			session.id: session,
		},
		connecting: make(map[string]bool),
	}

	otherOldConn := &MockDownstreamConnection{
		serverName: binding.Server,
		Status:     StatusRefreshFailed,
	}
	otherEndpoint := &CentianEndpoint{
		name:     "protected",
		endpoint: "/mcp/other/protected",
		config: &config.GatewayConfig{
			MCPServers: map[string]*config.MCPServerConfig{
				binding.Server: {URL: "http://127.0.0.1:9001/mcp"},
			},
		},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools: map[string]*DownstreamSessionPool{
			"pool-other": {
				identityKey:          binding.PrincipalID,
				downstreamSessionKey: "pool-other",
				downstreamConns: map[string]DownstreamConnectionInterface{
					binding.Server: otherOldConn,
				},
				upstreamSessions: map[string]*UpstreamSession{
					"session-other": {
						id:              "session-other",
						identityKey:     binding.PrincipalID,
						downstreamConns: map[string]DownstreamConnectionInterface{binding.Server: otherOldConn},
					},
				},
				connecting: make(map[string]bool),
			},
		},
	}
	otherEndpoint.connectionFactory = func(string, *config.MCPServerConfig) DownstreamConnectionInterface {
		t.Fatal("unexpected reconnect for non-matching endpoint")
		return nil
	}

	server := matchingEndpoint.server
	otherEndpoint.server = server
	server.Endpoints = []*CentianEndpoint{matchingEndpoint, nil, otherEndpoint}

	server.handleDownstreamAuthorized(binding)

	assert.Equal(t, matchingOldConn.CloseCalls, 1)
	assert.Assert(t, matchingEndpoint.downstreamPools[session.downstreamSessionKey].downstreamConns[binding.Server] == matchingReplacement)
	assert.Equal(t, otherOldConn.CloseCalls, 0)
	assert.Assert(t, otherEndpoint.downstreamPools["pool-other"].downstreamConns[binding.Server] == otherOldConn)
}
