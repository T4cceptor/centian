package proxy

import (
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestDownstreamSessionPoolIsConnecting(t *testing.T) {
	pool := &DownstreamSessionPool{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"connecting": NewDownstreamConnection("connecting", &config.MCPServerConfig{}),
			"idle":       NewDownstreamConnection("idle", &config.MCPServerConfig{}),
		},
	}

	connectingConn := pool.downstreamConns["connecting"].(*DownstreamConnection)
	connectingConn.status = StatusConnecting

	idleConn := pool.downstreamConns["idle"].(*DownstreamConnection)
	idleConn.status = StatusConnected

	isConnecting, err := pool.IsConnecting("connecting")
	assert.NilError(t, err)
	assert.Assert(t, isConnecting)

	isConnecting, err = pool.IsConnecting("idle")
	assert.NilError(t, err)
	assert.Assert(t, !isConnecting)

	_, err = pool.IsConnecting("missing")
	assert.ErrorContains(t, err, `no connection to server "missing" found`)
}

func TestDownstreamSessionPoolHasActiveConnectWorker(t *testing.T) {
	pool := &DownstreamSessionPool{
		connecting: map[string]bool{
			"server-a": true,
			"server-b": false,
		},
	}

	assert.Assert(t, pool.HasActiveConnectWorker("server-a"))
	assert.Assert(t, !pool.HasActiveConnectWorker("server-b"))
	assert.Assert(t, !pool.HasActiveConnectWorker("missing"))
}
