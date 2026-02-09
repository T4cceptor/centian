package processor

import (
	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProcessorContext is passed to processors as JSON
type ProcessorContext struct {
	Version   string `json:"version"`   // "1.0" - for evolution
	Direction string `json:"direction"` // "request" | "response" // TODO: this is already in "Event"

	// Parts - only populated based on processor config
	Event   *common.MCPEvent `json:"event,omitempty"`   // Contains event metadata
	Payload *PayloadPart     `json:"payload,omitempty"` // Payload of the request and result
	Routing *RoutingPart     `json:"routing,omitempty"`
	// Future: Headers, Auth, etc.
}

type PayloadPart struct {
	Request         *mcp.CallToolRequest `json:"request,omitempty"`          // The processed mcp.CallToolRequest
	OriginalRequest *mcp.CallToolRequest `json:"original_request,omitempty"` // The original mcp.CallToolRequest coming from upstream client
	Result          *mcp.CallToolResult  `json:"result,omitempty"`           // The processed mcp.CallToolResult
	OriginalResult  *mcp.CallToolResult  `json:"original_result,omitempty"`  // The original mcp.CallToolResult coming from downstream MCP
}

type RoutingPart struct {
	ServerName         string `json:"server_name"`
	ToolName           string `json:"tool_name"`
	OriginalServerName string `json:"original_server_name"`
	OriginalToolname   string `json:"original_tool_name"`
}

// TODO: add headers part
