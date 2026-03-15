package proxy

import (
	"context"
	"sync"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MockDownstreamConnection is a shared test double for DownstreamConnectionInterface.
type MockDownstreamConnection struct {
	mu           sync.RWMutex
	serverName   string
	tools        []*mcp.Tool
	resources    []*mcp.Resource
	prompts      []*mcp.Prompt
	cfg          *config.MCPServerConfig
	ConnectCalls int
	CloseCalls   int
	ConnectFunc  func(context.Context, *DownstreamConnectOptions) error

	// Captured call data.
	CapturedToolName     string
	CapturedArgs         map[string]any
	CapturedConnects     []DownstreamConnectOptions
	CapturedConnectAuths []map[string]string

	// Configurable downstream behavior.
	ResultToReturn *mcp.CallToolResult
	ErrorToReturn  error
	Status         ConnectionStatus
}

func (m *MockDownstreamConnection) GetServerName() string {
	if m.serverName != "" {
		return m.serverName
	}
	return "mock-server"
}

func (m *MockDownstreamConnection) Connect(ctx context.Context, options *DownstreamConnectOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ConnectCalls++
	m.CapturedConnects = append(m.CapturedConnects, cloneConnectOptions(options))
	m.CapturedConnectAuths = append(m.CapturedConnectAuths, cloneAuthHeaders(options.ForwardedHeaders))
	if m.ConnectFunc != nil {
		if err := m.ConnectFunc(ctx, options); err != nil {
			m.Status = StatusFailed
			return err
		}
	}
	if m.ErrorToReturn != nil {
		m.Status = StatusFailed
		return m.ErrorToReturn
	}
	m.Status = StatusConnected
	return nil
}

func (m *MockDownstreamConnection) GetStatus() ConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status
}

func (m *MockDownstreamConnection) GetError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ErrorToReturn
}

func (m *MockDownstreamConnection) CallTool(_ context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CapturedToolName = toolName
	m.CapturedArgs = args
	return m.ResultToReturn, m.ErrorToReturn
}

func (m *MockDownstreamConnection) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status.IsConnected()
}

func (m *MockDownstreamConnection) IsConnecting() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status.IsConnecting()
}

func (m *MockDownstreamConnection) IsDisconnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status.IsDisconnected()
}

func (m *MockDownstreamConnection) IsFailed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status.IsFailed()
}

func (m *MockDownstreamConnection) IsPending() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status.IsPending()
}

func (m *MockDownstreamConnection) Tools() []*mcp.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

func (m *MockDownstreamConnection) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCalls++
	m.Status = StatusDisconnected
	return nil
}

func (m *MockDownstreamConnection) GetConfig() *config.MCPServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *MockDownstreamConnection) Resources() []*mcp.Resource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resources
}

func (m *MockDownstreamConnection) Prompts() []*mcp.Prompt {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prompts
}

func (m *MockDownstreamConnection) DiscoverResources(_ context.Context) error {
	return nil
}

func (m *MockDownstreamConnection) DiscoverPrompts(_ context.Context) error {
	return nil
}

func (m *MockDownstreamConnection) ReadResource(_ context.Context, _ string) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{}, nil
}

func (m *MockDownstreamConnection) GetPrompt(_ context.Context, _ string, _ map[string]string) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{}, nil
}

func (m *MockDownstreamConnection) Complete(_ context.Context, _ *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return &mcp.CompleteResult{}, nil
}

func (m *MockDownstreamConnection) Subscribe(_ context.Context, _ string) error {
	return nil
}

func (m *MockDownstreamConnection) Unsubscribe(_ context.Context, _ string) error {
	return nil
}

func (m *MockDownstreamConnection) SyncClientState(_ context.Context, state *DownstreamClientState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.CapturedConnects) == 0 {
		m.CapturedConnects = append(m.CapturedConnects, DownstreamConnectOptions{})
	}
	last := m.CapturedConnects[len(m.CapturedConnects)-1]
	if state != nil {
		last.ClientState = *state
	}
	m.CapturedConnects[len(m.CapturedConnects)-1] = last
	return nil
}

func cloneConnectOptions(options *DownstreamConnectOptions) DownstreamConnectOptions {
	if options == nil {
		return DownstreamConnectOptions{}
	}
	cloned := DownstreamConnectOptions{
		ForwardedHeaders:   cloneAuthHeaders(options.ForwardedHeaders),
		SamplingHandler:    options.SamplingHandler,
		ElicitationHandler: options.ElicitationHandler,
	}
	cloned.ClientState = DownstreamClientState{
		ProtocolVersion:         options.ClientState.ProtocolVersion,
		ClientCapabilities:      cloneClientCapabilities(options.ClientState.ClientCapabilities),
		Roots:                   normalizeRoots(options.ClientState.Roots),
		CapabilitiesFingerprint: options.ClientState.CapabilitiesFingerprint,
		RootsFingerprint:        options.ClientState.RootsFingerprint,
	}
	return cloned
}
