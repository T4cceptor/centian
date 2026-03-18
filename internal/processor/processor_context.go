package processor

import (
	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CurrentDataContextVersion is the data contract version sent to processors.
// Follows major.minor semantics: a major bump is breaking, a minor bump is additive
// and backward-compatible (processors that ignore unknown fields remain functional).
const CurrentDataContextVersion = "1.0"

// DataContext is passed to processors as JSON.
// Holds all relevant information of a CallContext for processors to access and modify.
type DataContext struct {
	Version string `json:"version"` // "1.0" - for evolution

	// Parts - only populated based on processor config
	Event   *common.MCPEvent    `json:"event,omitempty"`   // Contains event metadata
	Payload *PayloadPart        `json:"payload,omitempty"` // Payload of the request and result
	Routing *RoutingPart        `json:"routing,omitempty"`
	Auth    *common.AuthContext `json:"auth,omitempty"` // Auth identity and principal context
	// Future: Headers, etc.
}

// PayloadPart holds payload information in ProcessorContext.
type PayloadPart struct {
	Request         *mcp.CallToolRequest `json:"request,omitempty"`          // The processed mcp.CallToolRequest
	OriginalRequest *mcp.CallToolRequest `json:"original_request,omitempty"` // The original mcp.CallToolRequest coming from upstream client
	Result          *mcp.CallToolResult  `json:"result,omitempty"`           // The processed mcp.CallToolResult
	OriginalResult  *mcp.CallToolResult  `json:"original_result,omitempty"`  // The original mcp.CallToolResult coming from downstream MCP
}

// RoutingPart holds routing information in ProcessorContext.
type RoutingPart struct {
	ServerName         string `json:"server_name"`
	ToolName           string `json:"tool_name"`
	OriginalServerName string `json:"original_server_name"`
	OriginalToolname   string `json:"original_tool_name"`
}

// TODO: add headers part
