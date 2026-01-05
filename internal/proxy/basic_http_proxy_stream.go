package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/CentianAI/centian-cli/internal/processor"
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
type HttpMcpServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type GatewayConfig struct {
	AllowDynamicProxy    bool                           `json:"allowDynamic"`
	AllowGatewayEndpoint bool                           `json:"setupGateway"`
	McpServers           map[string]HttpMcpServerConfig `json:"mcpServers"`
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
	ProxyConfiguration ProxyConfig              `json:"-"`
	Gateways           map[string]GatewayConfig `json:"gateways"`
}

type CentianServer struct {
	config         *CentianConfig
	mux            *http.ServeMux
	server         *http.Server
	logger         *logging.HttpLogger
	processorChain *processor.Chain
	serverID       string // used to uniquely identify this specific object instance
}

func (pc *HttpMcpServerConfig) GetSubstitutedHeaders() map[string]string {
	if pc.Headers == nil {
		return make(map[string]string)
	}
	// TODO: actually perform env var substitution on each header field
	return pc.Headers
}

func getSecondsFromInt(i int) time.Duration {
	return time.Duration(i) * time.Second
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
		ReadTimeout:  getSecondsFromInt(config.ProxyConfiguration.Timeout),
		WriteTimeout: getSecondsFromInt(config.ProxyConfiguration.Timeout),
	}

	// TODO: here we now have ONE logger for multiple endpoints - if we want to use this we
	// have to have a better way to store the URL and headers, as they are endpoint specific!
	logger, err := logging.NewHttpLogger("https://locahost:9000", nil)
	if err != nil {
		return nil, err
	}
	// TODO: add processor chain -> TODO: check how the config needs to look like!
	return &CentianServer{
		config:   config,
		mux:      mux,
		server:   server,
		logger:   logger,
		serverID: getServerID(config),
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
	for gatewayName, gatewayConfig := range config.Gateways {
		// 2. Iterate through each mcp server config for this gateway
		for serverName, serverConfig := range gatewayConfig.McpServers {
			// 3. create new endpoint for MCP server to be proxied
			endpoint := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			// TODO: add verification for both gatewayname and serverName to be used in a URL
			// TODO: add logging and processor logic
			// 4. attach proxy endpoint
			if err := c.RegisterProxy(endpoint, &serverConfig); err != nil {
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

func (c *CentianServer) LogHTTPRequest(r *http.Request, lifecycle string) {
	// TODO
	// requestID := fmt.Sprintf("http_request_%s", time.Now().UnixMicro()) // TODO: get real request ID -> maybe even from the request itself?
	// sessionID := ""
	// command := "" // here we HAVE to get the JSON RPC structure from the request body
	// args := []string{
	// 	"test",
	// }
	// c.logger.LogRequest(requestID, sessionID, command, args, c.serverID, "")
}

func (c *CentianServer) RegisterProxy(endpoint string, config *HttpMcpServerConfig) error {
	// 1. Get target URL from config
	target, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid URL for endpoint %s: %w", endpoint, err)
	}

	// 2. Get proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// 3. Configure the proxy for streaming (SSE support)
	// FlushInterval: -1 ensures that the data is sent to the client immediately
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1

	// deal with headers via Director
	headers := config.GetSubstitutedHeaders()
	proxy.Director = func(r *http.Request) {
		// TODO: add logging and processing before forwarding request to MCP server
		c.LogHTTPRequest(r, "")
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.Host = target.Host
		for k, v := range headers {
			r.Header.Set(k, v)
		}
	}

	// 4. Create the handler function
	c.mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Proxying request: %s %s", r.Method, r.URL.Path)
		proxy.ServeHTTP(w, r)
	})
	log.Printf("Registered proxy endpoint: %s -> %s", endpoint, config.URL)
	return nil
}

func (c *CentianServer) Shutdown(ctx context.Context) error {
	log.Println("Shutting down proxy server...")
	return c.server.Shutdown(ctx)
}
