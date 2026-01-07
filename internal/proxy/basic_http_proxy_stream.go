package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/CentianAI/centian-cli/internal/common"
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

const MaxBodySize = 10 * 1024 * 1024 // 10MB
const RequestIDField = "requestID"

type HttpMcpServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func (pc *HttpMcpServerConfig) GetSubstitutedHeaders() map[string]string {
	if pc.Headers == nil {
		return make(map[string]string)
	}
	// TODO: actually perform env var substitution on each header field
	return pc.Headers
}

type GatewayConfig struct {
	AllowDynamicProxy    bool                           `json:"allowDynamic"`
	AllowGatewayEndpoint bool                           `json:"setupGateway"`
	McpServersConfig     map[string]HttpMcpServerConfig `json:"mcpServers"`
	// TODO: could also store processorChain here -> makes more sense
}

type ProxyConfig struct {
	Port    string `json:"port"`    // Port which the proxy server will be hosted on
	Timeout int    `json:"timeout"` // Default timeout used for downstream servers
	// TODO: add more proxy config parameters
}

func NewDefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		Port:    "8080",
		Timeout: 30,
	}
}

func getNewUuidV7() string {
	result := ""
	if id, err := uuid.NewV7(); err == nil {
		result = id.String()
	}
	if result == "" {
		result = fmt.Sprintf("req_%d", time.Now().UnixMicro())
	}
	return result
}

func readBody(body io.ReadCloser) ([]byte, error) {
	// TODO: determine if this is acceptable -
	// alternative is to stream request via
	// TeeReader (log + forward without full buffering)
	limitedReader := io.LimitReader(body, MaxBodySize+1) // +1 to detect overflow
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

type CentianConfig struct {
	Name               string                   `json:"server_name"` // currently only used for logging
	ProxyConfiguration ProxyConfig              `json:"proxy_config"`
	GatewayConfigs     map[string]GatewayConfig `json:"gateways"`
}

type CentianServer struct {
	config     *CentianConfig
	mux        *http.ServeMux
	server     *http.Server
	baseLogger *logging.Logger // Shared base logger (ONE file handle)
	serverID   string          // used to uniquely identify this specific object instance
	gateways   map[string]*CentianProxyGateway
}

type CentianProxyGateway struct {
	config         *GatewayConfig
	name           string
	endpoints      []*CentianProxyEndpoint
	processorChain *processor.Chain
	server         *CentianServer
}

type CentianProxyEndpoint struct {
	endpoint      string
	mcpServerName string
	sessionID     string // TODO: check if we can use MCPs own session IDs
	config        *HttpMcpServerConfig
	gateway       *CentianProxyGateway
	server        *CentianServer
	proxy         *httputil.ReverseProxy

	substitutedHeaders map[string]string
	target             *url.URL
}

func (e *CentianProxyEndpoint) LogProxyStart() {
	e.LogProxyMessage(
		fmt.Sprintf("HTTP endpoint started: %s -> %s", e.endpoint, e.config.URL),
	)
}

func (e *CentianProxyEndpoint) LogProxyStop() {
	e.LogProxyMessage(
		fmt.Sprintf("HTTP endpoint stopped: %s -> %s", e.endpoint, e.config.URL),
	)
}

func (e *CentianProxyEndpoint) LogProxyMessage(message string) {
	requestID := fmt.Sprintf("server_event_%d", time.Now().UnixNano())
	baseEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		SessionID:        e.sessionID, // TODO
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
	httpEvent := &common.HttpEvent{}
	mcpEvent := common.HttpMcpEvent{
		BaseMcpEvent:  baseEvent,
		HttpEvent:     httpEvent,
		Gateway:       e.gateway.name,
		ServerName:    e.mcpServerName,
		Endpoint:      e.endpoint,
		DownstreamURL: e.config.URL,
		ProxyPort:     e.server.config.ProxyConfiguration.Port,
	}
	// TODO: maybe using the event structure for basic logs is not the best way?
	mcpEvent.HttpEvent.Body = []byte(message)
	e.server.baseLogger.LogMcpEvent(mcpEvent)
}

func getServerID(config *CentianConfig) string {
	// TODO: better way of determining server ID
	serverStr := "centian_server"
	if config.Name != "" {
		serverStr = config.Name
	}
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s_%d", serverStr, timestamp)
}

func NewCentianHTTPProxy(config *CentianConfig) (*CentianServer, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         ":" + config.ProxyConfiguration.Port,
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(config.ProxyConfiguration.Timeout),
		WriteTimeout: common.GetSecondsFromInt(config.ProxyConfiguration.Timeout),
	}
	baseLogger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	// TODO: add processor chain -> TODO: check how the config needs to look like!
	return &CentianServer{
		config:     config,
		mux:        mux,
		server:     server,
		baseLogger: baseLogger,
		serverID:   getServerID(config),
		gateways:   make(map[string]*CentianProxyGateway),
	}, nil
}

func (c *CentianServer) StartCentianServer() error {
	config := c.config

	// 1. Iterate through each gateway to create proxy endpoints
	// Note: we are leaving out
	// "Option A - single gateway endpoint with namespacing" for now
	// For this we will likely need to collect all the Tools/Resources
	// from the different servers first and indeed create our own MCP
	// server from them
	// Potentially we could handle this quite differently than originally planned:
	// instead of recreating a new MCP server
	// from all the tools/resources of configured MCP servers
	// we have some logic that understands to which server we should forward a request
	// this is VERY similar to using an endpoint, only that the client
	// has a different way to provide the endpoint,
	// e.g. through the tool name "<servername>__<actual_toolname>"
	for gatewayName, gatewayConfig := range config.GatewayConfigs {
		gateway := CentianProxyGateway{
			config:    &gatewayConfig,
			name:      gatewayName,
			endpoints: make([]*CentianProxyEndpoint, 0),
			server:    c,
		}
		c.gateways[gatewayName] = &gateway

		// 2. Iterate through each mcp server config for this gateway
		for mcpServerName, mcpServerConfig := range gatewayConfig.McpServersConfig {
			// 3. create new endpoint for MCP server to be proxied
			endpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, mcpServerName)
			// TODO: allow custom endpoint patterns
			proxyEndpoint := CreateProxyEndpoint(&mcpServerConfig, &gateway, endpoint, mcpServerName, c)

			// TODO: add verification for both gatewayName and serverName to be used in a URL
			// 4. attach proxy endpoint with logging
			if proxyEndpoint, err := c.RegisterProxy(&proxyEndpoint, &gateway); err != nil {
				log.Printf("Failed to register endpoint %s: %v", endpoint, err)
				// Continue with other servers
			} else {
				gateway.endpoints = append(gateway.endpoints, proxyEndpoint)
			}
		}

		if gatewayConfig.AllowDynamicProxy {
			// TODO: register dynamic endpoint able to dynamically proxy MCP servers
		}

		if gatewayConfig.AllowGatewayEndpoint {
			// TODO: setup endpoint for gateway proxying ALL MCP servers on a single endpoint
			// using namespacing logic on tools and resources

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
			*/

		}
	}
	/*
		Requests:

		- initialize
		- ping
		- tools/list
		- tools/call
		- prompts/list
		- prompts/get
		- resources/templates/list
		- resources/list
		- resources/read
		- resources/subscribe
		- resources/unsubscribe
		- roots/list
		- logging/setLevel
		- completion/complete
		- sampling/createMessage
		- elicitation/create

		Notifications:

		- notifications/initialized
		- notifications/cancelled
		- notifications/message
		- notifications/progress
		- notifications/prompts/list_changed
		- notifications/resources/list_changed
		- notifications/resources/updated
		- notifications/roots/list_changed
		- notifications/tools/list_changed
		- notifications/elicitation/complete
	*/

	// 5. Start the (proxy) Server
	log.Printf("Starting proxy server on :%s", config.ProxyConfiguration.Port)
	return c.server.ListenAndServe()
}

func (c *CentianServer) GetRequestID(r *http.Request) string {
	return getNewUuidV7()
}

func (c *CentianServer) GetResponseID(resp *http.Response) string {
	responseID := ""
	if resp != nil && resp.Request != nil {
		responseID, _ = resp.Request.Context().Value(RequestIDField).(string)
	}
	if responseID == "" {
		responseID = getNewUuidV7()
	}
	return responseID
}

// RegisterProxy creates the actual httputil.ReverseProxy and attaches it
// to both the provided endpoint and the servers mux to start proxying via
// this endpoint
func (c *CentianServer) RegisterProxy(endpoint *CentianProxyEndpoint, gateway *CentianProxyGateway) (*CentianProxyEndpoint, error) {
	// Log endpoint registration
	endpoint.LogProxyStart()

	// Get proxy
	_ = endpoint.createProxy()

	// Attach DirectorHandler to proxy
	endpoint.proxy.Director = endpoint.DirectorHandler

	// Attach ModifyResponseHandler to proxy
	endpoint.proxy.ModifyResponse = endpoint.ModifyResponseHandler

	// 7. Create the handler function
	c.mux.HandleFunc(endpoint.endpoint, func(w http.ResponseWriter, r *http.Request) {
		endpoint.proxy.ServeHTTP(w, r)
	})

	log.Printf("Registered proxy endpoint: %s -> %s", endpoint.endpoint, endpoint.config.URL)
	return endpoint, nil
}

func (c *CentianServer) Shutdown(ctx context.Context) error {
	log.Println("Shutting down proxy server...")

	// Log shutdown for all endpoints
	for _, gateway := range c.gateways {
		for _, endpoint := range gateway.endpoints {
			endpoint.LogProxyStop()
		}
	}

	// Close shared base logger (ONE close for all endpoints)
	if c.baseLogger != nil {
		_ = c.baseLogger.Close()
	}

	return c.server.Shutdown(ctx)
}

func CreateProxyEndpoint(
	config *HttpMcpServerConfig,
	gateway *CentianProxyGateway,
	endpoint, serverName string,
	server *CentianServer,
) CentianProxyEndpoint {
	timestamp := time.Now().UnixNano()
	sessionID := fmt.Sprintf("http_endpoint_%s_%d", endpoint, timestamp)
	proxyEndpoint := CentianProxyEndpoint{
		config:        config,
		gateway:       gateway,
		endpoint:      endpoint,
		mcpServerName: serverName,
		server:        server,
		proxy:         nil, // nil indicates that the proxy was not yet created
		sessionID:     sessionID,
	}

	proxyEndpoint.substitutedHeaders = config.GetSubstitutedHeaders()
	if target, err := url.Parse(config.URL); err != nil {
		common.LogError(err.Error())
	} else {
		proxyEndpoint.target = target
	}
	return proxyEndpoint
}

func (e *CentianProxyEndpoint) createProxy() *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(e.target)

	// Configure the proxy for streaming (SSE support)
	// FlushInterval: -1 ensures that the data is sent to the client immediately
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1
	e.proxy = proxy
	return proxy
}

func (e *CentianProxyEndpoint) mcpEventFromRequest(r *http.Request) *common.HttpMcpEvent {
	reqID := e.server.GetRequestID(r)
	mcpEvent := e.getHttpMcpEvent(
		reqID,
		true,
		common.DirectionClientToServer,
		common.MessageTypeRequest,
		common.NewHttpEventFromRequest(r, reqID),
	)
	var err error
	if r.Body != nil {
		if mcpEvent.HttpEvent.Body, err = readBody(r.Body); err != nil {
			common.LogError(err.Error())
			mcpEvent.ProcessingErrors["read_body"] = err
		}
	}
	return mcpEvent
}

func (e *CentianProxyEndpoint) getHttpMcpEvent(
	RequestID string,
	success bool,
	direction common.McpEventDirection,
	messageType common.McpMessageType,
	httpEvent *common.HttpEvent,
) *common.HttpMcpEvent {
	baseMcpEvent := common.BaseMcpEvent{
		Timestamp:        time.Now(),
		Transport:        "http",
		RequestID:        RequestID,
		SessionID:        e.sessionID,
		ServerID:         e.server.serverID,
		Direction:        direction,
		MessageType:      messageType,
		Success:          success,
		Error:            "",
		ProcessingErrors: make(map[string]error),
		Metadata:         make(map[string]string),
	}
	return &common.HttpMcpEvent{
		BaseMcpEvent:  baseMcpEvent,
		HttpEvent:     httpEvent,
		Gateway:       e.gateway.name,
		ServerName:    e.mcpServerName,
		Endpoint:      e.endpoint,
		DownstreamURL: e.config.URL,
		ProxyPort:     e.server.config.ProxyConfiguration.Port,
	}
}

func (e *CentianProxyEndpoint) mcpEventFromResponse(r *http.Response) *common.HttpMcpEvent {
	reqID := e.server.GetResponseID(r)
	mcpEvent := e.getHttpMcpEvent(
		reqID,
		r.StatusCode < 400, // success
		common.DirectionServerToClient,
		common.MessageTypeResponse,
		common.NewHttpEventFromResponse(r, reqID),
	)
	return mcpEvent
}

func (e *CentianProxyEndpoint) GetTarget() (*url.URL, error) {
	target, err := url.Parse(e.config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL for endpoint %s: %w", e.endpoint, err)
	}
	return target, nil
}

func (e *CentianProxyEndpoint) DirectorHandler(r *http.Request) {
	// Read and log request
	mcpRequest := e.mcpEventFromRequest(r)
	// mcpRequest has r.Body set if it is non-Nil, as mcpRequest.HttpEvent.Body

	// Log to file
	if err := e.server.baseLogger.LogMcpEvent(mcpRequest); err != nil {
		common.LogError(err.Error())
		mcpRequest.ProcessingErrors["log_request_init"] = err
	}

	// Debug output to stderr
	fmt.Fprintf(os.Stderr, "[CLIENT->SERVER] [%s] %s\n", e.endpoint, mcpRequest.RawMessage())

	// TODO: Execute processor chain on request (if configured)
	// requestBody needs to be modified here!
	processedMessage := mcpRequest.RawMessage()

	if processedMessage != mcpRequest.RawMessage() || len(mcpRequest.ProcessingErrors) > 0 {
		processedEvent := mcpRequest.DeepClone()
		processedEvent.RequestID = processedEvent.RequestID + "_processed"
		processedEvent.HttpEvent.Body = []byte(processedMessage)
		processedEvent.Metadata["processing_stage"] = "post_processor"
		processedEvent.Metadata["processed_at"] = time.Now().Format(time.RFC3339Nano)
		// TODO: double check if we need to change more fields
		if err := e.server.baseLogger.LogMcpEvent(processedEvent); err != nil {
			common.LogError(err.Error())
			processedEvent.ProcessingErrors["log_request_post_processors"] = err
		}
	}

	// Restore body for forwarding (important!)
	r.Body = io.NopCloser(bytes.NewBuffer([]byte(processedMessage)))

	// Store requestID in context for response logging
	ctx := context.WithValue(r.Context(), RequestIDField, mcpRequest.RequestID)
	*r = *r.WithContext(ctx)

	// Set target URL and headers
	r.URL.Scheme = e.target.Scheme
	r.URL.Host = e.target.Host
	r.Host = e.target.Host
	for k, v := range e.substitutedHeaders {
		r.Header.Set(k, v)
	}
}

func (e *CentianProxyEndpoint) ModifyResponseHandler(resp *http.Response) error {
	mcpResponse := e.mcpEventFromResponse(resp)
	// Read and log response
	var err error
	if resp.Body != nil {
		if mcpResponse.HttpEvent.Body, err = readBody(resp.Body); err != nil {
			common.LogError(err.Error())
		}
	}
	// Log to file
	if err := e.server.baseLogger.LogMcpEvent(mcpResponse); err != nil {
		common.LogError(err.Error())
		mcpResponse.ProcessingErrors["log_response_init"] = err
	}

	// Debug output to stderr
	fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] [%s] %s\n", e.endpoint, mcpResponse.RawMessage())

	// TODO: Execute processor chain on response (if configured)
	// responseBody needs to be modified here!
	processedMessage := mcpResponse.RawMessage()

	// TODO: better would be a flag if the McpEvent was modified
	if processedMessage != mcpResponse.RawMessage() || len(mcpResponse.ProcessingErrors) > 0 {
		processedEvent := mcpResponse.DeepClone()
		processedEvent.RequestID = processedEvent.RequestID + "_processed"
		processedEvent.HttpEvent.Body = []byte(processedMessage)
		processedEvent.Metadata["processing_stage"] = "post_processor"
		processedEvent.Metadata["processed_at"] = time.Now().Format(time.RFC3339Nano)
		// TODO: double check if we need to change more fields
		if err := e.server.baseLogger.LogMcpEvent(processedEvent); err != nil {
			common.LogError(err.Error())
			mcpResponse.ProcessingErrors["log_response_post_processors"] = err
		}

	}

	// Restore body for client (important!)
	if processedMessage != "" {
		resp.Body = io.NopCloser(bytes.NewBuffer([]byte(processedMessage)))
	}
	return nil
}
