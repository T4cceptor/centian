package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	centauth "github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/gateway"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file defines the core proxy types shared across the server, endpoint,
// session, and pooled downstream connection layers.

// CentianServer owns the HTTP server, server config, auth, logging, and the
// set of registered proxy endpoints exposed by the running Centian process.
//
// CentianServer is the main process for providing routes and delegating endpoint creation.
type CentianServer struct {
	Name             string
	ServerID         string
	Config           *config.ServerConfig
	Provider         gateway.GatewayProvider
	GlobalProcessors []*config.ProcessorConfig // loaded from GatewayFile on setup/reload
	Mux              *http.ServeMux
	Server           *http.Server
	Logger           *logging.Logger
	Gateways         map[string]*CentianEndpoint
	APIKeys          *centauth.APIKeyStore
	AuthHeader       string
	reloadMu         sync.RWMutex // guards Gateways + Handler during reload
}

// UpstreamSession represents one MCP client session talking to this proxy endpoint.
type UpstreamSession struct {
	id string

	upstreamServer              *mcp.Server
	downstreamConns             map[string]DownstreamConnectionInterface
	registeredTools             map[string]struct{}
	registeredResources         map[string]struct{} // keyed by resource URI
	registeredResourceTemplates map[string]struct{} // keyed by resource URI template
	registeredPrompts           map[string]struct{} // keyed by prompt name

	clientHeaders        http.Header
	identityKey          string
	downstreamSessionKey string
	authData             *AuthData

	protocolVersion         string
	clientCapabilities      *mcp.ClientCapabilities
	roots                   []*mcp.Root
	logLevel                mcp.LoggingLevel
	capabilitiesFingerprint string
	rootsFingerprint        string
	rootsDirty              bool
}

// DownstreamSessionPool owns the reusable downstream connection set for one downstream session key.
type DownstreamSessionPool struct {
	identityKey          string
	downstreamSessionKey string
	downstreamConns      map[string]DownstreamConnectionInterface
	upstreamSessions     map[string]*UpstreamSession
	connecting           map[string]bool
	lastUsed             time.Time
}

// connectionByServerName looks up a connection in a map by server name.
func connectionByServerName(conns map[string]DownstreamConnectionInterface, serverName string) (DownstreamConnectionInterface, error) {
	conn, ok := conns[serverName]
	if !ok {
		return nil, fmt.Errorf("no connection to server %q found", serverName)
	}
	return conn, nil
}

// GetConnectionByServerName returns a downstream connection for the given server name.
func (p *DownstreamSessionPool) GetConnectionByServerName(serverName string) (DownstreamConnectionInterface, error) {
	return connectionByServerName(p.downstreamConns, serverName)
}

// SetConnection sets a downstream connection for the given server name.
func (p *DownstreamSessionPool) SetConnection(serverName string, conn DownstreamConnectionInterface) {
	p.downstreamConns[serverName] = conn
}

// IsConnecting returns true if the connection for the given serverName is currently in state Connecting.
func (p *DownstreamSessionPool) IsConnecting(serverName string) (bool, error) {
	conn, ok := p.downstreamConns[serverName]
	if !ok {
		return false, fmt.Errorf("no connection to server %q found", serverName)
	}
	return conn.GetStatus().IsConnecting(), nil
}

const initialDownstreamReadyWait = 500 * time.Millisecond

// GetConnectionByServerName returns a downstream connection for the given server name.
func (s *UpstreamSession) GetConnectionByServerName(serverName string) (DownstreamConnectionInterface, error) {
	return connectionByServerName(s.downstreamConns, serverName)
}

func (s *UpstreamSession) downstreamClientState() *DownstreamClientState {
	return buildDownstreamClientState(s.protocolVersion, s.clientCapabilities, s.roots)
}

// GetAuthHeaders returns the subset of client headers that should be forwarded
// to downstream servers as authentication headers.
func (s *UpstreamSession) GetAuthHeaders(excludedHeader string) map[string]string {
	return getAuthHeaders(s.clientHeaders, excludedHeader)
}

// CentianEndpoint manages one proxied MCP endpoint, including upstream sessions,
// downstream pooling, tool registration, and request routing.
type CentianEndpoint struct {
	name     string // gatewayName or serverName - depending on if the endpoint is aggregated or not
	endpoint string

	upstreamSessions map[string]*UpstreamSession
	downstreamPools  map[string]*DownstreamSessionPool

	isAggregatedProxy bool

	server           *CentianServer
	config           *config.GatewayConfig
	globalProcessors []*config.ProcessorConfig // global processors from the gateway file

	eventProcessor ProcessingControllerInterface

	mu sync.RWMutex

	toolRegMu sync.Mutex

	connectionFactory func(string, *config.MCPServerConfig) DownstreamConnectionInterface
}


// NewAggregatedEndpoint creates an endpoint that aggregates multiple downstream servers.
func NewAggregatedEndpoint(gatewayName, endpoint string, gatewayConfig *config.GatewayConfig) *CentianEndpoint {
	proxy := &CentianEndpoint{
		name:              gatewayName,
		endpoint:          endpoint,
		config:            gatewayConfig,
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamSessionPool),
		isAggregatedProxy: true,
	}
	return proxy
}

// NewSingleEndpoint creates an endpoint for a single downstream server.
func NewSingleEndpoint(serverName, endpoint string, gatewayConfig *config.GatewayConfig) *CentianEndpoint {
	return &CentianEndpoint{
		name:             serverName,
		endpoint:         endpoint,
		config:           gatewayConfig,
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
	}
}

// GetActiveMCPServerConfigs returns the downstream server configs that this
// endpoint should actively use at runtime.
func (p *CentianEndpoint) GetActiveMCPServerConfigs() map[string]*config.MCPServerConfig {
	activeConfigs := make(map[string]*config.MCPServerConfig)
	if p == nil || p.config == nil {
		return activeConfigs
	}

	if p.isAggregatedProxy {
		for serverName, serverConfig := range p.config.MCPServers {
			if serverConfig != nil && serverConfig.IsEnabled() {
				activeConfigs[serverName] = serverConfig
			}
		}
		return activeConfigs
	}

	serverConfig, ok := p.config.MCPServers[p.name]
	if ok && serverConfig != nil && serverConfig.IsEnabled() {
		activeConfigs[p.name] = serverConfig
	}
	return activeConfigs
}

// sanitizeLogValue strips control characters to prevent log injection.
func sanitizeLogValue(value string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r':
			return ' '
		case r < 32 || r == 127:
			return -1
		default:
			return r
		}
	}, value)
	return strings.TrimSpace(sanitized)
}

func copyToolForRegistration(tool *mcp.Tool) *mcp.Tool {
	return &mcp.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		Annotations:  tool.Annotations,
		Meta:         tool.Meta,
		OutputSchema: tool.OutputSchema,
		Title:        tool.Title,
		Icons:        tool.Icons,
	}
}

func copyResourceForRegistration(resource *mcp.Resource) *mcp.Resource {
	return &mcp.Resource{
		Annotations: resource.Annotations,
		Description: resource.Description,
		MIMEType:    resource.MIMEType,
		Name:        resource.Name,
		Title:       resource.Title,
		URI:         resource.URI,
		Meta:        resource.Meta,
		Icons:       resource.Icons,
	}
}

func copyResourceTemplateForRegistration(resourceTemplate *mcp.ResourceTemplate) *mcp.ResourceTemplate {
	return &mcp.ResourceTemplate{
		Annotations: resourceTemplate.Annotations,
		Description: resourceTemplate.Description,
		MIMEType:    resourceTemplate.MIMEType,
		Name:        resourceTemplate.Name,
		Title:       resourceTemplate.Title,
		URITemplate: resourceTemplate.URITemplate,
		Meta:        resourceTemplate.Meta,
		Icons:       resourceTemplate.Icons,
	}
}
