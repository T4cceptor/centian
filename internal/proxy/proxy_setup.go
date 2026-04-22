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
	"github.com/T4cceptor/centian/internal/benchmarks"
	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	centoauth "github.com/T4cceptor/centian/internal/oauth"
	"github.com/T4cceptor/centian/internal/persistence"
	"github.com/T4cceptor/centian/internal/taskverification"
	centui "github.com/T4cceptor/centian/internal/ui"
)

// This file creates the Centian HTTP server and registers gateway and
// single-server proxy endpoints from config.

// NewCentianServer takes a GlobalConfig struct and returns a new CentianServer.
// It resolves the config into the project-based layout and initializes per-project
// services (persistence, task verification) for each project.
//
//nolint:gocyclo // Server startup coordinates several subsystems; helpers keep the branches local and explicit.
func NewCentianServer(globalConfig *config.GlobalConfig) (*CentianServer, error) {
	if globalConfig == nil || globalConfig.Proxy == nil {
		return nil, fmt.Errorf("proxy settings are required")
	}

	// Normalize config to project-based layout.
	globalConfig.ResolveProjects()

	host := globalConfig.Proxy.Host
	if host == "" {
		host = config.DefaultProxyHost
	}
	if host == "0.0.0.0" {
		// When binding to all interfaces, all projects must have auth explicitly configured.
		for slug, project := range globalConfig.Projects {
			if project != nil && project.AuthEnabled == nil {
				return nil, fmt.Errorf("auth must be explicitly set when binding to 0.0.0.0 (project '%s')", slug)
			}
		}
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

	centianServer := &CentianServer{
		Config:   globalConfig,
		Mux:      mux,
		Server:   server,
		Logger:   logger,
		ServerID: getServerID(),
		Projects: make(map[string]*CentianProject),
		APIKeys:  apiKeyStore,
		// AuthHeader is set per-project now, but keep a default for global middleware.
		AuthHeader: config.DefaultAuthHeader,
		// Legacy fields - will be populated from the default project below.
		Gateways:  make(map[string]*CentianEndpoint),
		Endpoints: []*CentianEndpoint{},
	}

	// Initialize per-project state.
	for slug, projectConfig := range globalConfig.Projects {
		project, err := newCentianProject(slug, projectConfig, workingDir, logger)
		if err != nil {
			return nil, fmt.Errorf("project '%s': %w", slug, err)
		}
		centianServer.Projects[slug] = project
	}

	// Populate legacy flat-access fields from the default project.
	if defaultProject, ok := centianServer.Projects[config.DefaultProjectSlug]; ok {
		centianServer.Gateways = defaultProject.Gateways
		centianServer.Endpoints = defaultProject.Endpoints
		centianServer.OAuth = defaultProject.OAuth
		centianServer.PersistenceStore = defaultProject.PersistenceStore
		centianServer.TaskVerification = defaultProject.TaskVerification
		centianServer.eventStoreCloser = defaultProject.eventStoreCloser
		centianServer.AuthHeader = defaultProject.Config.GetAuthHeader()
	}

	server.RegisterOnShutdown(func() {
		for _, err := range centianServer.Close() {
			common.LogWarn("error during centian shutdown cleanup: %v", err)
		}
	})

	return centianServer, nil
}

// newCentianProject initializes per-project runtime state.
func newCentianProject(
	slug string,
	projectConfig *config.ProjectConfig,
	workingDir string,
	logger *logging.Logger,
) (*CentianProject, error) {
	taskService, persistenceStore, eventStoreCloser, err := newProjectTaskVerificationService(projectConfig, slug, workingDir, logger)
	if err != nil {
		return nil, err
	}
	return &CentianProject{
		Slug:             slug,
		Config:           projectConfig,
		Gateways:         make(map[string]*CentianEndpoint),
		Endpoints:        []*CentianEndpoint{},
		PersistenceStore: persistenceStore,
		TaskVerification: taskService,
		eventStoreCloser: eventStoreCloser,
	}, nil
}

// newProjectTaskVerificationService creates task verification and persistence services for a project.
func newProjectTaskVerificationService(
	projectConfig *config.ProjectConfig,
	projectSlug string,
	workingDir string,
	logger *logging.Logger,
) (*taskverification.Service, *persistence.Store, io.Closer, error) {
	templateDir := resolveProjectTaskTemplatesPath(projectConfig, workingDir)
	eventStorage := projectConfig.EventStorageCapability()
	return buildTaskVerificationService(
		templateDir,
		workingDir,
		logger,
		eventStorage == nil || eventStorage.IsEnabled(),
		func() (string, error) {
			return resolveProjectEventStorePath(eventStorage, projectSlug)
		},
	)
}

func resolveProjectTaskTemplatesPath(projectConfig *config.ProjectConfig, workingDir string) string {
	defaultPath := filepath.Join(workingDir, "task-templates")
	if projectConfig == nil {
		return defaultPath
	}
	templatesPath := projectConfig.TaskVerificationCapability().GetTemplatesPath()
	if templatesPath == "" {
		return defaultPath
	}
	if filepath.IsAbs(templatesPath) {
		return templatesPath
	}
	return filepath.Join(workingDir, templatesPath)
}

// resolveProjectEventStorePath determines the SQLite path for a project.
// For the "default" project, this falls back to the global default path.
// For named projects, it uses ~/.centian/projects/<slug>/events.sqlite.
func resolveProjectEventStorePath(settings *config.EventStorageCapabilitySettings, projectSlug string) (string, error) {
	driver := config.DefaultEventStorageDriver
	if settings != nil {
		driver = settings.GetDriver()
	}
	if driver != config.DefaultEventStorageDriver {
		return "", fmt.Errorf("unsupported event storage driver %q", driver)
	}
	// Explicit path overrides everything.
	if settings != nil && settings.Path != "" {
		return settings.Path, nil
	}
	// Default project uses the legacy global path.
	if projectSlug == config.DefaultProjectSlug {
		storePath, err := logging.GetDefaultEventStorePath()
		if err != nil {
			return "", fmt.Errorf("failed to determine default event storage path: %w", err)
		}
		return storePath, nil
	}
	// Named projects get their own directory.
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine config directory: %w", err)
	}
	projectDir := filepath.Join(configDir, "projects", projectSlug)
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}
	return filepath.Join(projectDir, "events.sqlite"), nil
}

func loadAPIKeyStore(globalConfig *config.GlobalConfig) (*centauth.APIKeyStore, error) {
	// Check if any project has auth enabled.
	anyAuthEnabled := false
	if len(globalConfig.Projects) > 0 {
		for _, project := range globalConfig.Projects {
			if project != nil && project.IsAuthEnabled() {
				anyAuthEnabled = true
				break
			}
		}
	} else {
		// Fallback: check legacy flat field (before ResolveProjects).
		anyAuthEnabled = globalConfig.IsAuthEnabled()
	}

	if !anyAuthEnabled {
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

func buildTaskVerificationService(
	templateDir string,
	workingDir string,
	logger *logging.Logger,
	eventStorageEnabled bool,
	resolveStorePath func() (string, error),
) (*taskverification.Service, *persistence.Store, io.Closer, error) {
	taskService := taskverification.NewService(templateDir, workingDir)
	if !eventStorageEnabled {
		return taskService, nil, noopCloser{}, nil
	}

	storePath, err := resolveStorePath()
	if err != nil {
		return nil, nil, nil, err
	}
	store, err := persistence.NewSQLiteStore(storePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize event storage: %w", err)
	}
	taskService.EventStore = store
	taskService.RunStore = store
	logger.SetActionEventStore(store)
	return taskService, store, store, nil
}

// Setup uses CentianServer.config to create all gateways and endpoints for every project.
func (c *CentianServer) Setup() error {
	for slug, project := range c.Projects {
		if err := c.setupProject(slug, project); err != nil {
			return fmt.Errorf("project '%s': %w", slug, err)
		}
	}

	// Sync legacy flat-access fields from the default project.
	if defaultProject, ok := c.Projects[config.DefaultProjectSlug]; ok {
		c.Gateways = defaultProject.Gateways
		c.Endpoints = defaultProject.Endpoints
		c.OAuth = defaultProject.OAuth
		c.PersistenceStore = defaultProject.PersistenceStore
		c.TaskVerification = defaultProject.TaskVerification
	}

	return nil
}

// setupProject configures routes and services for a single project.
func (c *CentianServer) setupProject(slug string, project *CentianProject) error {
	projectConfig := project.Config

	// Determine route prefix. The default project uses no prefix for backwards compatibility.
	routePrefix := ""
	if slug != config.DefaultProjectSlug {
		routePrefix = "/" + slug
	}

	// OAuth setup for this project.
	if projectConfig.HasOAuthServers() {
		publicBaseURL := ""
		if projectConfig.Web != nil {
			publicBaseURL = projectConfig.Web.PublicBaseURL
		}
		manager, err := centoauth.NewManager(publicBaseURL, c.handleDownstreamAuthorized, c.handleDownstreamAuthorizationRequired)
		if err != nil {
			return err
		}
		project.OAuth = manager
		// TODO: add prefix support to OAuth manager for multi-project setups.
		// Currently all projects share the same /oauth/* routes.
		project.OAuth.RegisterRoutes(c.Mux)
	}

	// Register optional HTTP routes (API, UI) for this project.
	c.registerProjectHTTPRoutes(project)

	// Register gateway and server endpoints.
	for gatewayName, gatewayConfig := range projectConfig.Gateways {
		endpointPath := fmt.Sprintf("%s/mcp/%s", routePrefix, gatewayName)
		if !common.IsURLCompliant(gatewayName) {
			common.LogError("error creating endpoint for gateway '%s': name is not URL compliant", gatewayName)
			continue
		}

		gateway := NewAggregatedEndpoint(gatewayName, endpointPath, gatewayConfig)
		gateway.server = c
		gateway.project = project
		project.Gateways[gatewayName] = gateway
		project.Endpoints = append(project.Endpoints, gateway)

		if err := gateway.initEventProcessor(); err != nil {
			return err
		}
		RegisterEndpoint(gateway, c.Mux, nil)

		for serverName := range gateway.GetActiveMCPServerConfigs() {
			if gatewayConfig.MCPServers[serverName] == nil {
				continue
			}
			singleEndpointRoute := fmt.Sprintf("%s/mcp/%s/%s", routePrefix, gatewayName, serverName)
			singleEndpoint := NewSingleEndpoint(serverName, singleEndpointRoute, gatewayConfig)
			singleEndpoint.server = c
			singleEndpoint.project = project
			project.Endpoints = append(project.Endpoints, singleEndpoint)
			if err := singleEndpoint.initEventProcessor(); err != nil {
				return err
			}
			RegisterEndpoint(singleEndpoint, c.Mux, nil)
		}
	}
	return nil
}

// registerProjectHTTPRoutes registers API and UI routes for a project.
func (c *CentianServer) registerProjectHTTPRoutes(project *CentianProject) {
	if project == nil || project.PersistenceStore == nil {
		return
	}

	// TODO: implement prefixed route registration for API and UI handlers
	// in multi-project setups. Currently all projects share the same /api/* and /ui routes.
	handler := centapi.NewHandler(project.PersistenceStore)
	handler.RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
		return wrapWithAPIKeyAuth(c, project.Slug, next)
	})

	centapi.NewEventsHandler(project.PersistenceStore).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
		return wrapWithAPIKeyAuth(c, project.Slug, next)
	})

	centapi.NewBenchmarkHandler(benchmarks.NewReadService(project.PersistenceStore)).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
		return wrapWithAPIKeyAuth(c, project.Slug, next)
	})
	if project.Config != nil && project.Config.UIEnabled() {
		centui.NewHandler().RegisterRoutes(c.Mux)
	}
}

// Close releases endpoint-owned resources such as pooled downstream sessions.
func (c *CentianServer) Close() []error {
	if c == nil {
		return nil
	}

	errs := make([]error, 0)

	// Close project-scoped resources.
	for _, project := range c.Projects {
		errs = append(errs, closeProject(project)...)
	}

	// Close legacy flat-list resources not owned by any project.
	errs = append(errs, c.closeLegacyResources()...)
	return errs
}

func closeProject(project *CentianProject) []error {
	if project == nil {
		return nil
	}
	var errs []error
	if project.eventStoreCloser != nil {
		if err := project.eventStoreCloser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, endpoint := range project.Endpoints {
		if endpoint != nil {
			errs = append(errs, endpoint.Close()...)
		}
	}
	return errs
}

// closeLegacyResources closes resources in the flat Endpoints list that aren't part of any project.
// This handles tests and code that directly populates CentianServer.Endpoints.
func (c *CentianServer) closeLegacyResources() []error {
	var errs []error
	if c.eventStoreCloser != nil && !c.isEventStoreProjectOwned() {
		if err := c.eventStoreCloser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, endpoint := range c.Endpoints {
		if endpoint == nil || endpoint.project != nil {
			continue
		}
		errs = append(errs, endpoint.Close()...)
	}
	return errs
}

func (c *CentianServer) isEventStoreProjectOwned() bool {
	for _, project := range c.Projects {
		if project != nil && project.eventStoreCloser == c.eventStoreCloser {
			return true
		}
	}
	return false
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
