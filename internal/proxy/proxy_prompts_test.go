package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

type promptForwardConn struct {
	*MockDownstreamConnection
	result       *mcp.GetPromptResult
	err          error
	capturedName string
	capturedArgs map[string]string
}

func (c *promptForwardConn) GetPrompt(_ context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	c.capturedName = name
	c.capturedArgs = args
	return c.result, c.err
}

func TestRegisterPromptRegistersOnce(t *testing.T) {
	proxy := newLoggingTestProxy()
	session := newLoggingTestSession(proxy, "session-1")
	prompt := &mcp.Prompt{
		Name:        "review",
		Description: "Review code",
		Arguments: []*mcp.PromptArgument{
			{Name: "path", Required: true},
		},
	}

	proxy.registerPrompt(session, "server-a", prompt, "server-a___review")
	proxy.registerPrompt(session, "server-a", prompt, "server-a___review")

	assert.Equal(t, len(session.registeredPrompts), 1)
	_, ok := session.registeredPrompts["server-a___review"]
	assert.Assert(t, ok)
}

func TestForwardGetPromptReturnsDownstreamPrompt(t *testing.T) {
	conn := &promptForwardConn{
		MockDownstreamConnection: &MockDownstreamConnection{
			serverName: "server-a",
			Status:     StatusConnected,
		},
		result: &mcp.GetPromptResult{Description: "prompt"},
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardGetPrompt(context.Background(), session, "server-a", "review", &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "ignored-upstream",
			Arguments: map[string]string{"path": "/tmp/file.go"},
		},
	})

	assert.NilError(t, err)
	assert.Equal(t, result.Description, "prompt")
	assert.Equal(t, conn.capturedName, "review")
	assert.DeepEqual(t, conn.capturedArgs, map[string]string{"path": "/tmp/file.go"})
}

func TestForwardGetPromptNormalizesDownstreamMethodError(t *testing.T) {
	conn := &promptForwardConn{
		MockDownstreamConnection: &MockDownstreamConnection{
			serverName: "server-a",
			Status:     StatusConnected,
		},
		err: errors.New(`calling "prompts/get": method not found`),
	}
	proxy := &CentianEndpoint{name: "test"}
	session := &UpstreamSession{
		downstreamConns: map[string]DownstreamConnectionInterface{
			"server-a": conn,
		},
	}

	result, err := proxy.forwardGetPrompt(context.Background(), session, "server-a", "review", &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{},
	})

	assert.Assert(t, result == nil)
	assert.Equal(t, err.Error(), "method not found")
}
