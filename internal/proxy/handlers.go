package proxy

import (
	"encoding/json"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CallContextHandler handles a specific part of the processing context.
// Each handler knows how to extract its part for processor input (Get)
// and apply processor output back to CallContext (Apply).
type CallContextHandler interface {
	// Get extracts this handler's part from CallContext for processor input
	Get(callCtx CallContext) any

	// Apply updates CallContext based on processor output for this part
	Apply(callCtx CallContext, result map[string]any) error
}

// LogHandler defines how CallContext is serialized for logging.
// Different implementations allow different log formats.
type LogHandler interface {
	// ToLogEntry creates a loggable representation of the current state
	ToLogEntry(callCtx CallContext) any
}

// =============================================================================
// PayloadHandler - handles tool arguments (request) and result content (response)
// =============================================================================

type PayloadHandler struct{}

func (h *PayloadHandler) Get(callCtx CallContext) any {
	if callCtx.GetDirection() == DirectionRequest {
		// Return arguments from request
		var args map[string]any
		if req := callCtx.GetRequest(); req != nil && req.Params != nil {
			_ = json.Unmarshal(req.Params.Arguments, &args)
		}
		return map[string]any{
			"arguments": args,
			"tool_name": callCtx.GetToolName(),
		}
	}
	// Response direction - return result content
	result := callCtx.GetResult()
	if result == nil {
		return map[string]any{
			"content":  []any{},
			"is_error": false,
		}
	}
	return map[string]any{
		"content":  resultToContentBlocks(result),
		"is_error": result.IsError,
	}
}

func (h *PayloadHandler) Apply(callCtx CallContext, result map[string]any) error {
	payload, ok := result["payload"].(map[string]any)
	if !ok {
		return nil // No payload changes
	}

	if callCtx.GetDirection() == DirectionRequest {
		// Apply argument changes
		if args, ok := payload["arguments"].(map[string]any); ok {
			argsJSON, err := json.Marshal(args)
			if err != nil {
				return err
			}
			callCtx.GetRequest().Params.Arguments = argsJSON
		}
		// Apply tool name changes
		if name, ok := payload["tool_name"].(string); ok && name != "" {
			callCtx.GetRequest().Params.Name = name
		}
	} else {
		// Apply result changes
		if content, ok := payload["content"].([]any); ok {
			isError := false
			if ie, ok := payload["is_error"].(bool); ok {
				isError = ie
			}
			callCtx.SetResult(contentBlocksToResult(content, isError))
		}
	}
	return nil
}

// resultToContentBlocks converts mcp.CallToolResult to a serializable format
func resultToContentBlocks(result *mcp.CallToolResult) []map[string]any {
	if result == nil {
		return nil
	}
	blocks := make([]map[string]any, 0, len(result.Content))
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": c.Text,
			})
		case *mcp.ImageContent:
			blocks = append(blocks, map[string]any{
				"type":      "image",
				"data":      string(c.Data),
				"mime_type": c.MIMEType,
			})
		}
	}
	return blocks
}

// contentBlocksToResult converts serializable content blocks back to mcp.CallToolResult
func contentBlocksToResult(blocks []any, isError bool) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		IsError: isError,
		Content: make([]mcp.Content, 0, len(blocks)),
	}
	for _, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		contentType, _ := blockMap["type"].(string)
		switch contentType {
		case "text":
			if text, ok := blockMap["text"].(string); ok {
				result.Content = append(result.Content, &mcp.TextContent{Text: text})
			}
		case "image":
			data, _ := blockMap["data"].(string)
			mimeType, _ := blockMap["mime_type"].(string)
			result.Content = append(result.Content, &mcp.ImageContent{
				Data:     []byte(data),
				MIMEType: mimeType,
			})
		}
	}
	return result
}

// =============================================================================
// MetaHandler - provides read-only metadata to processors
// =============================================================================

type MetaHandler struct{}

func (h *MetaHandler) Get(callCtx CallContext) any {
	meta := map[string]any{
		"direction":             string(callCtx.GetDirection()),
		"timestamp":             time.Now().Format(time.RFC3339),
		"server_name":           callCtx.GetServerName(),
		"original_server_name":  callCtx.GetOriginalServerName(),
		"tool_name":             callCtx.GetToolName(),
		"original_tool_name":    callCtx.GetOriginalToolName(),
	}

	// Add routing context if available
	if rc := callCtx.GetRoutingContext(); rc != nil {
		meta["routing"] = map[string]any{
			"transport":    string(rc.Transport),
			"gateway":      rc.Gateway,
			"endpoint":     rc.Endpoint,
			"downstream":   rc.DownstreamURL,
		}
	}

	return meta
}

func (h *MetaHandler) Apply(callCtx CallContext, result map[string]any) error {
	// Meta is mostly read-only, but can handle status changes from processors
	meta, ok := result["meta"].(map[string]any)
	if !ok {
		return nil
	}

	// Handle status code changes
	if status, ok := meta["status"].(float64); ok {
		callCtx.SetStatus(int(status))
	}

	// Handle error message
	if errMsg, ok := meta["error"].(string); ok {
		callCtx.SetError(errMsg)
	}

	return nil
}

// =============================================================================
// RoutingHandler - handles server name and routing decisions
// =============================================================================

type RoutingHandler struct{}

func (h *RoutingHandler) Get(callCtx CallContext) any {
	return map[string]any{
		"server_name":          callCtx.GetServerName(),
		"original_server_name": callCtx.GetOriginalServerName(),
		"tool_name":            callCtx.GetToolName(),
		"original_tool_name":   callCtx.GetOriginalToolName(),
	}
}

func (h *RoutingHandler) Apply(callCtx CallContext, result map[string]any) error {
	routing, ok := result["routing"].(map[string]any)
	if !ok {
		return nil
	}

	// Apply server name changes (re-routing)
	if serverName, ok := routing["server_name"].(string); ok && serverName != "" {
		callCtx.SetServerName(serverName)
	}

	// Apply tool name changes (aliasing)
	if toolName, ok := routing["tool_name"].(string); ok && toolName != "" {
		callCtx.GetRequest().Params.Name = toolName
	}

	return nil
}

// =============================================================================
// DefaultLogHandler - produces MCPEvent-compatible log entries
// =============================================================================

type DefaultLogHandler struct {
	RedactHeaders []string
}

func NewDefaultLogHandler() *DefaultLogHandler {
	return &DefaultLogHandler{
		RedactHeaders: []string{"Authorization", "X-API-Key", "X-Auth-Token"},
	}
}

func (h *DefaultLogHandler) ToLogEntry(callCtx CallContext) any {
	direction := common.DirectionClientToServer
	if callCtx.GetDirection() == DirectionResponse {
		direction = common.DirectionServerToClient
	}

	msgType := common.MessageTypeRequest
	if callCtx.GetDirection() == DirectionResponse {
		msgType = common.MessageTypeResponse
	}

	event := &common.MCPEvent{
		BaseMcpEvent: common.BaseMcpEvent{
			Timestamp:   time.Now(),
			Transport:   h.getTransport(callCtx),
			RequestID:   callCtx.GetRequestID(),
			SessionID:   callCtx.GetSessionID(),
			Direction:   direction,
			MessageType: msgType,
			Status:      callCtx.GetStatus(),
			Success:     callCtx.GetStatus() < 400,
		},
	}

	// Add routing context
	if rc := callCtx.GetRoutingContext(); rc != nil {
		event.Routing = *rc
	}

	// Add tool call context
	event.ToolCall = &common.ToolCallContext{
		Name:         callCtx.GetToolName(),
		OriginalName: callCtx.GetOriginalToolName(),
	}

	// Add arguments for request
	if callCtx.GetDirection() == DirectionRequest {
		if req := callCtx.GetRequest(); req != nil && req.Params != nil {
			event.ToolCall.Arguments = req.Params.Arguments
		}
	}

	// Add result for response
	if callCtx.GetDirection() == DirectionResponse {
		if result := callCtx.GetResult(); result != nil {
			resultJSON, _ := json.Marshal(result)
			event.ToolCall.Result = resultJSON
			event.ToolCall.IsError = result.IsError
		}
	}

	return event
}

func (h *DefaultLogHandler) getTransport(callCtx CallContext) string {
	if rc := callCtx.GetRoutingContext(); rc != nil {
		return string(rc.Transport)
	}
	return "unknown"
}