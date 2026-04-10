package proxy

import (
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestApplyForceReadOnlyHintsNilAnnotations(t *testing.T) {
	// Given: a tool with nil annotations
	tool := &mcp.Tool{Name: "test-tool"}

	// When: applying force read-only hints
	applyForceReadOnlyHints(tool)

	// Then: annotations are created with ReadOnlyHint=true
	assert.Assert(t, tool.Annotations != nil)
	assert.Equal(t, tool.Annotations.ReadOnlyHint, true)
}

func TestApplyForceReadOnlyHintsPreservesExistingFields(t *testing.T) {
	// Given: a tool with existing annotations (destructive=false, open-world=false)
	destructive := false
	openWorld := false
	tool := &mcp.Tool{
		Name: "test-tool",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
			IdempotentHint:  true,
		},
	}

	// When: applying force read-only hints
	applyForceReadOnlyHints(tool)

	// Then: ReadOnlyHint is set and other fields are preserved
	assert.Equal(t, tool.Annotations.ReadOnlyHint, true)
	assert.Equal(t, tool.Annotations.IdempotentHint, true)
	assert.Assert(t, tool.Annotations.DestructiveHint != nil)
	assert.Equal(t, *tool.Annotations.DestructiveHint, false)
	assert.Assert(t, tool.Annotations.OpenWorldHint != nil)
	assert.Equal(t, *tool.Annotations.OpenWorldHint, false)
}

func TestApplyForceSafeToolHintsOverridesAllFields(t *testing.T) {
	// Given: a tool with existing conflicting annotations
	destructive := true
	openWorld := true
	tool := &mcp.Tool{
		Name: "test-tool",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			IdempotentHint:  false,
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
		},
	}

	// When: applying force safe tool hints
	applyForceSafeToolHints(tool)

	// Then: all safety-related fields are overridden
	assert.Equal(t, tool.Annotations.ReadOnlyHint, true)
	assert.Equal(t, tool.Annotations.IdempotentHint, true)
	assert.Assert(t, tool.Annotations.DestructiveHint != nil)
	assert.Equal(t, *tool.Annotations.DestructiveHint, false)
	assert.Assert(t, tool.Annotations.OpenWorldHint != nil)
	assert.Equal(t, *tool.Annotations.OpenWorldHint, false)
}

func TestApplyConfiguredToolHintOverridesPrefersForceSafeToolHints(t *testing.T) {
	// Given: a gateway with both override flags enabled
	forceRO := true
	forceSafe := true
	destructive := true
	openWorld := true
	tool := &mcp.Tool{
		Name: "test-tool",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
		},
	}
	gateway := &config.GatewayConfig{
		ForceReadOnlyHints: &forceRO,
		ForceSafeToolHints: &forceSafe,
	}

	// When: applying configured overrides
	applyConfiguredToolHintOverrides(tool, gateway)

	// Then: the stronger force-safe override wins
	assert.Equal(t, tool.Annotations.ReadOnlyHint, true)
	assert.Equal(t, tool.Annotations.IdempotentHint, true)
	assert.Assert(t, tool.Annotations.DestructiveHint != nil)
	assert.Equal(t, *tool.Annotations.DestructiveHint, false)
	assert.Assert(t, tool.Annotations.OpenWorldHint != nil)
	assert.Equal(t, *tool.Annotations.OpenWorldHint, false)
}
