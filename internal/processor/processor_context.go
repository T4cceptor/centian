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
	Event       *common.MetaContext `json:"event,omitempty"`   // Contains event metadata
	Payload     *PayloadPart        `json:"payload,omitempty"` // Payload of the request and result
	Routing     *RoutingPart        `json:"routing,omitempty"`
	Auth        *common.AuthContext `json:"auth,omitempty"` // Auth identity and principal context
	Annotations *AnnotationPart     `json:"annotations,omitempty"`
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

// AnnotationPart lets processors report findings about an event without modifying the MCP call.
type AnnotationPart struct {
	Reports []ProcessorReport `json:"reports,omitempty"`
}

// ProcessorReport describes a processor observation or policy action.
type ProcessorReport struct {
	Processor string             `json:"processor,omitempty"`
	Action    string             `json:"action,omitempty"`
	Severity  string             `json:"severity,omitempty"`
	Message   string             `json:"message,omitempty"`
	Findings  []ProcessorFinding `json:"findings,omitempty"`
}

// ProcessorFinding identifies a specific processor finding.
type ProcessorFinding struct {
	Rule string `json:"rule,omitempty"`
	Path string `json:"path,omitempty"`
}

// TODO: add headers part
