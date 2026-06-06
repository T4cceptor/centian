package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// CentianServerOptions customizes runtime dependencies for server construction.
type CentianServerOptions struct {
	LoggerFactory func(projectSlug string) (*logging.Logger, error)
}

// NewCentianServer takes a GlobalConfig struct and returns a new CentianServer.
// It resolves the config into the project-based layout and initializes per-project
// services (persistence, task verification) for each project.
func NewCentianServer(globalConfig *config.GlobalConfig) (*CentianServer, error) {
	return NewCentianServerWithOptions(globalConfig, CentianServerOptions{})
}

// NewCentianServerWithOptions takes a GlobalConfig struct and explicit runtime
// dependencies, then returns a new CentianServer.
//
//nolint:gocyclo // Server startup coordinates several subsystems; helpers keep the branches local and explicit.
func NewCentianServerWithOptions(globalConfig *config.GlobalConfig, options CentianServerOptions) (*CentianServer, error) {
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
	loggerFactory := options.loggerFactory()
	logger, err := loggerFactory(config.DefaultProjectSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	principals, err := loadPrincipalProvider(globalConfig)
	if err != nil {
		return nil, err
	}
	var authorizer centauth.Authorizer
	if principals != nil {
		authorizer = centauth.DirectGrantAuthorizer{}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to determine current working directory: %w", err)
	}

	centianServer := &CentianServer{
		Config:     globalConfig,
		Mux:        mux,
		Server:     server,
		Logger:     logger,
		ServerID:   getServerID(),
		Projects:   make(map[string]*CentianProject),
		Principals: principals,
		Authorizer: authorizer,
		// AuthHeader is set per-project now, but keep a default for global middleware.
		AuthHeader: config.DefaultAuthHeader,
		// Legacy fields - will be populated from the default project below.
		Gateways:  make(map[string]*CentianEndpoint),
		Endpoints: []*CentianEndpoint{},
	}

	// Initialize per-project state.
	for slug, projectConfig := range globalConfig.Projects {
		project, err := newCentianProject(slug, projectConfig, workingDir, logger, loggerFactory)
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
		centianServer.Logger = defaultProject.Logger
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

func (o CentianServerOptions) loggerFactory() func(projectSlug string) (*logging.Logger, error) {
	if o.LoggerFactory != nil {
		return o.LoggerFactory
	}
	return defaultProjectLogger
}

// newCentianProject initializes per-project runtime state.
func newCentianProject(
	slug string,
	projectConfig *config.ProjectConfig,
	workingDir string,
	defaultLogger *logging.Logger,
	loggerFactory func(projectSlug string) (*logging.Logger, error),
) (*CentianProject, error) {
	projectLogger, err := newProjectLogger(slug, defaultLogger, loggerFactory)
	if err != nil {
		return nil, err
	}
	taskService, persistenceStore, eventStoreCloser, err := newProjectTaskVerificationService(projectConfig, slug, workingDir)
	if err != nil {
		if projectLogger != nil && projectLogger != defaultLogger {
			_ = projectLogger.Close()
		}
		return nil, err
	}
	if projectLogger != nil {
		projectLogger.SetActionEventStore(persistenceStore)
	}
	return &CentianProject{
		Slug:             slug,
		Config:           projectConfig,
		Gateways:         make(map[string]*CentianEndpoint),
		Endpoints:        []*CentianEndpoint{},
		Logger:           projectLogger,
		PersistenceStore: persistenceStore,
		TaskVerification: taskService,
		eventStoreCloser: eventStoreCloser,
	}, nil
}

func newProjectLogger(projectSlug string, defaultLogger *logging.Logger, loggerFactory func(projectSlug string) (*logging.Logger, error)) (*logging.Logger, error) {
	if projectSlug == config.DefaultProjectSlug {
		return defaultLogger, nil
	}
	return loggerFactory(projectSlug)
}

func defaultProjectLogger(projectSlug string) (*logging.Logger, error) {
	if projectSlug == config.DefaultProjectSlug {
		return logging.NewLogger()
	}
	logDir, err := resolveProjectLogDir(projectSlug)
	if err != nil {
		return nil, err
	}
	return logging.NewLoggerInDir(logDir)
}

// newProjectTaskVerificationService creates task verification and persistence services for a project.
func newProjectTaskVerificationService(
	projectConfig *config.ProjectConfig,
	projectSlug string,
	workingDir string,
) (*taskverification.Service, *persistence.Store, io.Closer, error) {
	templateDir := resolveProjectTaskTemplatesPath(projectConfig, workingDir)
	eventStorage := projectConfig.EventStorageCapability()
	return buildTaskVerificationService(
		templateDir,
		workingDir,
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

func resolveProjectLogDir(projectSlug string) (string, error) {
	if projectSlug == config.DefaultProjectSlug {
		return logging.GetLogsDirectory()
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine config directory: %w", err)
	}
	return filepath.Join(configDir, "projects", projectSlug, "logs"), nil
}

func loadPrincipalProvider(globalConfig *config.GlobalConfig) (centauth.PrincipalProvider, error) {
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
		//nolint:nilnil // nil provider is the sentinel that downstream auth middleware is disabled.
		return nil, nil
	}

	backendType, store := globalConfig.GetAuthBackend()
	provider, err := centauth.NewPrincipalProvider(backendType, store)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve auth backend: %w", err)
	}
	if err := provider.Setup(context.Background()); err != nil {
		if errors.Is(err, centauth.ErrAPIKeysNotFound) {
			return nil, fmt.Errorf("api key auth enabled but key file not found \n - run `centian auth new-key` to create a new api key\nError: %w", err)
		}
		return nil, fmt.Errorf("failed to load principals: %w", err)
	}
	// Both providers expose Count/Path for startup logging. A sqlite store with no
	// principals is valid on a fresh install (warn instead of failing).
	if counter, ok := provider.(interface {
		Count() int
		Path() string
	}); ok {
		if count := counter.Count(); count == 0 {
			common.LogWarn("Auth enabled but no principals found in %s - run `centian auth new-key`\n", counter.Path())
		} else {
			common.LogInfo("Loaded %d principal(s) from %s\n", count, counter.Path())
		}
	}
	return provider, nil
}

type noopCloser struct{}

// Close implements io.Closer for no-op cleanup paths.
func (noopCloser) Close() error {
	return nil
}

func buildTaskVerificationService(
	templateDir string,
	workingDir string,
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
	return taskService, store, store, nil
}

// Setup uses CentianServer.config to create all gateways and endpoints for every project.
func (c *CentianServer) Setup() error {
	c.registerProjectHTTPRoutes()

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
		c.Logger = defaultProject.Logger
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

// registerProjectHTTPRoutes registers project-aware API routes and the shared UI.
func (c *CentianServer) registerProjectHTTPRoutes() {
	if c == nil || c.Mux == nil {
		return
	}

	centapi.NewProjectsHandler(c.projectSummaries()).WithFilter(func(r *http.Request, project centapi.ProjectSummary) bool {
		authData := getAuthData(r.Context())
		if authData == nil || authData.Principal == nil || c.Authorizer == nil {
			return true
		}
		return c.Authorizer.Authorize(r.Context(), authData.Principal, centauth.ActionAccess, centauth.ProjectResource(project.Slug)) == nil
	}).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
		return wrapWithAPIKeyAuth(c, "", next)
	})

	uiEnabled := false
	for _, project := range c.Projects {
		if project == nil || project.PersistenceStore == nil {
			continue
		}
		slug := project.Slug
		prefix := "/api/" + slug
		centapi.NewHandler(project.PersistenceStore).RegisterRoutesWithPrefix(c.Mux, prefix, func(next http.Handler) http.Handler {
			return wrapWithAPIKeyAuth(c, slug, next)
		})
		centapi.NewEventsHandler(project.PersistenceStore).RegisterRoutesWithPrefix(c.Mux, prefix, func(next http.Handler) http.Handler {
			return wrapWithAPIKeyAuth(c, slug, next)
		})

		if project.Config != nil && project.Config.UIEnabled() {
			uiEnabled = true
		}
	}

	if defaultProject, ok := c.Projects[config.DefaultProjectSlug]; ok && defaultProject != nil && defaultProject.PersistenceStore != nil {
		centapi.NewHandler(defaultProject.PersistenceStore).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
			return wrapWithAPIKeyAuth(c, config.DefaultProjectSlug, next)
		})
		centapi.NewEventsHandler(defaultProject.PersistenceStore).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
			return wrapWithAPIKeyAuth(c, config.DefaultProjectSlug, next)
		})
		centapi.NewBenchmarkHandler(benchmarks.NewReadService(defaultProject.PersistenceStore)).RegisterRoutesWithMiddleware(c.Mux, func(next http.Handler) http.Handler {
			return wrapWithAPIKeyAuth(c, config.DefaultProjectSlug, next)
		})
	}

	if uiEnabled {
		centui.NewHandler().RegisterRoutes(c.Mux)
	}
}

func (c *CentianServer) projectSummaries() []centapi.ProjectSummary {
	if c == nil || len(c.Projects) == 0 {
		return []centapi.ProjectSummary{}
	}
	slugs := make([]string, 0, len(c.Projects))
	for slug := range c.Projects {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	summaries := make([]centapi.ProjectSummary, 0, len(slugs))
	for _, slug := range slugs {
		project := c.Projects[slug]
		if project == nil || project.Config == nil {
			continue
		}
		summaries = append(summaries, centapi.ProjectSummary{
			Slug:                    slug,
			Name:                    projectDisplayName(slug, project),
			Description:             project.Config.Description,
			IsDefault:               slug == config.DefaultProjectSlug,
			UIEnabled:               project.Config.UIEnabled(),
			EventStorageEnabled:     project.PersistenceStore != nil,
			TaskVerificationEnabled: project.Config.TaskVerificationEnabled(),
		})
	}
	return summaries
}

func projectDisplayName(slug string, project *CentianProject) string {
	if project != nil && project.Config != nil && project.Config.Metadata != nil {
		if name, ok := project.Config.Metadata["name"].(string); ok {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				return trimmed
			}
		}
	}
	return slug
}

// Close releases endpoint-owned resources such as pooled downstream sessions.
func (c *CentianServer) Close() []error {
	if c == nil {
		return nil
	}

	errs := make([]error, 0)

	// Close the principal provider (releases any backend resources).
	if c.Principals != nil {
		if err := c.Principals.Close(); err != nil {
			errs = append(errs, err)
		}
	}

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
	if project.Logger != nil {
		if err := project.Logger.Close(); err != nil {
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
