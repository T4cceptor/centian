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
	// AttachPart extracts this handler's part from CallContext for processor input
	AttachPart(callCtx CallContext, input *processor.DataContext)

	// Apply updates CallContext based on processor output for this part
	Apply(callCtx CallContext, output *processor.DataContext) error
}

// LogHandler defines how CallContext is serialized for logging.
// Different implementations allow different log formats.
type LogHandler interface {
	// ToLogEntry creates a loggable representation of the current state
	ToLogEntry(callCtx CallContext) *common.MCPEvent
	Log(callCtx CallContext) error
}

// =============================================================================
// PayloadHandler - handles tool arguments (request) and result content (response)
// =============================================================================

// DefaultPayloadHandler provides payload information for request and result,
// as well as applies modifications to those provided by a processor.
//
// Note: DefaultPayloadHandler does a full replacement of request params and result, requiring the processors
// to return an exact copy of the structure. Partial replacement is not possible with this handlers.
type DefaultPayloadHandler struct{}

// AttachPart attaches Payload information from CallContext on input ProcessorContext.
func (h *DefaultPayloadHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
	payload := processor.PayloadPart{
		Request:         callCtx.GetRequest(),
		OriginalRequest: callCtx.GetOriginalRequest(),
		Result:          callCtx.GetResult(),
		OriginalResult:  callCtx.GetOriginalResult(),
	}
	input.Payload = &payload
}

// Apply modifies payload using result ProcessorContext.
//
// Behavior: both result.Payload.Request.Params and result.Payload.Result
// will be used to modify CallContext accordingly.
func (h *DefaultPayloadHandler) Apply(callCtx CallContext, result *processor.DataContext) error {
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

// DefaultMetaHandler provides default behavior for providing and modifying
// metadata information to/by a processor.
type DefaultMetaHandler struct{}

// AttachPart attaches MCPEvent information from CallContext on input ProcessorContext.
func (h *DefaultMetaHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
	input.Event = callCtx.GetEventInfo()
	// TODO: slight issue here: we are attaching a reference to the callCtx
	// - this can result in processors changing the attached MCPEvent before calling Apply
	// Better: clone MCPEvent
}

// Apply uses provided result ProcessorContext to modify the CallContext.
func (h *DefaultMetaHandler) Apply(callCtx CallContext, result *processor.DataContext) error {
	if result.Event != nil {
		callCtx.SetEventInfo(result.Event)
	}
	return nil
}

// =============================================================================
// RoutingHandler - handles server name and routing decisions
// =============================================================================

// DefaultRoutingHandler handles routing data and modifications on CallContext.
type DefaultRoutingHandler struct{}

// AttachPart attaches routing information from CallContext to input ProcessorContext.
func (h *DefaultRoutingHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
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

// Apply uses provided result ProcessorContext to modify the CallContext.
// Returns an error if no request is attached to CallContext.
func (h *DefaultRoutingHandler) Apply(callCtx CallContext, result *processor.DataContext) error {
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

// DefaultLogHandler provides simple, default functionality for logging CallContext data.
type DefaultLogHandler struct {
	// TODO: enable data redaction in logs -> new logger
	logger *logging.Logger
}

// NewDefaultLogHandler returns a new DefaultLogHandler.
func NewDefaultLogHandler(logger *logging.Logger) *DefaultLogHandler {
	return &DefaultLogHandler{
		logger: logger,
	}
}

// Log uses the attached logger and ToLogEntry to log the provided CallContext data.
func (h *DefaultLogHandler) Log(callCtx CallContext) error {
	return h.logger.LogEntry(h.ToLogEntry(callCtx))
}

// ToLogEntry transforms the provided CallContext into an MCPEvent struct.
func (h *DefaultLogHandler) ToLogEntry(callCtx CallContext) *common.MCPEvent {
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

	// Add arguments for request
	req := callCtx.GetRequest()
	if req != nil && req.Params != nil {
		event.WithToolRequest(callCtx.GetToolName(), callCtx.GetOriginalToolName(), req.Params.Arguments)
	}

	// Add result for response
	if callCtx.HasResult() {
		result := callCtx.GetResult()
		resultJSON, _ := json.Marshal(result)
		event.WithToolResult(resultJSON, result.IsError)
	}
	return event
}

func (h *DefaultLogHandler) getTransport(callCtx CallContext) string {
	if rc := callCtx.GetRoutingContext(); rc != nil {
		return string(rc.Transport)
	}
	return "unknown"
}
