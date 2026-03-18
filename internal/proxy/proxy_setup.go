package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	centauth "github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/gateway"
	"github.com/T4cceptor/centian/internal/logging"
)

// This file creates the Centian HTTP server and registers gateway and
// single-server proxy endpoints from config.

// NewCentianServer creates a new CentianServer from a ServerConfig and a GatewayProvider.
func NewCentianServer(serverConfig *config.ServerConfig, provider gateway.GatewayProvider) (*CentianServer, error) {
	if serverConfig == nil || serverConfig.Proxy == nil {
		return nil, fmt.Errorf("proxy settings are required")
	}

	host := serverConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	if host == "0.0.0.0" && serverConfig.AuthEnabled == nil {
		// TODO: move this into validation
		return nil, fmt.Errorf("auth must be explicitly set when binding to 0.0.0.0")
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         net.JoinHostPort(host, serverConfig.Proxy.Port),
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(serverConfig.Proxy.Timeout),
		WriteTimeout: common.GetSecondsFromInt(serverConfig.Proxy.Timeout),
	}
	logger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	var apiKeyStore *centauth.APIKeyStore
	if serverConfig.IsAuthEnabled() {
		loadedStore, err := centauth.LoadDefaultAPIKeys()
		if err != nil {
			if errors.Is(err, centauth.ErrAPIKeysNotFound) {
				return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian auth new-key` to create a new api key\nError: %w", err)
			}
			return nil, fmt.Errorf("failed to load api keys: %w", err)
		}
		apiKeyStore = loadedStore
		if apiKeyStore.Count() == 0 {
			common.LogWarn("Auth enabled but no API keys available from %s\n", apiKeyStore.Path())
		} else {
			common.LogInfo("Loaded %d API keys from %s\n", apiKeyStore.Count(), apiKeyStore.Path())
		}
	} else {
		common.LogInfo("API key auth disabled via config\n")
	}

	return &CentianServer{
		Config:     serverConfig,
		Provider:   provider,
		Mux:        mux,
		Server:     server,
		Logger:     logger,
		ServerID:   getServerID(serverConfig.Name),
		Gateways:   make(map[string]*CentianEndpoint),
		APIKeys:    apiKeyStore,
		AuthHeader: serverConfig.GetAuthHeader(),
	}, nil
}

// Setup loads the gateway file from the provider and creates all gateway endpoints.
func (c *CentianServer) Setup() error {
	file, err := c.Provider.LoadGatewayFile()
	if err != nil {
		return fmt.Errorf("failed to load gateway file: %w", err)
	}
	if err := config.ValidateGatewayFile(file, true); err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	mux, gateways, err := c.buildGatewayMux(file.GlobalProcessors, file.Gateways)
	if err != nil {
		return err
	}

	c.GlobalProcessors = file.GlobalProcessors
	c.Gateways = gateways
	c.registerAdminEndpoints(mux)
	c.Mux = mux
	c.Server.Handler = mux
	return nil
}

// ReloadGateways reloads gateway configuration from the provider without restarting the server.
// Active sessions are terminated during reload. On error, the existing gateways remain active.
func (c *CentianServer) ReloadGateways() error {
	file, err := c.Provider.LoadGatewayFile()
	if err != nil {
		return fmt.Errorf("failed to load gateway file: %w", err)
	}
	if err := config.ValidateGatewayFile(file, true); err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	newMux, newGateways, err := c.buildGatewayMux(file.GlobalProcessors, file.Gateways)
	if err != nil {
		return err
	}
	c.registerAdminEndpoints(newMux)

	c.reloadMu.Lock()
	// Close existing gateways to tear down downstream sessions.
	for _, ep := range c.Gateways {
		_ = ep.Close()
	}
	c.GlobalProcessors = file.GlobalProcessors
	c.Gateways = newGateways
	c.Mux = newMux
	c.Server.Handler = newMux
	c.reloadMu.Unlock()

	common.LogInfo("Reloaded gateway configuration: %d gateways active", len(newGateways))
	return nil
}

// buildGatewayMux constructs a new ServeMux and endpoint map from the given gateway configs.
func (c *CentianServer) buildGatewayMux(
	globalProcessors []*config.ProcessorConfig,
	gateways map[string]*config.GatewayConfig,
) (*http.ServeMux, map[string]*CentianEndpoint, error) {
	mux := http.NewServeMux()
	endpoints := make(map[string]*CentianEndpoint)

	for gatewayName, gatewayConfig := range gateways {
		endpointPath, err := getEndpointString(gatewayName, "")
		if err != nil {
			common.LogError("error creating endpoint for gateway '%s': %s", gatewayName, err.Error())
			continue
		}

		ep := NewAggregatedEndpoint(gatewayName, endpointPath, gatewayConfig)
		ep.server = c
		ep.globalProcessors = globalProcessors
		endpoints[gatewayName] = ep

		if err := ep.initEventProcessor(); err != nil {
			return nil, nil, err
		}
		RegisterEndpoint(ep, mux, nil)

		for serverName := range ep.GetActiveMCPServerConfigs() {
			if gatewayConfig.MCPServers[serverName] == nil {
				continue
			}
			singleEndpointRoute := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			singleEndpoint := NewSingleEndpoint(serverName, singleEndpointRoute, gatewayConfig)
			singleEndpoint.server = c
			singleEndpoint.globalProcessors = globalProcessors
			if err := singleEndpoint.initEventProcessor(); err != nil {
				return nil, nil, err
			}
			RegisterEndpoint(singleEndpoint, mux, nil)
		}
	}
	return mux, endpoints, nil
}

// registerAdminEndpoints registers the /admin/reload endpoint on the given mux.
func (c *CentianServer) registerAdminEndpoints(mux *http.ServeMux) {
	var handler http.Handler = http.HandlerFunc(c.handleAdminReload)
	if c.APIKeys != nil {
		// Wrap with the existing API key middleware.
		// Admin endpoint requires a key with no gateway restriction (empty gateways = allow all).
		// Scoped keys (restricted to specific gateways) cannot call /admin/reload.
		handler = apiKeyMiddlewareWithHeader(c.APIKeys, c.AuthHeader, handler)
	}
	mux.Handle("/admin/reload", handler)
}

// initEventProcessor initializes the event processor for this ProxyEndpoint.
func (p *CentianEndpoint) initEventProcessor() error {
	if p.server == nil {
		return fmt.Errorf("ProxyEndpoint[%s]: cannot initialize processor without a server reference", p.name)
	}

	var allProcessors []*config.ProcessorConfig
	if p.globalProcessors != nil {
		allProcessors = append(allProcessors, p.globalProcessors...)
	}
	if p.config != nil && p.config.Processors != nil {
		allProcessors = append(allProcessors, p.config.Processors...)
	}

	controller, err := NewProcessingController(allProcessors)
	if err != nil {
		return err
	}
	p.eventProcessor = controller
	if len(allProcessors) > 0 {
		common.LogInfo("ProxyEndpoint[%s]: initialized event processor with %d processors", p.name, len(allProcessors))
	}
	return nil
}
