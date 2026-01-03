package proxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

/*
Full config example:

	{
		"gateways": [
			{
				"name": "my-gateway",
				"mcpServers": {
					"my-mcp-proxy-to-github": {
						"url": "https://api.githubcopilot.com/mcp/",
						"headers": {
							"Authorization": "Bearer ${GITHUB_PAT}"
						}
					}
				}
			}
		]
	}
*/
type HttpMcpServerConfig struct {
	URL     string          `json:"url"`
	Headers *map[string]any `json:"headers"`
}

type GatewayConfig struct {
	Name       string                         `json:"name"`
	McpServers map[string]HttpMcpServerConfig `json:"mcpServers"`
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
	ProxyConfiguration ProxyConfig     `json:"-"`
	Gateways           []GatewayConfig `json:"gateways"`
}

func NewHTTPProxyConfig(url string, headers map[string]any) HttpMcpServerConfig {
	return HttpMcpServerConfig{
		URL:     url,
		Headers: &headers,
	}
}

func (pc *HttpMcpServerConfig) GetSubstitutedHeaders() map[string]any {
	// TODO: actually perform env var substitution on each header field
	return *pc.Headers
}

func StartCentianServer(config *CentianConfig) {
	// 1. Iterate through each gateway to create proxy endpoints
	for _, gatewayConfig := range config.Gateways {
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
		for serverName, serverConfig := range gatewayConfig.McpServers {
			// create endpoint:
			endpoint := fmt.Sprintf("/mcp/%s/%s", gatewayConfig.Name, serverName)
			// TODO: add verification for both gatewayname and serverName to be used in a URL

			// TODO: add logging and processor logic
			StartMCPProxy(endpoint, &serverConfig)
		}
	}

	// last step: Start the Proxy Server
	addr := fmt.Sprintf(":%s", config.ProxyConfiguration.Port)
	log.Printf("Proxy server started on %s", config.ProxyConfiguration.Port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func StartMCPProxy(endpoint string, config *HttpMcpServerConfig) {
	// 1. Get target URL from config
	target, err := url.Parse(config.URL)
	if err != nil {
		log.Fatalf("Invalid target URL: %s - Config: %#v", err, config)
	}

	// 2. Get proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// 3. Configure the proxy for streaming (SSE support)
	// FlushInterval: -1 ensures that the data is sent to the client immediately
	// without being buffered, which is essential for text/event-stream.
	proxy.FlushInterval = -1

	// 4. Create the handler function
	// TODO: add correct path pattern (should be "/mcp/<gateway_name>/<server_name>")
	http.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Proxying request: %s %s", r.Method, r.URL.Path)

		// Optional: Update the host header to match the target
		r.Host = target.Host

		// Set headers
		for k, v := range config.GetSubstitutedHeaders() {
			// TODO: check if this any -> string transformation makes sense
			r.Header.Set(k, fmt.Sprintf("%v", v))
		}

		proxy.ServeHTTP(w, r)
	})
}
