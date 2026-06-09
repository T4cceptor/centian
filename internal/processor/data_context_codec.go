package processor

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type processorInputDTO struct {
	Version     string              `json:"version,omitempty"`
	Event       *common.MetaContext `json:"event,omitempty"`
	Payload     *payloadPartDTO     `json:"payload,omitempty"`
	Routing     *RoutingPart        `json:"routing,omitempty"`
	Auth        *common.AuthContext `json:"auth,omitempty"`
	Annotations *AnnotationPart     `json:"annotations,omitempty"`
	ToolSurface *ToolSurfacePart    `json:"tool_surface,omitempty"`
}

type payloadPartDTO struct {
	Request         *callToolRequestDTO `json:"request,omitempty"`
	OriginalRequest *callToolRequestDTO `json:"original_request,omitempty"`
	Result          *mcp.CallToolResult `json:"result,omitempty"`
	OriginalResult  *mcp.CallToolResult `json:"original_result,omitempty"`
}

type callToolRequestDTO struct {
	Params *mcp.CallToolParamsRaw `json:"Params,omitempty"`
}

func marshalProcessorInput(input *DataContext) ([]byte, error) {
	if input == nil {
		return json.Marshal(&processorInputDTO{})
	}

	dto := &processorInputDTO{
		Version:     input.Version,
		Event:       input.Event,
		Routing:     input.Routing,
		Auth:        input.Auth,
		Annotations: input.Annotations,
		ToolSurface: input.ToolSurface,
	}

	if input.Payload != nil {
		dto.Payload = &payloadPartDTO{
			Request:         cloneRequestForDTO(input.Payload.Request),
			OriginalRequest: cloneRequestForDTO(input.Payload.OriginalRequest),
			Result:          input.Payload.Result,
			OriginalResult:  input.Payload.OriginalResult,
		}
	}

	return json.Marshal(dto)
}

func unmarshalProcessorOutput(data []byte) (*DataContext, error) {
	var output DataContext
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func cloneRequestForDTO(req *mcp.CallToolRequest) *callToolRequestDTO {
	if req == nil || req.Params == nil {
		return nil
	}

	argsCopy := make(json.RawMessage, len(req.Params.Arguments))
	copy(argsCopy, req.Params.Arguments)

	params := &mcp.CallToolParamsRaw{
		Name:      req.Params.Name,
		Arguments: argsCopy,
		Meta:      req.Params.Meta,
	}
	return &callToolRequestDTO{Params: params}
}

func expandProcessorHeaders(headers map[string]string) map[string]string {
	expanded := make(map[string]string, len(headers))
	for key, value := range headers {
		expanded[key] = os.Expand(value, os.Getenv)
	}
	return expanded
}

func decodeProcessorJSONOutput(processorName string, stdout []byte) (*DataContext, error) {
	output, err := unmarshalProcessorOutput(stdout)
	if err != nil {
		errorMsg := fmt.Sprintf("processor '%s' returned invalid JSON: %v", processorName, err)
		if len(stdout) > 0 {
			errorMsg = fmt.Sprintf("%s\nstdout: %s", errorMsg, string(stdout))
		}
		return nil, fmt.Errorf("%s", errorMsg)
	}
	return output, nil
}
