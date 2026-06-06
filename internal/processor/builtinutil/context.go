package builtinutil

import (
	"encoding/json"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DataContext mirrors the processor JSON contract for built-in processors.
// It intentionally lives outside internal/processor to avoid a dispatch import cycle.
type DataContext struct {
	Version     string              `json:"version"`
	Event       *common.MetaContext `json:"event,omitempty"`
	Payload     *PayloadPart        `json:"payload,omitempty"`
	Routing     *RoutingPart        `json:"routing,omitempty"`
	Auth        *common.AuthContext `json:"auth,omitempty"`
	Annotations *AnnotationPart     `json:"annotations,omitempty"`
}

// PayloadPart holds tool-call request/result payloads.
type PayloadPart struct {
	Request         *mcp.CallToolRequest `json:"request,omitempty"`
	OriginalRequest *mcp.CallToolRequest `json:"original_request,omitempty"`
	Result          *mcp.CallToolResult  `json:"result,omitempty"`
	OriginalResult  *mcp.CallToolResult  `json:"original_result,omitempty"`
}

// RoutingPart holds routing data for a proxied call.
type RoutingPart struct {
	ServerName         string `json:"server_name"`
	ToolName           string `json:"tool_name"`
	OriginalServerName string `json:"original_server_name"`
	OriginalToolname   string `json:"original_tool_name"`
}

// AnnotationPart contains processor-supplied event reports.
type AnnotationPart struct {
	Reports []common.EventAnnotation `json:"reports,omitempty"`
}

// DecodeContext decodes one serialized built-in processor DataContext.
func DecodeContext(input []byte) (*DataContext, error) {
	var ctx DataContext
	if err := json.Unmarshal(input, &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// EncodeContext encodes a built-in processor DataContext.
func EncodeContext(ctx *DataContext) ([]byte, error) {
	if ctx == nil {
		ctx = &DataContext{}
	}
	return json.Marshal(ctx)
}
