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

type CentianConfig struct {
	Name               string                   `json:"server_name"` // currently only used for logging
	ProxyConfiguration ProxyConfig              `json:"proxy_config"`
	GatewayConfigs     map[string]GatewayConfig `json:"gateways"`
}

type CentianServer struct {
	config         *CentianConfig
	mux            *http.ServeMux
	server         *http.Server
	baseLogger     *logging.Logger                       // Shared base logger (ONE file handle)
	loggers        map[string]logging.McpLoggerInterface // One logger per endpoint
	processorChain *processor.Chain                      // TODO: processorChain could also be on a specific Gateway
	serverID       string                                // used to uniquely identify this specific object instance
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

	// Create ONE base logger (ONE file handle) shared by all endpoint loggers
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
		loggers:    make(map[string]logging.McpLoggerInterface),
		serverID:   getServerID(config),
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
		// 2. Iterate through each mcp server config for this gateway
		for serverName, serverConfig := range gatewayConfig.McpServersConfig {
			// 3. create new endpoint for MCP server to be proxied
			endpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			// TODO: allow custom endpoint patterns

			// TODO: add verification for both gatewayName and serverName to be used in a URL
			// 4. attach proxy endpoint with logging
			if err := c.RegisterProxy(endpoint, &serverConfig, gatewayName, serverName); err != nil {
				log.Printf("Failed to register endpoint %s: %v", endpoint, err)
				// Continue with other servers
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
	log.Printf("Starting proxy server on %s", config.ProxyConfiguration.Port)
	return c.server.ListenAndServe()
}

func getNewId() string {
	result := ""
	if id, err := uuid.NewV7(); err == nil {
		result = id.String()
	}
	if result == "" {
		result = fmt.Sprintf("req_%d", time.Now().UnixMicro())
	}
	return result
}

func (c *CentianServer) GetRequestID(r *http.Request) string {
	return getNewId()
}

func (c *CentianServer) GetResponseID(resp *http.Response) string {
	responseID := ""
	if resp != nil && resp.Request != nil {
		responseID, _ = resp.Request.Context().Value("requestID").(string)
	}
	if responseID == "" {
		responseID = getNewId()
	}
	return responseID
}

func (c *CentianServer) readBody(reader io.Reader) ([]byte, error) {
	bodyBytes, err := io.ReadAll(reader)
	if len(bodyBytes) >= MaxBodySize {
		log.Print("request body exceeds maximum size")
		return nil, fmt.Errorf("request body exceeds maximum size")
	}
	if err != nil {
		log.Printf("Error reading request body: %s", err)
		return nil, err
	}
	return bodyBytes, nil
}

func (c *CentianServer) getProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Configure the proxy for streaming (SSE support)
	// FlushInterval: -1 ensures that the data is sent to the client immediately
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1
	return proxy
}

func (c *CentianServer) RegisterProxy(endpoint string, config *HttpMcpServerConfig, gatewayName, serverName string) error {
	// 1. Create endpoint-specific logger (shares baseLogger file handle)
	endpointLogger := logging.NewHttpLogger(
		c.baseLogger,
		c.config.ProxyConfiguration.Port,
		gatewayName,
		serverName,
		endpoint,
		config.URL,
	)

	// Store logger for this endpoint
	c.loggers[endpoint] = endpointLogger
	// TODO: should we extend this into a bigger state for the endpoint?
	// the endpoint could have its own struct with logger, and other relevant information on it

	// Log endpoint registration
	_ = endpointLogger.LogProxyStart(nil)

	// 2. Get target URL from config
	target, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid URL for endpoint %s: %w", endpoint, err)
	}

	// 3. Get proxy
	proxy := c.getProxy(target)

	// 5. Deal with headers and request logging via Director
	headers := config.GetSubstitutedHeaders()
	proxy.Director = func(r *http.Request) {
		// Read and log request
		mcpRequest := McpEvent{
			EventType:  McpRequestEvent,
			RequestID:  c.GetRequestID(r),
			SessionID:  endpointLogger.SessionID(), // TODO ?
			Endpoint:   endpoint,
			ReceivedAt: time.Now(),
			Request:    r,
			Response:   nil,
			ReqBody:    nil, // currently empty - needs to be filled
			RespBody:   nil,
			JSONRPC:    nil, // nil indicates NO body - this does NOT represent an EMPTY body
			metadata: map[string]string{
				"test": "test",
			},
		}

		if r.Body != nil {
			// TODO: determine if this is acceptable -
			// alternative is to stream request via
			// TeeReader (log + forward without full buffering)
			// TODO: ContentLength check
			limitedBody := io.LimitReader(r.Body, MaxBodySize)
			if mcpRequest.ReqBody, err = c.readBody(limitedBody); err != nil {
				common.LogError(err.Error())
				mcpRequest.processingErrors["read_body"] = err
			}
		}

		// Log to file
		// TODO: refactor logging -> we want to know about requests without a body too!!
		if err := endpointLogger.LogRequest(mcpRequest); err != nil {
			common.LogError(err.Error())
			mcpRequest.processingErrors["log_request"] = err
		}

		// Debug output to stderr
		fmt.Fprintf(os.Stderr, "[CLIENT->SERVER] [%s] %s\n", endpoint, requestBody)

		// TODO: Execute processor chain on request (if configured)
		// requestBody needs to be modified here!

		// Restore body for forwarding (important!)
		r.Body = io.NopCloser(bytes.NewBuffer([]byte(requestBody)))

		// Store requestID in context for response logging
		ctx := context.WithValue(r.Context(), "requestID", requestID)
		*r = *r.WithContext(ctx)

		// Set target URL and headers
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.Host = target.Host
		for k, v := range headers {
			r.Header.Set(k, v)
		}
	}

	// 6. Add response logging via ModifyResponse hook
	proxy.ModifyResponse = func(resp *http.Response) error {
		requestID := c.GetResponseID(resp)
		responseBody := ""
		// TODO: this should actually be a struct and contain
		// more information about the request/response

		// Read and log response
		if resp.Body != nil {
			// TODO: ContentLength check
			if bodyBytes, err := io.ReadAll(resp.Body); err != nil {
				common.LogError(err.Error())
			} else {
				responseBody = string(bodyBytes)
			}
		}
		// Log to file
		if err := endpointLogger.LogResponse(requestID, responseBody, true, "", nil); err != nil {
			common.LogError(err.Error())
		}

		// Debug output to stderr
		fmt.Fprintf(os.Stderr, "[SERVER->CLIENT] [%s] %s\n", endpoint, responseBody)

		// TODO: Execute processor chain on response (if configured)
		// responseBody needs to be modified here!

		// Restore body for client (important!)
		if responseBody != "" {
			resp.Body = io.NopCloser(bytes.NewBuffer([]byte(responseBody)))
		}
		return nil
	}

	// 7. Create the handler function
	c.mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	log.Printf("Registered proxy endpoint: %s -> %s", endpoint, config.URL)
	return nil
}

func (c *CentianServer) Shutdown(ctx context.Context) error {
	log.Println("Shutting down proxy server...")

	// Log shutdown for all endpoints
	for _, logger := range c.loggers {
		_ = logger.LogProxyStop(true, "", nil)
	}

	// Close shared base logger (ONE close for all endpoints)
	if c.baseLogger != nil {
		_ = c.baseLogger.Close()
	}

	return c.server.Shutdown(ctx)
}
