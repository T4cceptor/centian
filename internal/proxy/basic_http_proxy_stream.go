// Package proxy provides functionality to proxy MCP communication between Client and Server.
// The main functionality splits between stdio and http proxies, and includes a central
// processing loop which logs and calls any configured processors for all MCP requests/responses
// which are proxied.
package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/config"
	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/CentianAI/centian-cli/internal/processor"
	"github.com/google/uuid"
)

/*
Full config example:

	{
		"gateways": {
			"my-gateway": {
				"mcpServers": {
					"my-mcp-proxy-to-github": {
						"url": "https://api.githubcopilot.com/mcp/",
						"headers": {
							"Authorization": "Bearer ${GITHUB_PAT}"
						}
					}
				}
			}
		}
	}
*/

// MaxBodySize represents the maximal allowed size of a request/response body.
const MaxBodySize = 10 * 1024 * 1024 // 10MB

type requestIDField string

const reqIDField requestIDField = "requestID"

func getNewUUIDV7() string {
	result := ""
	if id, err := uuid.NewV7(); err == nil {
		result = id.String()
	}
	if result == "" {
		result = fmt.Sprintf("req_%d", time.Now().UnixMicro())
	}
	return result
}

func readBody(body io.ReadCloser, headers http.Header) ([]byte, error) {
	var limitedReader io.Reader
	// Check if response is gzip-compressed.
	if strings.EqualFold(headers.Get("Content-Encoding"), "gzip") {
		// Decompress gzip response.
		var gzipErr error
		limitedReader, gzipErr = gzip.NewReader(body)
		if gzipErr != nil {
			return nil, gzipErr
		}
	} else {
		// TODO: determine if this is acceptable -.
		// alternative is to stream request via.
		// TeeReader (log + forward without full buffering).
		limitedReader = io.LimitReader(body, MaxBodySize+1) // +1 to detect overflow
	}
	bodyBytes, err := io.ReadAll(limitedReader)
	if len(bodyBytes) > MaxBodySize {
		log.Print("request body exceeds maximum size")
		return nil, fmt.Errorf("request body exceeds maximum size")
	}
	if err != nil {
		log.Printf("Error reading request body: %s", err)
		return nil, err
	}
	return bodyBytes, nil
}

/*
CentianServer is the main server struct.

It holds 4 critical components:
- mux - used to register URL paths
- server - used to serve the mux
- logger - main logger for all events in the proxied endpoints
- gateways - holds all gateways and proxy endpoints for easy access

Additionally it has a reference to the global config which was loaded to
initialize this server.
*/
type CentianServer struct {
	config   *config.GlobalConfig
	mux      *http.ServeMux
	server   *http.Server
	logger   *logging.Logger // Shared base logger (ONE file handle)
	serverID string          // used to uniquely identify this specific object instance
	gateways map[string]*CentianProxyGateway
}

// CentianProxyGateway represents a gateway holding one or multiple CentianProxyEndpoint.
//
// The gateway provides a way to group MCP servers together that should all apply
// the same processing configuration.
type CentianProxyGateway struct {
	config    *config.GatewayConfig
	name      string
	endpoints []*CentianProxyEndpoint
	server    *CentianServer
}

// CentianProxyEndpoint is the main struct representing a proxy, forwarding requrests on.
// "endpoint" to the configured downstream MCP server (see downstreamURL).
type CentianProxyEndpoint struct {
	endpoint           string
	mcpServerName      string
	sessionID          string // TODO: check if we can use MCPs own session IDs
	config             *config.MCPServerConfig
	gateway            *CentianProxyGateway
	server             *CentianServer
	proxy              *httputil.ReverseProxy
	processor          *EventProcessor
	substitutedHeaders map[string]string
	target             *url.URL
}

// LogProxyStart logs a "proxy started" message.
func (e *CentianProxyEndpoint) LogProxyStart() {
	e.logSystemMessage(fmt.Sprintf("HTTP endpoint started: %s -> %s", e.endpoint, e.config.URL))
}

// LogProxyStop logs a "proxy stopped" message.
func (e *CentianProxyEndpoint) LogProxyStop() {
	e.logSystemMessage(fmt.Sprintf("HTTP endpoint stopped: %s -> %s", e.endpoint, e.config.URL))
}

func (e *CentianProxyEndpoint) logSystemMessage(message string) {
	requestID := fmt.Sprintf("system_event_%d", time.Now().UnixNano())
	baseEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		SessionID:        e.sessionID,
		ServerID:         e.server.serverID,
		Transport:        "http",
		RequestID:        requestID,
		Direction:        common.DirectionSystem,
		MessageType:      common.MessageTypeSystem,
		Error:            "",
		Success:          true,
		Metadata:         nil,
		ProcessingErrors: make(map[string]error),
	}
	httpEvent := &common.HTTPEvent{
		Body: []byte(message),
	}
	mcpEvent := &common.HTTPMcpEvent{
		BaseMcpEvent:  baseEvent,
		HTTPEvent:     httpEvent,
		Gateway:       e.gateway.name,
		ServerName:    e.mcpServerName,
		Endpoint:      e.endpoint,
		DownstreamURL: e.config.URL,
		ProxyPort:     e.server.config.Proxy.Port,
	}
	if err := e.server.logger.LogMcpEvent(mcpEvent); err != nil {
		common.LogError(err.Error())
	}
}

// getServerID returns a new serverID using the server name.
func getServerID(globalConfig *config.GlobalConfig) string {
	// TODO: better way of determining server ID.
	serverStr := "centian_server"
	if globalConfig.Name != "" {
		serverStr = globalConfig.Name
	}
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s_%d", serverStr, timestamp)
}

// NewCentianHTTPProxy takes a GlobalConfig struct and returns a new CentianServer.
//
// Note: the server does not have gateways and endpoints attached until StartCentianServer is called.
func NewCentianHTTPProxy(globalConfig *config.GlobalConfig) (*CentianServer, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         ":" + globalConfig.Proxy.Port,
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
		WriteTimeout: common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
	}
	logger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	return &CentianServer{
		config:   globalConfig,
		mux:      mux,
		server:   server,
		logger:   logger,
		serverID: getServerID(globalConfig),
		gateways: make(map[string]*CentianProxyGateway),
	}, nil
}

// GetNewGateway returns a new CentianProxyGateway for the given parameters.
func (c *CentianServer) GetNewGateway(
	gatewayName string,
	gatewayConfig *config.GatewayConfig,
) CentianProxyGateway {
	return CentianProxyGateway{
		config:    gatewayConfig,
		name:      gatewayName,
		endpoints: make([]*CentianProxyEndpoint, 0),
		server:    c,
	}
}

// getEndpoint returns a new endpoint path for the given gatewayName and mcpServerName.
func getEndpoint(gatewayName, mcpServerName string) (string, error) {
	result := fmt.Sprintf("/mcp/%s/%s", gatewayName, mcpServerName)
	if !common.IsURLCompliant(result) {
		return "", fmt.Errorf("endpoint '%s' is not a compliant URL", result)
	}
	return result, nil
}

// StartCentianServer uses CentianServer.config to create all gateways and endpoints,
// and start listening on the configured endpoints.
func (c *CentianServer) StartCentianServer() error {
	serverConfig := c.config

	// 1. Iterate through each gateway to create proxy endpoints.
	// Note: we are leaving out.
	// "Option A - single gateway endpoint with namespacing" for now.
	// For this we will likely need to collect all the Tools/Resources.
	// from the different servers first and indeed create our own MCP.
	// server from them.
	// Potentially we could handle this quite differently than originally planned:.
	// instead of recreating a new MCP server.
	// from all the tools/resources of configured MCP servers.
	// we have some logic that understands to which server we should forward a request.
	// this is VERY similar to using an endpoint, only that the client.
	// has a different way to provide the endpoint,
	// e.g. through the tool name "<servername>__<actual_toolname>".
	for gatewayName, gatewayConfig := range serverConfig.Gateways {
		gateway := c.GetNewGateway(gatewayName, gatewayConfig)

		c.gateways[gatewayName] = &gateway

		// 2. Iterate through each mcp server config for this gateway.
		for mcpServerName, mcpServerConfig := range gatewayConfig.MCPServers {
			// 3. create new endpoint for MCP server to be proxied.
			endpoint, err := getEndpoint(gatewayName, mcpServerName)
			if err != nil {
				common.LogError(err.Error())
				continue // we do not proceed if we receive an error during endpoint creation
			}
			proxyEndpoint := CreateProxyEndpoint(
				mcpServerConfig,
				&gateway,
				endpoint,
				mcpServerName,
				c, // sever
			)

			// 4. attach proxy endpoint with logging.
			if err := c.RegisterProxy(&proxyEndpoint); err != nil {
				log.Printf("Failed to register endpoint %s: %v", endpoint, err)
				// Continue with other servers.
			} else {
				gateway.endpoints = append(gateway.endpoints, &proxyEndpoint)
			}
		}

		// TODO: register dynamic endpoint able to dynamically proxy MCP servers.
		// if gatewayConfig.AllowDynamic {}.

		// TODO: setup endpoint for gateway proxying ALL MCP servers on a single endpoint.
		// using namespacing logic on tools and resources.

		/*
			TODO: decide on option how to do this

			Option A:
			- use SDK to query all downstream MCP servers to get all Tools and Resources
			- rename tools and resources based on namespace schema
			- setup our own MCP server with the new renamed tools and resources
			Pros: straight forward on high-level
			Cons: can get very complicated fast in the details

			Option B:
			- proxy specific requests/actions, like "initialize" or "ping", "tools/list",
			then have a logic that routes the request based on the action:
				- tools/list -> triggers tools/list for ALL servers
					- challenge here might be session management

			if gatewayConfig.AllowGatewayEndpoint {}
		*/
	}

	// 5. Start the (proxy) Server.
	log.Printf("Starting proxy server on :%s", serverConfig.Proxy.Port)
	return c.server.ListenAndServe()
}

// GetRequestID returns a new requestID using UUIDv7.
func (c *CentianServer) GetRequestID(_ *http.Request) string {
	return getNewUUIDV7()
}

// GetResponseID returns a responseID for the given http.Response.
//
// Note: it will create a new responseID using UUIDv7 if it cannot
// extract the response ID from the given http.Response.
func (c *CentianServer) GetResponseID(resp *http.Response) string {
	responseID := ""
	if resp != nil && resp.Request != nil {
		responseID, _ = resp.Request.Context().Value(reqIDField).(string)
	}
	if responseID == "" {
		responseID = getNewUUIDV7()
	}
	return responseID
}

// RegisterProxy creates the actual httputil.ReverseProxy and attaches it
// to both the provided endpoint and the servers mux to start proxying via
// this endpoint.
func (c *CentianServer) RegisterProxy(endpoint *CentianProxyEndpoint) error {
	// Log endpoint registration.
	endpoint.LogProxyStart()

	// Get proxy.
	_ = endpoint.createProxy()

	// Attach Director to proxy - this handles the target URL and headers settings.
	endpoint.proxy.Director = func(r *http.Request) {
		r.URL.Scheme = endpoint.target.Scheme
		r.URL.Host = endpoint.target.Host
		r.URL.Path = endpoint.target.Path // Use the target's path, not the proxy endpoint path
		r.Host = endpoint.target.Host
		for k, v := range endpoint.substitutedHeaders {
			r.Header.Set(k, v)
		}
	}

	// Attach ModifyResponseHandler to proxy.
	endpoint.proxy.ModifyResponse = endpoint.ModifyResponseHandler

	// 7. Create the handler function.
	c.mux.HandleFunc(endpoint.endpoint, func(w http.ResponseWriter, r *http.Request) {
		mcpEvent := endpoint.RequestHandler(r)
		if mcpEvent.Status > 299 {
			// If status indicates an error we return to the client immediately.
			// TODO: log this?
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(mcpEvent.GetBaseEvent().Status)
			if _, err := w.Write([]byte(mcpEvent.RawMessage())); err != nil {
				common.LogError(err.Error())
			}
			return // we return WITHOUT forwarding the message to the server
		}
		endpoint.proxy.ServeHTTP(w, r)
	})

	log.Printf("Registered proxy endpoint: %s -> %s", endpoint.endpoint, endpoint.config.URL)
	return nil
}

// Shutdown stops the whole server, including all endpoints and gateways.
func (c *CentianServer) Shutdown(ctx context.Context) error {
	log.Println("Shutting down proxy server...")

	// Log shutdown for all endpoints.
	for _, gateway := range c.gateways {
		for _, endpoint := range gateway.endpoints {
			endpoint.LogProxyStop()
		}
	}

	// Close shared base logger (ONE close for all endpoints).
	if c.logger != nil {
		_ = c.logger.Close()
	}

	return c.server.Shutdown(ctx)
}

// CreateProxyEndpoint takes both MCPServerConfig and a CentianProxyGateway to create a new CentianProxyEndpoint for the provided parameters.
//
// This creates the main proxy between Client and downstream URl, at 'endpoint'.
func CreateProxyEndpoint(
	mcpServerConfig *config.MCPServerConfig,
	gateway *CentianProxyGateway,
	endpoint, serverName string,
	server *CentianServer,
) CentianProxyEndpoint {
	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("http_endpoint_%s_%d", endpoint, timestamp)
	proxyEndpoint := CentianProxyEndpoint{
		config:        mcpServerConfig,
		gateway:       gateway,
		endpoint:      endpoint,
		mcpServerName: serverName,
		server:        server,
		proxy:         nil, // nil indicates that the proxy was not yet created
		sessionID:     sessionID,
	}

	proxyEndpoint.substitutedHeaders = mcpServerConfig.GetSubstitutedHeaders()
	if target, err := url.Parse(mcpServerConfig.URL); err != nil {
		common.LogError(err.Error())
	} else {
		proxyEndpoint.target = target
	}

	// Set processorChain.
	// Note: we combine both global (server) and local (gateway) processors.
	// TODO: later we can handle this more granulary, but works for now.
	allProcessors := append(
		append(
			[]*config.ProcessorConfig{},
			server.config.Processors...,
		),
		gateway.config.Processors...,
	)
	processorChain, err := processor.NewChain(allProcessors, server.config.Name, sessionID)
	if err != nil {
		common.LogError(err.Error())
	}
	proxyEndpoint.processor = NewEventProcessor(server.logger, processorChain)
	return proxyEndpoint
}

// createProxy creates the httputil.ReverseProxy used for proxying between client and server.
func (e *CentianProxyEndpoint) createProxy() *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(e.target)

	// Configure the proxy for streaming (SSE support).
	// FlushInterval: -1 ensures that the data is sent to the client immediately.
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1
	e.proxy = proxy
	return proxy
}

// mcpEventFromRequest uses http.Request to create a HTTPMcpEvent.
func (e *CentianProxyEndpoint) mcpEventFromRequest(r *http.Request) *common.HTTPMcpEvent {
	reqID := e.server.GetRequestID(r)
	mcpEvent := e.getHTTPMcpEvent(
		reqID,
		true,
		common.DirectionClientToServer,
		common.MessageTypeRequest,
		common.NewHTTPEventFromRequest(r, reqID),
	)
	var err error
	if r.Body != nil {
		if mcpEvent.HTTPEvent.Body, err = readBody(r.Body, r.Header); err != nil {
			common.LogError(err.Error())
			mcpEvent.ProcessingErrors["read_body"] = err
		}
	}
	return mcpEvent
}

// getHTTPMcpEvent creates a new common.HTTPMcpEvent from the given parameters.
func (e *CentianProxyEndpoint) getHTTPMcpEvent(
	requestID string,
	success bool,
	direction common.McpEventDirection,
	messageType common.McpMessageType,
	httpEvent *common.HTTPEvent,
) *common.HTTPMcpEvent {
	baseMcpEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		Transport:        "http",
		RequestID:        requestID,
		SessionID:        e.sessionID,
		ServerID:         e.server.serverID,
		Direction:        direction,
		MessageType:      messageType,
		Success:          success,
		Error:            "",
		ProcessingErrors: make(map[string]error),
		Metadata:         make(map[string]string),
	}
	return &common.HTTPMcpEvent{
		BaseMcpEvent:  baseMcpEvent,
		HTTPEvent:     httpEvent,
		Gateway:       e.gateway.name,
		ServerName:    e.mcpServerName,
		Endpoint:      e.endpoint,
		DownstreamURL: e.config.URL,
		ProxyPort:     e.server.config.Proxy.Port,
	}
}

func (e *CentianProxyEndpoint) mcpEventFromResponse(r *http.Response) *common.HTTPMcpEvent {
	reqID := e.server.GetResponseID(r)
	mcpEvent := e.getHTTPMcpEvent(
		reqID,
		r.StatusCode < 400, // success
		common.DirectionServerToClient,
		common.MessageTypeResponse,
		common.NewHTTPEventFromResponse(r, reqID),
	)
	return mcpEvent
}

// GetTarget returns the target URL for the given proxy endpoint
// as url.URL, or an error if the URL cannot be parsed.
func (e *CentianProxyEndpoint) GetTarget() (*url.URL, error) {
	target, err := url.Parse(e.config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL for endpoint %s: %w", e.endpoint, err)
	}
	return target, nil
}

// RequestHandler is called when the MCP client sends an event, before it is forwarded to the MCP server.
func (e *CentianProxyEndpoint) RequestHandler(r *http.Request) *common.HTTPMcpEvent {
	// Read and log request.
	mcpRequest := e.mcpEventFromRequest(r)
	// mcpRequest has r.Body set if it is non-Nil, as mcpRequest.HTTPEvent.Body.

	// Debug output to stderr.
	fmt.Fprintf(
		os.Stderr,
		"[CLIENT->SERVER] - %s - [%s] %s\n",
		mcpRequest.HTTPEvent.Method,
		e.endpoint,
		mcpRequest.RawMessage(),
	)

	if err := e.processor.Process(mcpRequest); err != nil {
		common.LogError(err.Error())
		mcpRequest.ProcessingErrors["processing_error"] = err
	}

	// Restore body for forwarding (important!).
	// Only set body if there's content - empty bodies should remain nil/empty.
	if mcpRequest.HasContent() {
		r.Body = io.NopCloser(bytes.NewBuffer([]byte(mcpRequest.RawMessage())))
	} else {
		r.Body = http.NoBody // Use http.NoBody for empty requests
	}

	// Store requestID in context for response logging.
	ctx := context.WithValue(r.Context(), reqIDField, mcpRequest.RequestID)
	*r = *r.WithContext(ctx)
	return mcpRequest
}

// ModifyResponseHandler is called when the proxied MCP server returns an answer or sends an event, before it is forwarded to the MCP client.
func (e *CentianProxyEndpoint) ModifyResponseHandler(resp *http.Response) error {
	mcpResponse := e.mcpEventFromResponse(resp)
	// Read and log response.
	var err error
	if resp.Body != nil {
		// Read uncompressed body normally.
		if mcpResponse.HTTPEvent.Body, err = readBody(resp.Body, resp.Header); err != nil {
			common.LogError(err.Error())
			mcpResponse.ProcessingErrors["read_body"] = err
		}
	}
	// Debug output to stderr.
	fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] [%s] %s\n", e.endpoint, mcpResponse.RawMessage())

	if err := e.processor.Process(mcpResponse); err != nil {
		common.LogError(err.Error())
		mcpResponse.ProcessingErrors["processing_error"] = err
	}

	// Restore body for client (important!).
	// Only set body if there's content - empty bodies should remain nil/empty.
	if mcpResponse.HasContent() {
		resp.Body = io.NopCloser(bytes.NewBuffer([]byte(mcpResponse.RawMessage())))
	} else {
		resp.Body = http.NoBody // Use http.NoBody for empty responses
	}
	return nil
}
