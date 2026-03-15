package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	centauth "github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file wraps the MCP HTTP transport to observe requests and run
// post-handler session-state synchronization.

type observedMCPRequest struct {
	sessionID string
	methods   map[string]struct{}
}

func (o observedMCPRequest) hasMethod(method string) bool {
	_, ok := o.methods[method]
	return ok
}

func (o observedMCPRequest) shouldDelayResponse() bool {
	return o.hasMethod("initialize") || o.hasMethod("notifications/roots/list_changed")
}

type delayedResponseWriter struct {
	headers    http.Header
	statusCode int
	body       bytes.Buffer
}

func newDelayedResponseWriter() *delayedResponseWriter {
	return &delayedResponseWriter{
		headers: make(http.Header),
	}
}

// Header returns the buffered response headers.
func (w *delayedResponseWriter) Header() http.Header {
	return w.headers
}

func (w *delayedResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.body.Write(data)
}

// WriteHeader stores the status code until the buffered response is flushed.
func (w *delayedResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// Flush is a no-op to satisfy http.Flusher when a wrapped handler expects it.
func (w *delayedResponseWriter) Flush() {}

func writeDelayedResponse(target http.ResponseWriter, source *delayedResponseWriter) {
	if target == nil || source == nil {
		return
	}
	for key, values := range source.headers {
		for _, value := range values {
			target.Header().Add(key, value)
		}
	}
	if source.statusCode != 0 {
		target.WriteHeader(source.statusCode)
	}
	if source.body.Len() > 0 {
		_, _ = target.Write(source.body.Bytes())
	}
}

func (p *CentianEndpoint) observeMCPRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observation := inspectMCPRequest(r)
		responseWriter := w
		var delayedWriter *delayedResponseWriter
		if observation.shouldDelayResponse() {
			delayedWriter = newDelayedResponseWriter()
			responseWriter = delayedWriter
		}

		next.ServeHTTP(responseWriter, r)

		sessionID := observation.sessionID
		if sessionID == "" && observation.hasMethod("initialize") {
			if delayedWriter != nil {
				sessionID = delayedWriter.Header().Get("Mcp-Session-Id")
			} else {
				sessionID = w.Header().Get("Mcp-Session-Id")
			}
		}
		if sessionID == "" {
			if delayedWriter != nil {
				writeDelayedResponse(w, delayedWriter)
			}
			return
		}
		if observation.hasMethod("initialize") {
			p.syncUpstreamSessionState(r.Context(), sessionID)
		}
		if observation.hasMethod("notifications/roots/list_changed") {
			p.markUpstreamSessionRootsDirty(sessionID)
			p.syncUpstreamSessionState(r.Context(), sessionID)
		}
		if delayedWriter != nil {
			writeDelayedResponse(w, delayedWriter)
		}
	})
}

func inspectMCPRequest(r *http.Request) observedMCPRequest {
	if r == nil || r.Method != http.MethodPost {
		return observedMCPRequest{}
	}

	body := cloneRequestBody(r)
	if len(body) == 0 {
		return observedMCPRequest{sessionID: r.Header.Get("Mcp-Session-Id")}
	}

	methods := extractMCPMethods(body)
	return observedMCPRequest{
		sessionID: r.Header.Get("Mcp-Session-Id"),
		methods:   methods,
	}
}

func extractMCPMethods(body []byte) map[string]struct{} {
	methods := make(map[string]struct{})

	type rpcEnvelope struct {
		Method string `json:"method"`
	}

	var batch []rpcEnvelope
	if err := json.Unmarshal(body, &batch); err == nil {
		for _, item := range batch {
			if item.Method != "" {
				methods[item.Method] = struct{}{}
			}
		}
		if len(methods) > 0 {
			return methods
		}
	}

	var single rpcEnvelope
	if err := json.Unmarshal(body, &single); err == nil && single.Method != "" {
		methods[single.Method] = struct{}{}
	}
	return methods
}

func cloneRequestBody(r *http.Request) []byte {
	if r == nil || r.Body == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		common.LogWarn("Failed to read request body: %v", err)
		return nil
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	return bodyBytes
}

func apiKeyMiddlewareWithHeader(store *centauth.APIKeyStore, headerName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			next.ServeHTTP(w, r)
			return
		}

		token := extractAuthToken(r.Header.Get(headerName))
		if token == "" {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: missing auth token from %s", r.RemoteAddr)
			return
		}

		entry, ok := store.Lookup(token)
		if !ok {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: invalid auth token from %s", r.RemoteAddr)
			return
		}

		gatewayName := getGatewayFromPath(r.URL.Path)
		if !entry.AllowsGateway(gatewayName) {
			writeUnauthorized(w, headerName)
			common.LogWarn("Unauthorized request: key '%s' not allowed for gateway '%s' from %s", entry.ID, gatewayName, r.RemoteAddr)
			return
		}

		ctx := withRequestIdentity(r.Context(), "auth:"+entry.ID)
		authData := &AuthData{
			AuthHeaderName: headerName,
			Gateway:        gatewayName,
			Headers:        r.Header.Clone(),
			KeyEntry:       entry,
		}
		ctx = withAuthData(ctx, authData)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getGatewayFromPath(requestPath string) string {
	normalized := path.Clean("/" + strings.TrimSpace(requestPath))
	parts := strings.Split(normalized, "/")
	if len(parts) >= 3 && parts[1] == "mcp" {
		return parts[2]
	}
	return ""
}

func getCredentialFingerprint(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func getPrincipalID(keyID, gateway string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(keyID) + ":" + strings.TrimSpace(gateway)))
	return hex.EncodeToString(hash[:])
}

func extractAuthToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return header
}

func writeUnauthorized(w http.ResponseWriter, headerName string) {
	if strings.EqualFold(headerName, "Authorization") {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

func logRequestForDebug(r *http.Request) {
	if !common.DebugLoggingEnabled() || r == nil || r.Body == nil {
		return
	}

	common.LogDebug("Received request: %s - %s - %s", r.Method, r.URL, r.UserAgent())
	common.LogDebug("Received headers: %#v", redactHeaders(r.Header))
	bodyBytes := cloneRequestBody(r)
	if len(bodyBytes) == 0 {
		common.LogDebug("Received request body: <empty>")
		return
	}
	common.LogDebug("Received request body (%d bytes): %s", len(bodyBytes), string(bodyBytes))
}

func authHeadersFingerprint(forwardedHeaders map[string]string) string {
	if len(forwardedHeaders) == 0 {
		return "forwarded-auth:none"
	}

	keys := make([]string, 0, len(forwardedHeaders))
	for key := range forwardedHeaders {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(forwardedHeaders[key])
		builder.WriteString("\n")
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:8])
}

func cloneAuthHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

// RegisterEndpoint registers a ServerProvider with the HTTP mux.
func RegisterEndpoint(proxy *CentianEndpoint, mux *http.ServeMux, options *mcp.StreamableHTTPOptions) {
	if options == nil {
		options = &mcp.StreamableHTTPOptions{
			SessionTimeout: 10 * time.Minute,
			Stateless:      false,
		}
	}

	baseHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			logRequestForDebug(r)
			return proxy.GetOrCreateServerForRequest(r)
		},
		options,
	)

	handler := proxy.observeMCPRequests(baseHandler)
	if proxy.server != nil && proxy.server.APIKeys != nil {
		headerName := proxy.server.AuthHeader
		if headerName == "" {
			headerName = strings.Clone(config.DefaultAuthHeader)
		}
		handler = apiKeyMiddlewareWithHeader(proxy.server.APIKeys, headerName, handler)
	}

	mux.Handle(proxy.endpoint, handler)
	common.LogInfo("Registered handler at %s", proxy.endpoint)
}
