package proxy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type plainError string

func (e plainError) Error() string { return string(e) }

type readResourceErrorConn struct {
	*MockDownstreamConnection
	err error
}

func (c *readResourceErrorConn) ReadResource(_ context.Context, _ string) (*mcp.ReadResourceResult, error) {
	return nil, c.err
}

func TestNormalizeForwardedMethodError(t *testing.T) {
	t.Run("unwraps matching sdk wrapper", func(t *testing.T) {
		inner := errors.New("validation failed")
		err := fmt.Errorf("calling %q: %w", mcpMethodCallTool, inner)

		got := normalizeForwardedMethodError(mcpMethodCallTool, err)

		assert.Assert(t, errors.Is(got, inner))
	})

	t.Run("unwraps matching non tool method", func(t *testing.T) {
		inner := errors.New("resource missing")
		err := fmt.Errorf("calling %q: %w", mcpMethodReadResource, inner)

		got := normalizeForwardedMethodError(mcpMethodReadResource, err)

		assert.Assert(t, errors.Is(got, inner))
	})

	t.Run("leaves mismatched method untouched", func(t *testing.T) {
		inner := errors.New("validation failed")
		err := fmt.Errorf("calling %q: %w", mcpMethodCallTool, inner)

		got := normalizeForwardedMethodError(mcpMethodReadResource, err)

		assert.Equal(t, got.Error(), err.Error())
	})

	t.Run("leaves unrelated errors untouched", func(t *testing.T) {
		err := errors.New("plain downstream error")

		got := normalizeForwardedMethodError(mcpMethodCallTool, err)

		assert.Assert(t, errors.Is(got, err))
	})

	t.Run("falls back to trimming one matching prefix when unwrapping is unavailable", func(t *testing.T) {
		err := plainError(`calling "resources/read": resource missing`)

		got := normalizeForwardedMethodError(mcpMethodReadResource, err)

		assert.Equal(t, got.Error(), "resource missing")
	})
}

func TestToolCallContextSendRequestNormalizesDownstreamMethodWrapper(t *testing.T) {
	inner := errors.New("validation failed")
	toolCtx := &ToolCallContext{
		proxy:              &CentianEndpoint{},
		originalServerName: "orig-srv",
		upstreamSession: &UpstreamSession{
			downstreamConns: map[string]DownstreamConnectionInterface{
				"srv": &MockDownstreamConnection{
					cfg:           &config.MCPServerConfig{Command: "python3"},
					Status:        StatusConnected,
					ErrorToReturn: fmt.Errorf("calling %q: %w", mcpMethodCallTool, inner),
				},
			},
		},
		routingContext: &common.RoutingContext{ServerName: "srv"},
		request: &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "tool_name",
				Arguments: []byte(`{"a":1}`),
			},
		},
		event: common.NewMCPRequestEvent("stdio"),
	}

	err := toolCtx.SendRequest(context.Background())

	assert.Assert(t, errors.Is(err, inner))
}

func TestForwardReadResourceNormalizesDownstreamMethodWrapper(t *testing.T) {
	inner := errors.New("resource missing")
	conn := &readResourceErrorConn{
		MockDownstreamConnection: &MockDownstreamConnection{
			serverName: "srv",
			Status:     StatusConnected,
		},
		err: fmt.Errorf("calling %q: %w", mcpMethodReadResource, inner),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"srv": conn,
		},
	}

	_, err := proxy.forwardReadResource(context.Background(), session, "srv", "file:///test")

	assert.Assert(t, errors.Is(err, inner))
}
