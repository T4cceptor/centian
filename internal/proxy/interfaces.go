package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file contains the small interfaces used to isolate endpoint processing
// and downstream connection behavior for testing.

// ProcessingControllerInterface abstracts event processing for testability.
type ProcessingControllerInterface interface {
	Process(event CallContext) error
}

// DownstreamConnectionInterface abstracts downstream MCP server connections for testability.
//
// The actual connection lives in DownstreamConnection.
type DownstreamConnectionInterface interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error)
	GetServerName() string
	IsConnected() bool
	IsConnecting() bool
	IsDisconnected() bool
	IsFailed() bool
	IsPending() bool
	Tools() []*mcp.Tool
	Close() error
	GetConfig() *config.MCPServerConfig
	GetStatus() ConnectionStatus
	GetError() error
	Connect(context.Context, *DownstreamConnectOptions) error
	SyncClientState(context.Context, *DownstreamClientState) error
}

// Compile-time interface compliance checks.
var (
	_ ProcessingControllerInterface = (*ProcessingController)(nil)
	_ DownstreamConnectionInterface = (*DownstreamConnection)(nil)
)
