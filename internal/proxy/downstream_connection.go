package proxy

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownstreamConnection struct {
	serverName string
	config     *config.MCPServerConfig

	client    *mcp.Client
	session   *mcp.ClientSession
	tools     []*mcp.Tool
	resources []*mcp.Resource // If we support resources

	connected bool
	mu        sync.RWMutex
}

// NewDownstreamConnection creates an unconnected downstream wrapper
func NewDownstreamConnection(name string, cfg *config.MCPServerConfig) *DownstreamConnection {
	return &DownstreamConnection{
		serverName: name,
		config:     cfg,
		connected:  false,
	}
}

// Connect establishes connection to downstream server
// authHeaders: additional headers from upstream request (for passthrough auth)
func (dc *DownstreamConnection) Connect(ctx context.Context, authHeaders map[string]string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.connected {
		return nil // Already connected
	}

	dc.client = mcp.NewClient(&mcp.Implementation{
		Name:    dc.serverName,
		Version: "1.0.0", // TODO: replace with gateway version
	}, nil)

	transport, err := dc.createTransport(authHeaders)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}

	// TODO: logging & processing -> we are now connecting to downstream server
	session, err := dc.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	dc.session = session

	// Discover tools // TODO: resources
	if err := dc.discoverTools(ctx); err != nil {
		dc.session.Close()
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	dc.connected = true
	return nil
}

type HeaderRoundTripper struct {
	Base    http.RoundTripper
	Headers map[string]string
}

func (rt HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}
	// Clone to avoid mutating the original request.
	cloned := req.Clone(req.Context())
	for k, v := range rt.Headers {
		// Use Set to overwrite, or Add to append.
		cloned.Header.Set(k, v)
	}
	// TODO: raw request processing
	resp, err := base.RoundTrip(cloned)
	// TODO: raw response processing
	return resp, err
}

func (dc *DownstreamConnection) createTransport(authHeaders map[string]string) (mcp.Transport, error) {
	isHTTPtransport := dc.config.URL != ""
	isStdioTransport := dc.config.Command != ""
	if isHTTPtransport && isStdioTransport {
		return nil, fmt.Errorf("both URL or Command configured for server %s", dc.serverName)
	}

	if isHTTPtransport {
		// Merge config headers with passed auth headers
		allHeaders := make(map[string]string)
		for k, v := range dc.config.Headers {
			allHeaders[k] = v
		}
		for k, v := range authHeaders {
			allHeaders[k] = v // Auth headers override config
		}

		// HTTP transport
		httpClient := &http.Client{
			Transport: HeaderRoundTripper{
				Headers: allHeaders,
			},
			Timeout: 30 * time.Second,
		}

		// TODO: Add header injection to StreamableClientTransport
		// This requires a custom RoundTripper
		transport := &mcp.StreamableClientTransport{
			Endpoint:   dc.config.URL,
			HTTPClient: httpClient,
		}
		return transport, nil
	}

	if isStdioTransport {
		// Stdio transport
		cmd := exec.Command(dc.config.Command, dc.config.Args...)
		// Add environment variables
		for k, v := range dc.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	}

	return nil, fmt.Errorf("no URL or Command configured for server %s", dc.serverName)
}

func (dc *DownstreamConnection) discoverTools(ctx context.Context) error {
	result, err := dc.session.ListTools(ctx, nil)
	if err != nil {
		return err
	}

	// TODO: logging & processing
	// -> tool aggregation/federation
	// we could for example not provide ALL tools, but allow the
	// agent to search for a specific tool

	dc.tools = result.Tools
	return nil
}

// CallTool forwards a tool call to the downstream server
func (dc *DownstreamConnection) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if !dc.connected || dc.session == nil {
		return nil, fmt.Errorf("not connected to %s", dc.serverName)
	}

	// TODO: logging & processing - pre-tool call
	result, err := dc.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	// TODO: logging & processing - post-tool call
	return result, err
}

// Close terminates the downstream connection
func (dc *DownstreamConnection) Close() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.session != nil {
		// TODO: log
		dc.session.Close()
	}
	dc.connected = false
	return nil
}

// Tools returns the cached tools (nil if not connected)
func (dc *DownstreamConnection) Tools() []*mcp.Tool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	// TODO: check if we should refresh the tools
	return dc.tools
}

func (dc *DownstreamConnection) IsConnected() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.connected
}
