package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	centapi "github.com/T4cceptor/centian/internal/api"
	centauth "github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskverification"
)

// This file creates the Centian HTTP server and registers gateway and
// single-server proxy endpoints from config.

// NewCentianServer takes a GlobalConfig struct and returns a new CentianServer.
//
//nolint:gocyclo // Server startup coordinates several subsystems; helpers keep the branches local and explicit.
func NewCentianServer(globalConfig *config.GlobalConfig) (*CentianServer, error) {
	if globalConfig == nil || globalConfig.Proxy == nil {
		return nil, fmt.Errorf("proxy settings are required")
	}

	host := globalConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	if host == "0.0.0.0" && globalConfig.AuthEnabled == nil {
		// TODO: move this into validation
		return nil, fmt.Errorf("auth must be explicitly set when binding to 0.0.0.0")
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:         net.JoinHostPort(host, globalConfig.Proxy.Port),
		Handler:      mux,
		ReadTimeout:  common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
		WriteTimeout: common.GetSecondsFromInt(globalConfig.Proxy.Timeout),
	}
	logger, err := logging.NewLogger()
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	apiKeyStore, err := loadAPIKeyStore(globalConfig)
	if err != nil {
		return nil, err
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to determine current working directory: %w", err)
	}

	taskService, persistenceStore, eventStoreCloser, err := newTaskVerificationService(globalConfig, workingDir, logger)
	if err != nil {
		return nil, err
	}

	centianServer := &CentianServer{
		Config:           globalConfig,
		Mux:              mux,
		Server:           server,
		Logger:           logger,
		ServerID:         getServerID(globalConfig.Name),
		Gateways:         make(map[string]*CentianEndpoint),
		Endpoints:        []*CentianEndpoint{},
		APIKeys:          apiKeyStore,
		AuthHeader:       globalConfig.GetAuthHeader(),
		PersistenceStore: persistenceStore,
		TaskVerification: taskService,
		eventStoreCloser: eventStoreCloser,
	}
	server.RegisterOnShutdown(func() {
		for _, err := range centianServer.Close() {
			common.LogWarn("error during centian shutdown cleanup: %v", err)
		}
	})

	return centianServer, nil
}

func loadAPIKeyStore(globalConfig *config.GlobalConfig) (*centauth.APIKeyStore, error) {
	if !globalConfig.IsAuthEnabled() {
		common.LogInfo("API key auth disabled via config\n")
		//nolint:nilnil // nil store is the sentinel that downstream auth middleware is disabled.
		return nil, nil
	}

	apiKeyStore, err := centauth.LoadDefaultAPIKeys()
	if err != nil {
		if errors.Is(err, centauth.ErrAPIKeysNotFound) {
			return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian auth new-key` to create a new api key\nError: %w", err)
		}
		return nil, fmt.Errorf("failed to load api keys: %w", err)
	}
	if apiKeyStore.Count() == 0 {
		common.LogWarn("Auth enabled but no API keys available from %s\n", apiKeyStore.Path())
	} else {
		common.LogInfo("Loaded %d API keys from %s\n", apiKeyStore.Count(), apiKeyStore.Path())
	}
	return apiKeyStore, nil
}

type noopCloser struct{}

// Close implements io.Closer for no-op cleanup paths.
func (noopCloser) Close() error {
	return nil
}

func newTaskVerificationService(
	globalConfig *config.GlobalConfig,
	workingDir string,
	logger *logging.Logger,
) (*taskverification.Service, *persistence.Store, io.Closer, error) {
	templateDir := resolveTaskTemplatesPath(globalConfig.Proxy, workingDir)
	taskService := taskverification.NewService(templateDir, workingDir)
	eventStorage := globalConfig.Proxy.EventStorageCapability()
	if eventStorage != nil && !eventStorage.IsEnabled() {
		return taskService, nil, noopCloser{}, nil
	}

	storePath, err := resolveEventStorePath(eventStorage)
	if err != nil {
		return nil, nil, nil, err
	}
	store, err := persistence.NewSQLiteStore(storePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize event storage: %w", err)
	}
	taskService.EventStore = store
	logger.SetActionEventStore(store)
	return taskService, store, store, nil
}

func resolveTaskTemplatesPath(settings *config.ProxySettings, workingDir string) string {
	defaultPath := filepath.Join(workingDir, "task-templates")
	if settings == nil {
		return defaultPath
	}
	templatesPath := settings.TaskVerificationCapability().GetTemplatesPath()
	if templatesPath == "" {
		return defaultPath
	}
	if filepath.IsAbs(templatesPath) {
		return templatesPath
	}
	return filepath.Join(workingDir, templatesPath)
}

func resolveEventStorePath(settings *config.EventStorageCapabilitySettings) (string, error) {
	driver := config.DefaultEventStorageDriver
	if settings != nil {
		driver = settings.GetDriver()
	}
	if driver != config.DefaultEventStorageDriver {
		return "", fmt.Errorf("unsupported event storage driver %q", driver)
	}
	if settings != nil && settings.Path != "" {
		return settings.Path, nil
	}
	storePath, err := logging.GetDefaultEventStorePath()
	if err != nil {
		return "", fmt.Errorf("failed to determine default event storage path: %w", err)
	}
	return storePath, nil
}

// Setup uses CentianServer.config to create all gateways and endpoints.
func (c *CentianServer) Setup() error {
	if config.HasOAuthServers(c.Config) {
		publicBaseURL := ""
		if c.Config != nil && c.Config.Proxy != nil && c.Config.Proxy.Web != nil {
			publicBaseURL = c.Config.Proxy.Web.PublicBaseURL
		}
		manager, err := centoauth.NewManager(publicBaseURL, c.handleDownstreamAuthorized, c.handleDownstreamAuthorizationRequired)
		if err != nil {
			return err
		}
		c.OAuth = manager
		c.OAuth.RegisterRoutes(c.Mux)
	}
	if c.PersistenceStore != nil {
		centapi.NewHandler(c.PersistenceStore).RegisterRoutes(c.Mux)
	}

	for gatewayName, gatewayConfig := range c.Config.Gateways {
		endpoint, err := getEndpointString(gatewayName, "")
		if err != nil {
			common.LogError("error creating endpoint for gateway '%s': %s", gatewayName, err.Error())
			continue
		}

		gateway := NewAggregatedEndpoint(gatewayName, endpoint, gatewayConfig)
		gateway.server = c
		c.Gateways[gatewayName] = gateway
		c.Endpoints = append(c.Endpoints, gateway)

		if err := gateway.initEventProcessor(); err != nil {
			return err
		}
		RegisterEndpoint(gateway, c.Mux, nil)

		for serverName := range gateway.GetActiveMCPServerConfigs() {
			if gatewayConfig.MCPServers[serverName] == nil {
				continue
			}
			singleEndpointRoute := fmt.Sprintf("/mcp/%s/%s", gatewayName, serverName)
			singleEndpoint := NewSingleEndpoint(serverName, singleEndpointRoute, gatewayConfig)
			singleEndpoint.server = c
			c.Endpoints = append(c.Endpoints, singleEndpoint)
			if err := singleEndpoint.initEventProcessor(); err != nil {
				return err
			}
			RegisterEndpoint(singleEndpoint, c.Mux, nil)
		}
	}
	return nil
}

// Close releases endpoint-owned resources such as pooled downstream sessions.
func (c *CentianServer) Close() []error {
	if c == nil {
		return nil
	}

	errs := make([]error, 0)
	if c.eventStoreCloser != nil {
		if err := c.eventStoreCloser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, endpoint := range c.Endpoints {
		if endpoint == nil {
			continue
		}
		errs = append(errs, endpoint.Close()...)
	}
	return errs
}

// initEventProcessor initializes the event processor for this ProxyEndpoint.
func (p *CentianEndpoint) initEventProcessor() error {
	if p.server == nil {
		return fmt.Errorf("ProxyEndpoint[%s]: cannot initialize processor without a server reference", p.name)
	}

	var allProcessors []*config.ProcessorConfig
	if p.server.Config.Processors != nil {
		allProcessors = append(allProcessors, p.server.Config.Processors...)
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
