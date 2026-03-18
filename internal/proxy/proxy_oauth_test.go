package proxy

import (
	"context"
	"fmt"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

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
