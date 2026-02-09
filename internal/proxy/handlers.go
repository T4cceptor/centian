package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CallContextHandler handles a specific part of the processing context.
// Each handler knows how to extract its part for processor input (Get)
// and apply processor output back to CallContext (Apply).
type CallContextHandler interface {
	// Get extracts this handler's part from CallContext for processor input
	Get(callCtx CallContext, input *processor.ProcessorContext)

	// Apply updates CallContext based on processor output for this part
	Apply(callCtx CallContext, output *processor.ProcessorContext) error
}

// LogHandler defines how CallContext is serialized for logging.
// Different implementations allow different log formats.
type LogHandler interface {
	// ToLogEntry creates a loggable representation of the current state
	ToLogEntry(callCtx CallContext) any
	Log(callCtx CallContext) error
}

// =============================================================================
// PayloadHandler - handles tool arguments (request) and result content (response)
// =============================================================================

type DefaultPayloadHandler struct{}

func (h *DefaultPayloadHandler) Get(callCtx CallContext, input *processor.ProcessorContext) {
	payload := processor.PayloadPart{
		Request:         callCtx.GetRequest(),
		OriginalRequest: callCtx.GetOriginalRequest(),
		Result:          callCtx.GetResult(),
		OriginalResult:  callCtx.GetOriginalResult(),
	}
	input.Payload = &payload
}

func (h *DefaultPayloadHandler) Apply(callCtx CallContext, result *processor.ProcessorContext) error {
	payload := result.Payload
	if payload == nil {
		return nil // No payload changes
	}

	// Modifying request
	req := callCtx.GetRequest()
	if req != nil && payload.Request != nil && payload.Request.Params != nil {
		req.Params = payload.Request.Params
	}

	if payload.Result != nil {
		callCtx.SetResult(payload.Result)
	}
	return nil
}

// =============================================================================
// MetaHandler - provides metadata to processors
// =============================================================================

type DefaultMetaHandler struct{}

func (h *DefaultMetaHandler) Get(callCtx CallContext, input *processor.ProcessorContext) {
	input.Event = callCtx.GetEventInfo()
	// TODO: slight issue here: we are attaching a reference to the callCtx
	// - this can result in processors changing the attached MCPEvent before calling Apply
	// Better: clone MCPEvent
}

func (h *DefaultMetaHandler) Apply(callCtx CallContext, result *processor.ProcessorContext) error {
	if result.Event != nil {
		callCtx.SetEventInfo(result.Event)
	}
	return nil
}

// =============================================================================
// RoutingHandler - handles server name and routing decisions
// =============================================================================

type DefaultRoutingHandler struct{}

func (h *DefaultRoutingHandler) Get(callCtx CallContext, input *processor.ProcessorContext) {
	if input == nil || callCtx == nil {
		return // nothing to do
	}
	input.Routing = &processor.RoutingPart{
		ServerName:         callCtx.GetServerName(),
		ToolName:           callCtx.GetToolName(),
		OriginalServerName: callCtx.GetOriginalServerName(),
		OriginalToolname:   callCtx.GetOriginalToolName(),
	}
}

func (h *DefaultRoutingHandler) Apply(callCtx CallContext, result *processor.ProcessorContext) error {
	if result.Routing == nil {
		return nil // no routing received, nothing to apply
	}
	if result.Routing.ServerName != "" {
		callCtx.SetServerName(result.Routing.ServerName)
	}
	if result.Routing.ToolName != "" {
		req := callCtx.GetRequest()
		if req == nil {
			return fmt.Errorf("call context does not contain request")
		}
		if req.Params == nil {
			req.Params = &mcp.CallToolParamsRaw{}
		}
		req.Params.Name = result.Routing.ToolName
	}
	return nil
}

// =============================================================================
// DefaultLogHandler - produces MCPEvent-compatible log entries
// =============================================================================

type DefaultLogHandler struct {
	// TODO: enable data redaction in logs -> new logger
	logger *logging.Logger
}

func NewDefaultLogHandler(logger *logging.Logger) *DefaultLogHandler {
	return &DefaultLogHandler{
		logger: logger,
	}
}

func (h *DefaultLogHandler) Log(callCtx CallContext) error {
	return h.logger.LogEntry(h.ToLogEntry(callCtx))
}

func (h *DefaultLogHandler) ToLogEntry(callCtx CallContext) any {
	direction := callCtx.GetDirection()
	msgType := callCtx.GetMessageType()
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
		// TODO
		event.Routing = *rc
	}

	// Add tool call context
	event.ToolCall = &common.ToolCallLog{
		Name:         callCtx.GetToolName(),
		OriginalName: callCtx.GetOriginalToolName(),
	}

	// Add arguments for request
	if msgType == common.MessageTypeRequest {
		if req := callCtx.GetRequest(); req != nil && req.Params != nil {
			event.ToolCall.Arguments = req.Params.Arguments
		}
	}

	// Add result for response
	if msgType == common.MessageTypeResponse {
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
