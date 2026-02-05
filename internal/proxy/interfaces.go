package proxy

import (
	"context"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EventProcessorInterface abstracts event processing for testability.
type EventProcessorInterface interface {
	Process(event common.McpEventInterface) error
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
	_ EventProcessorInterface       = (*EventProcessor)(nil)
	_ DownstreamConnectionInterface = (*DownstreamConnection)(nil)
)
