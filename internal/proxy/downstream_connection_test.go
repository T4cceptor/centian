package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestHeaderRoundTripperRoundTrip(t *testing.T) {
	// Given: a test server and header round tripper
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt := HeaderRoundTripper{
		Headers: map[string]string{"X-Test": "value"},
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	assert.NilError(t, err)

	// When: performing the round trip
	resp, err := rt.RoundTrip(request)

	// Then: header is added and request succeeds
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	headers := <-received
	assert.Equal(t, headers.Get("X-Test"), "value")
}

func TestCreateTransport_HTTP(t *testing.T) {
	// Given: a downstream connection configured with HTTP URL
	cfg := &config.MCPServerConfig{
		URL: "https://example.com/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer config",
		},
	}
	dc := NewDownstreamConnection("server", cfg)
	authHeaders := map[string]string{"Authorization": "Bearer auth", "X-Extra": "1"}

	// When: creating the transport
	transport, err := dc.createTransport(authHeaders)

	// Then: it returns a StreamableClientTransport with merged headers
	assert.NilError(t, err)
	streamable, ok := transport.(*mcp.StreamableClientTransport)
	assert.Assert(t, ok)
	assert.Equal(t, streamable.Endpoint, cfg.URL)

	roundTripper, ok := streamable.HTTPClient.Transport.(HeaderRoundTripper)
	assert.Assert(t, ok)
	assert.Equal(t, roundTripper.Headers["Authorization"], "Bearer auth")
	assert.Equal(t, roundTripper.Headers["X-Extra"], "1")
}

func TestCreateTransport_Stdio(t *testing.T) {
	// Given: a downstream connection configured with stdio command
	cfg := &config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env:     map[string]string{"A": "B"},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	transport, err := dc.createTransport(nil)

	// Then: it returns a CommandTransport with env set
	assert.NilError(t, err)
	cmdTransport, ok := transport.(*mcp.CommandTransport)
	assert.Assert(t, ok)
	assert.Assert(t, strings.HasPrefix(filepath.Base(cmdTransport.Command.Path), "echo"))
	assert.Assert(t, containsEnv(cmdTransport.Command.Env, "A=B"))
}

func TestCreateTransport_InvalidConfigs(t *testing.T) {
	// Given: a config with both URL and Command
	cfg := &config.MCPServerConfig{URL: "https://example.com", Command: "echo"}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	_, err := dc.createTransport(nil)

	// Then: error is returned
	assert.Assert(t, err != nil)

	// Given: a config with neither URL nor Command
	cfg = &config.MCPServerConfig{}
	dc = NewDownstreamConnection("server", cfg)

	// When: creating the transport
	_, err = dc.createTransport(nil)

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestDownstreamConnectionDefaults(t *testing.T) {
	// Given: a new downstream connection
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})

	// Then: it starts disconnected with no tools
	assert.Assert(t, !dc.IsConnected())
	assert.Assert(t, dc.Tools() == nil)

	// When: closing without a session
	err := dc.Close()

	// Then: no error is returned
	assert.NilError(t, err)
}

func containsEnv(env []string, entry string) bool {
	for _, value := range env {
		if value == entry {
			return true
		}
	}
	return false
}

func TestDownstreamConnectionConnect_EarlyReturn(t *testing.T) {
	// Given: an already connected downstream
	dc := NewDownstreamConnection("server", &config.MCPServerConfig{})
	dc.connected = true

	// When: connecting
	err := dc.Connect(context.Background(), nil)

	// Then: it returns without error
	assert.NilError(t, err)
}

func TestCreateTransport_HTTPHeaderSubstitution(t *testing.T) {
	// Given: config headers with env substitution
	os.Setenv("TEST_HEADER", "value")
	t.Cleanup(func() { os.Unsetenv("TEST_HEADER") })

	cfg := &config.MCPServerConfig{
		URL: "https://example.com/mcp",
		Headers: map[string]string{
			"X-Test": "${TEST_HEADER}",
		},
	}
	dc := NewDownstreamConnection("server", cfg)

	// When: creating the transport
	transport, err := dc.createTransport(nil)

	// Then: substituted header is used
	assert.NilError(t, err)
	streamable, ok := transport.(*mcp.StreamableClientTransport)
	assert.Assert(t, ok)
	roundTripper, ok := streamable.HTTPClient.Transport.(HeaderRoundTripper)
	assert.Assert(t, ok)
	assert.Equal(t, roundTripper.Headers["X-Test"], "value")
}
