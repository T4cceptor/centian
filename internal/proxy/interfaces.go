package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProcessingControllerInterface abstracts event processing for testability.
type ProcessingControllerInterface interface {
	Process(event CallContext) error
}

// DownstreamConnectionInterface abstracts downstream MCP server connections for testability.
type DownstreamConnectionInterface interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error)
	IsConnected() bool
	Tools() []*mcp.Tool
	Close() error
	GetConfig() *config.MCPServerConfig
}

// Compile-time interface compliance checks.
var (
	_ ProcessingControllerInterface = (*ProcessingController)(nil)
	_ DownstreamConnectionInterface = (*DownstreamConnection)(nil)
)
