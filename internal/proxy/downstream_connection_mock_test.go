package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MockDownstreamConnection is a shared test double for DownstreamConnectionInterface.
type MockDownstreamConnection struct {
	connected    bool
	tools        []*mcp.Tool
	cfg          *config.MCPServerConfig
	ConnectCalls int
	CloseCalls   int
	ConnectFunc  func(context.Context, map[string]string) error

	// Captured call data.
	CapturedToolName     string
	CapturedArgs         map[string]any
	CapturedConnectAuths []map[string]string

	// Configurable downstream behavior.
	ResultToReturn *mcp.CallToolResult
	ErrorToReturn  error
	Status         ConnectionStatus
}

func (m *MockDownstreamConnection) Connect(ctx context.Context, headers map[string]string) error {
	m.ConnectCalls++
	capturedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		capturedHeaders[key] = value
	}
	m.CapturedConnectAuths = append(m.CapturedConnectAuths, capturedHeaders)
	if m.ConnectFunc != nil {
		if err := m.ConnectFunc(ctx, headers); err != nil {
			m.Status = StatusFailed
			return err
		}
	}
	if m.ErrorToReturn != nil {
		m.Status = StatusFailed
		return m.ErrorToReturn
	}
	m.Status = StatusConnected
	m.connected = true
	return nil
}

func (m *MockDownstreamConnection) GetStatus() ConnectionStatus {
	return m.Status
}

func (m *MockDownstreamConnection) GetError() error {
	return m.ErrorToReturn
}

func (m *MockDownstreamConnection) CallTool(_ context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	m.CapturedToolName = toolName
	m.CapturedArgs = args
	return m.ResultToReturn, m.ErrorToReturn
}

func (m *MockDownstreamConnection) IsConnected() bool {
	return m.connected
}

func (m *MockDownstreamConnection) Tools() []*mcp.Tool {
	return m.tools
}

func (m *MockDownstreamConnection) Close() error {
	m.CloseCalls++
	m.connected = false
	return nil
}

func (m *MockDownstreamConnection) GetConfig() *config.MCPServerConfig {
	return m.cfg
}
