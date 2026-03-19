package proxy

import (
	"fmt"

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

// AttachPart attaches MetaContext information from CallContext on input ProcessorContext.
func (h *DefaultMetaHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
	input.Event = callCtx.GetMetaContext()
	// TODO: slight issue here: we are attaching a reference to the callCtx
	// - this can result in processors changing the attached MetaContext before calling Apply
	// Better: clone MetaContext
}

// Apply uses provided result ProcessorContext to modify the CallContext.
func (h *DefaultMetaHandler) Apply(callCtx CallContext, result *processor.DataContext) error {
	if result.Event != nil {
		callCtx.SetMetaContext(result.Event)
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
// AuthHandler - provides read-only auth context to processors
// =============================================================================

// DefaultAuthHandler attaches auth context and treats it as immutable.
type DefaultAuthHandler struct{}

// AttachPart attaches auth information from CallContext to input ProcessorContext.
func (h *DefaultAuthHandler) AttachPart(callCtx CallContext, input *processor.DataContext) {
	if input == nil || callCtx == nil {
		return
	}
	input.Auth = h.buildAuthContext(callCtx)
}

func (h *DefaultAuthHandler) buildAuthContext(callCtx CallContext) *common.AuthContext {
	if callCtx == nil {
		return nil
	}
	authData := callCtx.GetAuthData()
	if authData == nil {
		return nil
	}

	authCtx := &common.AuthContext{
		Authenticated: true,
		PrincipalType: "api_key",
		Gateway:       authData.Gateway,
		AuthHeader:    authData.AuthHeaderName,
		// Internal session ID comes from proxy session mapping.
		InternalSessionID: callCtx.GetSessionID(),
	}

	if authData.KeyEntry != nil {
		authCtx.KeyID = authData.KeyEntry.ID
		authCtx.PrincipalID = getPrincipalID(authData.KeyEntry.ID, authData.Gateway)
	}

	token := ""
	if authData.Headers != nil {
		token = extractAuthToken(authData.Headers.Get(authData.AuthHeaderName))
		if token == "" {
			token = extractAuthToken(authData.Headers.Get("Authorization"))
		}
		authCtx.TransportSessionID = authData.Headers.Get("Mcp-Session-Id")
	}
	if token != "" {
		authCtx.CredentialFingerprint = getCredentialFingerprint(token)
	}

	// Prefer request-bound MCP session ID if available.
	if req := callCtx.GetRequest(); req != nil && req.Session != nil {
		authCtx.TransportSessionID = req.Session.ID()
	}
	return authCtx
}

// Apply ignores auth modifications from processors to keep auth context read-only.
func (h *DefaultAuthHandler) Apply(_ CallContext, _ *processor.DataContext) error {
	return nil
}

// =============================================================================
// DefaultLogHandler writes the current call state to the request log.
// =============================================================================

// DefaultLogHandler provides simple, default functionality for logging CallContext data.
type DefaultLogHandler struct {
	logger *logging.Logger
}

// NewDefaultLogHandler returns a new DefaultLogHandler.
func NewDefaultLogHandler(logger *logging.Logger) *DefaultLogHandler {
	return &DefaultLogHandler{
		logger: logger,
	}
}

// Log uses the attached logger to log the provided CallContext data.
func (h *DefaultLogHandler) Log(callCtx CallContext) error {
	return h.logger.LogEntry(callCtx.ToLogEntry())
}
