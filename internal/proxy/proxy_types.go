package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	centauth "github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file defines the core proxy types shared across the server, endpoint,
// session, and pooled downstream connection layers.

// CentianServer owns the HTTP server, global config, auth, logging, and the
// set of registered proxy endpoints exposed by the running Centian process.
//
// CentianServer is the main process for providing routes and delegating endpoint creation.
type CentianServer struct {
	Name       string
	ServerID   string
	Config     *config.GlobalConfig
	Mux        *http.ServeMux
	Server     *http.Server
	Logger     *logging.Logger
	Gateways   map[string]*CentianEndpoint
	APIKeys    *centauth.APIKeyStore
	AuthHeader string
}

// UpstreamSession represents one MCP client session talking to this proxy endpoint.
type UpstreamSession struct {
	id string

	upstreamServer  *mcp.Server
	downstreamConns map[string]DownstreamConnectionInterface
	registeredTools map[string]struct{}

	clientHeaders        http.Header
	identityKey          string
	downstreamSessionKey string
	authData             *AuthData

	protocolVersion         string
	clientCapabilities      *mcp.ClientCapabilities
	roots                   []*mcp.Root
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

// GetConnectionByServerName returns a downstream connection for the given server name.
func (p *DownstreamSessionPool) GetConnectionByServerName(serverName string) (DownstreamConnectionInterface, error) {
	conn, ok := p.downstreamConns[serverName]
	if !ok {
		return nil, fmt.Errorf("no connection to server %q found", serverName)
	}
	return conn, nil
}

const initialDownstreamReadyWait = 500 * time.Millisecond

// GetConnectionByServerName returns a downstream connection for the given server name.
func (s *UpstreamSession) GetConnectionByServerName(serverName string) (DownstreamConnectionInterface, error) {
	conn, ok := s.downstreamConns[serverName]
	if !ok {
		return nil, fmt.Errorf("no connection to server %q found", serverName)
	}
	return conn, nil
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
	name     string
	endpoint string

	downstreams map[string]*DownstreamConnection

	upstreamSessions map[string]*UpstreamSession
	downstreamPools  map[string]*DownstreamSessionPool

	isAggregatedProxy bool

	server *CentianServer
	config *config.GatewayConfig

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
		downstreams:       make(map[string]*DownstreamConnection),
		upstreamSessions:  make(map[string]*UpstreamSession),
		downstreamPools:   make(map[string]*DownstreamSessionPool),
		isAggregatedProxy: true,
	}

	for serverName, serverConfig := range gatewayConfig.MCPServers {
		if serverConfig.IsEnabled() {
			proxy.downstreams[serverName] = NewDownstreamConnection(serverName, serverConfig)
		}
	}

	return proxy
}

// NewSingleEndpoint creates an endpoint for a single downstream server.
func NewSingleEndpoint(serverName, endpoint string, serverConfig *config.MCPServerConfig) *CentianEndpoint {
	return &CentianEndpoint{
		name:     serverName,
		endpoint: endpoint,
		downstreams: map[string]*DownstreamConnection{
			serverName: NewDownstreamConnection(serverName, serverConfig),
		},
		upstreamSessions: make(map[string]*UpstreamSession),
		downstreamPools:  make(map[string]*DownstreamSessionPool),
	}
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

// DownstreamConnectionPool is kept as a compatibility alias for existing tests/helpers.
type DownstreamConnectionPool = DownstreamSessionPool
