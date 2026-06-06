package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/persistence"
	"gotest.tools/assert"
)

// testEchoProcessorConfig returns a minimal valid processor config for use in tests.
// The processor is never executed during setup, so the command only needs to be creatable.
func testEchoProcessorConfig(name string) *config.ProcessorConfig {
	return &config.ProcessorConfig{
		Name:    name,
		Type:    "cli",
		Enabled: true,
		Timeout: 10,
		Config: map[string]interface{}{
			"command": "echo",
		},
	}
}

func testProjectConfig(
	slug string,
	description string,
	authEnabled *bool,
	uiEnabled *bool,
	taskVerificationEnabled *bool,
	eventStorePath string,
) *config.ProjectConfig {
	eventStorageEnabled := true
	return &config.ProjectConfig{
		Slug:        slug,
		Description: description,
		AuthEnabled: authEnabled,
		Capabilities: &config.CapabilitiesSettings{
			TaskVerification: &config.TaskVerificationCapabilitySettings{
				Enabled: taskVerificationEnabled,
			},
			EventStorage: &config.EventStorageCapabilitySettings{
				Enabled: &eventStorageEnabled,
				Driver:  config.DefaultEventStorageDriver,
				Path:    eventStorePath,
			},
			UI: &config.UICapabilitySettings{
				Enabled: uiEnabled,
			},
		},
		Gateways: map[string]*config.GatewayConfig{},
	}
}

func TestCentianServerSetup_RegistersHandlers(t *testing.T) {
	// Given: a global config with auth disabled and a gateway
	authDisabled := false
	enabled := true
	disabled := false
	uiEnabled := true
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
			Capabilities: &config.CapabilitiesSettings{
				UI: &config.UICapabilitySettings{
					Enabled: &uiEnabled,
				},
			},
		},
		Gateways: map[string]*config.GatewayConfig{
			"gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"enabled":  {Command: "node", Enabled: &enabled},
					"disabled": {Command: "node", Enabled: &disabled},
				},
			},
		},
	}
	// Ensure logger writes to temp HOME
	t.Setenv("HOME", t.TempDir())

	proxy, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)

	// When: setting up the proxy
	err = proxy.Setup()

	// Then: aggregated and single endpoints are registered
	assert.NilError(t, err)

	aggregatedReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", http.NoBody)
	aggregatedHandler, aggregatedPattern := proxy.Mux.Handler(aggregatedReq)
	assert.Assert(t, aggregatedHandler != nil)
	assert.Equal(t, aggregatedPattern, "/mcp/gateway")

	singleReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/enabled", http.NoBody)
	singleHandler, singlePattern := proxy.Mux.Handler(singleReq)
	assert.Assert(t, singleHandler != nil)
	assert.Equal(t, singlePattern, "/mcp/gateway/enabled")

	// Then: disabled endpoint is not registered
	disabledReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/disabled", http.NoBody)
	_, disabledPattern := proxy.Mux.Handler(disabledReq)
	assert.Equal(t, disabledPattern, "")

	// Then: task run API endpoints are registered when persistence is enabled
	apiListReq, _ := http.NewRequest(http.MethodGet, "http://example.com/api/task-runs", http.NoBody)
	apiListHandler, apiListPattern := proxy.Mux.Handler(apiListReq)
	assert.Assert(t, apiListHandler != nil)
	assert.Equal(t, apiListPattern, "GET /api/task-runs")

	validRunID := "tr_1742947200123_0000000001"
	apiEventsReq, _ := http.NewRequest(http.MethodGet, "http://example.com/api/task-runs/"+validRunID+"/events", http.NoBody)
	apiEventsHandler, apiEventsPattern := proxy.Mux.Handler(apiEventsReq)
	assert.Assert(t, apiEventsHandler != nil)
	assert.Equal(t, apiEventsPattern, "GET /api/task-runs/{runID}/events")

	// Then: UI routes are registered for the embedded frontend
	uiIndexReq, _ := http.NewRequest(http.MethodGet, "http://example.com/ui", http.NoBody)
	uiIndexHandler, uiIndexPattern := proxy.Mux.Handler(uiIndexReq)
	assert.Assert(t, uiIndexHandler != nil)
	assert.Equal(t, uiIndexPattern, "GET /ui")

	uiTasksReq, _ := http.NewRequest(http.MethodGet, "http://example.com/ui/tasks", http.NoBody)
	uiTasksHandler, uiTasksPattern := proxy.Mux.Handler(uiTasksReq)
	assert.Assert(t, uiTasksHandler != nil)
	assert.Equal(t, uiTasksPattern, "GET /ui/")
}

func TestCentianServerSetup_RegistersProjectScopedAPIRoutes(t *testing.T) {
	authDisabled := false
	uiEnabled := true
	taskVerificationEnabled := true
	defaultStorePath := filepath.Join(t.TempDir(), "default.sqlite")
	researchStorePath := filepath.Join(t.TempDir(), "research.sqlite")
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Projects: map[string]*config.ProjectConfig{
			config.DefaultProjectSlug: testProjectConfig(config.DefaultProjectSlug, "Default project", &authDisabled, &uiEnabled, &taskVerificationEnabled, defaultStorePath),
			"research":                testProjectConfig("research", "Research project", &authDisabled, &uiEnabled, &taskVerificationEnabled, researchStorePath),
		},
	}
	t.Setenv("HOME", t.TempDir())

	proxy, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range proxy.Close() {
			assert.NilError(t, closeErr)
		}
	})

	err = proxy.Setup()
	assert.NilError(t, err)

	projectsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/projects", http.NoBody)
	projectsRec := httptest.NewRecorder()
	proxy.Mux.ServeHTTP(projectsRec, projectsReq)
	assert.Equal(t, projectsRec.Code, http.StatusOK)
	var response struct {
		Projects []struct {
			Slug                    string `json:"slug"`
			Name                    string `json:"name"`
			Description             string `json:"description"`
			IsDefault               bool   `json:"isDefault"`
			UIEnabled               bool   `json:"uiEnabled"`
			EventStorageEnabled     bool   `json:"eventStorageEnabled"`
			TaskVerificationEnabled bool   `json:"taskVerificationEnabled"`
		} `json:"projects"`
	}
	assert.NilError(t, json.Unmarshal(projectsRec.Body.Bytes(), &response))
	assert.Equal(t, len(response.Projects), 2)
	assert.Equal(t, response.Projects[0].Slug, config.DefaultProjectSlug)
	assert.Equal(t, response.Projects[0].Name, config.DefaultProjectSlug)
	assert.Equal(t, response.Projects[0].IsDefault, true)
	assert.Equal(t, response.Projects[0].UIEnabled, true)
	assert.Equal(t, response.Projects[0].EventStorageEnabled, true)
	assert.Equal(t, response.Projects[0].TaskVerificationEnabled, true)
	assert.Equal(t, response.Projects[1].Slug, "research")
	assert.Equal(t, response.Projects[1].Name, "research")
	assert.Equal(t, response.Projects[1].Description, "Research project")

	legacyReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/task-runs", http.NoBody)
	_, legacyPattern := proxy.Mux.Handler(legacyReq)
	assert.Equal(t, legacyPattern, "GET /api/task-runs")

	defaultReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/default/task-runs", http.NoBody)
	_, defaultPattern := proxy.Mux.Handler(defaultReq)
	assert.Equal(t, defaultPattern, "GET /api/default/task-runs")

	researchReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/research/task-runs", http.NoBody)
	_, researchPattern := proxy.Mux.Handler(researchReq)
	assert.Equal(t, researchPattern, "GET /api/research/task-runs")

	researchEventsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/research/events", http.NoBody)
	_, researchEventsPattern := proxy.Mux.Handler(researchEventsReq)
	assert.Equal(t, researchEventsPattern, "GET /api/research/events")

	unknownReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/unknown/task-runs", http.NoBody)
	_, unknownPattern := proxy.Mux.Handler(unknownReq)
	assert.Equal(t, unknownPattern, "")

	scopedBenchmarkReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/research/benchmarks/suites", http.NoBody)
	_, scopedBenchmarkPattern := proxy.Mux.Handler(scopedBenchmarkReq)
	assert.Equal(t, scopedBenchmarkPattern, "")

	legacyBenchmarkReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/benchmarks/suites", http.NoBody)
	_, legacyBenchmarkPattern := proxy.Mux.Handler(legacyBenchmarkReq)
	assert.Equal(t, legacyBenchmarkPattern, "GET /api/benchmarks/suites")
}

func TestCentianServerSetup_FiltersProjectsByAPIKeyScope(t *testing.T) {
	authEnabled := true
	uiEnabled := true
	taskVerificationEnabled := true
	t.Setenv("HOME", t.TempDir())
	entry, err := auth.NewAPIKeyEntry(auth.GeneratedKey{CredID: "key_research", Secret: "secret"})
	assert.NilError(t, err)
	entry.Projects = []string{"research"}
	defaultPath, err := auth.DefaultAPIKeysPath()
	assert.NilError(t, err)
	assert.NilError(t, auth.WriteAPIKeyFile(defaultPath, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authEnabled,
		AuthBackend: &config.AuthBackendSettings{Type: "file"},
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Projects: map[string]*config.ProjectConfig{
			config.DefaultProjectSlug: testProjectConfig(config.DefaultProjectSlug, "Default project", &authEnabled, &uiEnabled, &taskVerificationEnabled, filepath.Join(t.TempDir(), "default.sqlite")),
			"research":                testProjectConfig("research", "Research project", &authEnabled, &uiEnabled, &taskVerificationEnabled, filepath.Join(t.TempDir(), "research.sqlite")),
		},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})
	assert.NilError(t, server.Setup())

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/projects", http.NoBody)
	req.Header.Set(config.DefaultAuthHeader, "Bearer sk-key_research.secret")
	rec := httptest.NewRecorder()
	server.Mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var response struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, len(response.Projects), 1)
	assert.Equal(t, response.Projects[0].Slug, "research")
}

func TestCentianServerSetup_ProjectsAPIAllowAllKeySeesAllProjects(t *testing.T) {
	authEnabled := true
	uiEnabled := true
	taskVerificationEnabled := true
	t.Setenv("HOME", t.TempDir())
	entry, err := auth.NewAPIKeyEntry(auth.GeneratedKey{CredID: "key_all", Secret: "secret"})
	assert.NilError(t, err)
	entry.Projects = []string{"*"}
	defaultPath, err := auth.DefaultAPIKeysPath()
	assert.NilError(t, err)
	assert.NilError(t, auth.WriteAPIKeyFile(defaultPath, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authEnabled,
		AuthBackend: &config.AuthBackendSettings{Type: "file"},
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Projects: map[string]*config.ProjectConfig{
			config.DefaultProjectSlug: testProjectConfig(config.DefaultProjectSlug, "Default project", &authEnabled, &uiEnabled, &taskVerificationEnabled, filepath.Join(t.TempDir(), "default.sqlite")),
			"research":                testProjectConfig("research", "Research project", &authEnabled, &uiEnabled, &taskVerificationEnabled, filepath.Join(t.TempDir(), "research.sqlite")),
		},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})
	assert.NilError(t, server.Setup())

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/projects", http.NoBody)
	req.Header.Set(config.DefaultAuthHeader, "Bearer sk-key_all.secret")
	rec := httptest.NewRecorder()
	server.Mux.ServeHTTP(rec, req)

	assert.Equal(t, rec.Code, http.StatusOK)
	var response struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	assert.NilError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, len(response.Projects), 2)
	assert.Equal(t, response.Projects[0].Slug, config.DefaultProjectSlug)
	assert.Equal(t, response.Projects[1].Slug, "research")
}

func TestProjectSummariesUseConfiguredDisplayName(t *testing.T) {
	authDisabled := false
	uiEnabled := true
	taskVerificationEnabled := true
	project := testProjectConfig("demo", "Demo project", &authDisabled, &uiEnabled, &taskVerificationEnabled, ":memory:")
	project.Metadata = map[string]interface{}{"name": "Centian Demonstration"}
	server := &CentianServer{
		Projects: map[string]*CentianProject{
			"demo": {
				Config:           project,
				PersistenceStore: &persistence.Store{},
			},
		},
	}

	summaries := server.projectSummaries()

	assert.Equal(t, len(summaries), 1)
	assert.Equal(t, summaries[0].Slug, "demo")
	assert.Equal(t, summaries[0].Name, "Centian Demonstration")
}

func TestNewCentianServerWithOptionsUsesLoggerFactory(t *testing.T) {
	authDisabled := false
	uiEnabled := true
	taskVerificationEnabled := true
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Projects: map[string]*config.ProjectConfig{
			config.DefaultProjectSlug: testProjectConfig(config.DefaultProjectSlug, "Default project", &authDisabled, &uiEnabled, &taskVerificationEnabled, ":memory:"),
			"research":                testProjectConfig("research", "Research project", &authDisabled, &uiEnabled, &taskVerificationEnabled, ":memory:"),
		},
	}
	calls := make(map[string]int)

	server, err := NewCentianServerWithOptions(globalConfig, CentianServerOptions{
		LoggerFactory: func(projectSlug string) (*logging.Logger, error) {
			calls[projectSlug]++
			return logging.NewDiscardLogger()
		},
	})
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})

	assert.Equal(t, calls[config.DefaultProjectSlug], 1)
	assert.Equal(t, calls["research"], 1)
	assert.Assert(t, server.Projects[config.DefaultProjectSlug].Logger != nil)
	assert.Assert(t, server.Projects["research"].Logger != nil)
	assert.Equal(t, server.Projects[config.DefaultProjectSlug].Logger.GetLogPath(), "")
	assert.Equal(t, server.Projects["research"].Logger.GetLogPath(), "")
}

func TestCentianServerSetup_ProtectsAPIRoutesButLeavesUIReachable(t *testing.T) {
	authEnabled := true
	uiEnabled := true
	t.Setenv("HOME", t.TempDir())
	entry, err := auth.NewAPIKeyEntry(auth.GeneratedKey{CredID: "key_test", Secret: "secret"})
	assert.NilError(t, err)
	defaultPath, err := auth.DefaultAPIKeysPath()
	assert.NilError(t, err)
	assert.NilError(t, auth.WriteAPIKeyFile(defaultPath, &auth.APIKeyFile{Keys: []auth.APIKeyEntry{entry}}))
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authEnabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
			Capabilities: &config.CapabilitiesSettings{
				UI: &config.UICapabilitySettings{
					Enabled: &uiEnabled,
				},
			},
		},
		Gateways: map[string]*config.GatewayConfig{},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	err = server.Setup()
	assert.NilError(t, err)

	apiReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/task-runs", http.NoBody)
	apiRec := httptest.NewRecorder()
	server.Mux.ServeHTTP(apiRec, apiReq)
	assert.Equal(t, apiRec.Code, http.StatusUnauthorized)
	assert.Equal(t, apiRec.Header().Get(unauthorizedAuthHeaderHint), config.DefaultAuthHeader)

	uiReq := httptest.NewRequest(http.MethodGet, "http://example.com/ui", http.NoBody)
	uiRec := httptest.NewRecorder()
	server.Mux.ServeHTTP(uiRec, uiReq)
	assert.Equal(t, uiRec.Code, http.StatusOK)
}

func TestInitEventProcessor_GatewayProcessorsAppliedToAggregatedEndpoint(t *testing.T) {
	// Given: a server and a gateway config with one gateway-level processor
	t.Setenv("HOME", t.TempDir())
	server := &CentianServer{
		Config: &config.GlobalConfig{},
	}
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{"server1": {Command: "node"}},
		Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
	}
	endpoint := NewAggregatedEndpoint("gateway", "/mcp/gateway", gatewayConfig)
	endpoint.server = server

	// When: the event processor is initialized
	err := endpoint.initEventProcessor()

	// Then: the aggregated endpoint has the gateway processor
	assert.NilError(t, err)
	pc := endpoint.eventProcessor.(*ProcessingController)
	assert.Equal(t, 1, len(pc.processors))
}

func TestInitEventProcessor_GatewayProcessorsAppliedToSingleServerEndpoint(t *testing.T) {
	// Given: a server and a gateway config with one gateway-level processor
	t.Setenv("HOME", t.TempDir())
	server := &CentianServer{
		Config: &config.GlobalConfig{},
	}
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{"server1": {Command: "node"}},
		Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
	}
	endpoint := NewSingleEndpoint("server1", "/mcp/gateway/server1", gatewayConfig)
	endpoint.server = server

	// When: the event processor is initialized
	err := endpoint.initEventProcessor()

	// Then: the single-server endpoint also has the gateway processor
	assert.NilError(t, err)
	pc := endpoint.eventProcessor.(*ProcessingController)
	assert.Equal(t, 1, len(pc.processors))
}

func TestInitEventProcessor_GlobalProcessorsBeforeGatewayProcessors(t *testing.T) {
	// Given: a server with global processors and a gateway with its own processors
	t.Setenv("HOME", t.TempDir())
	server := &CentianServer{
		Config: &config.GlobalConfig{
			Processors: []*config.ProcessorConfig{testEchoProcessorConfig("global-proc")},
		},
	}
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{"server1": {Command: "node"}},
		Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
	}
	endpoint := NewSingleEndpoint("server1", "/mcp/gateway/server1", gatewayConfig)
	endpoint.server = server

	// When: the event processor is initialized
	err := endpoint.initEventProcessor()

	// Then: processors are ordered global-first, then gateway
	assert.NilError(t, err)
	pc := endpoint.eventProcessor.(*ProcessingController)
	assert.Equal(t, 2, len(pc.processors))
	assert.Equal(t, "global-proc", pc.processors[0].GetConfig().Name)
	assert.Equal(t, "gateway-proc", pc.processors[1].GetConfig().Name)
}

func TestSetup_ProcessorsInitializedForBothEndpointTypes(t *testing.T) {
	// Given: a global config with gateway-level processors on one of its gateways
	t.Setenv("HOME", t.TempDir())
	authDisabled := false
	enabled := true
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9001",
			Timeout: 10,
		},
		Gateways: map[string]*config.GatewayConfig{
			"gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"server1": {Command: "node", Enabled: &enabled},
				},
				Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
			},
		},
	}

	centianServer, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)

	// When: setting up the proxy
	err = centianServer.Setup()
	assert.NilError(t, err)

	// Then: the aggregated gateway endpoint has gateway processors initialized
	aggregatedEndpoint := centianServer.Gateways["gateway"]
	assert.Assert(t, aggregatedEndpoint != nil)
	aggregatedPC := aggregatedEndpoint.eventProcessor.(*ProcessingController)
	assert.Equal(t, 1, len(aggregatedPC.processors))

	// Then: the single-server endpoint also has the gateway processors initialized
	singleReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/server1", http.NoBody)
	singleHandler, singlePattern := centianServer.Mux.Handler(singleReq)
	assert.Assert(t, singleHandler != nil)
	assert.Equal(t, "/mcp/gateway/server1", singlePattern)
}

func TestCentianServerClose_ClosesEndpointPools(t *testing.T) {
	conn := &MockDownstreamConnection{serverName: "server-a", Status: StatusConnected}
	endpoint := &CentianEndpoint{
		downstreamPools: map[string]*DownstreamSessionPool{
			"pool-1": {
				downstreamConns: map[string]DownstreamConnectionInterface{
					"server-a": conn,
				},
				connecting: make(map[string]bool),
			},
		},
	}
	server := &CentianServer{
		Endpoints: []*CentianEndpoint{nil, endpoint},
	}

	errs := server.Close()

	assert.Assert(t, len(errs) == 0)
	assert.Equal(t, conn.CloseCalls, 1)
	assert.Equal(t, len(endpoint.downstreamPools), 0)
}

func TestNewCentianServer_DefaultEventStorageCreatesSQLiteStore(t *testing.T) {
	authDisabled := false
	logDir := t.TempDir()
	t.Setenv("CENTIAN_LOG_DIR", logDir)
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Gateways: map[string]*config.GatewayConfig{},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})

	defaultPath, err := logging.GetDefaultEventStorePath()
	assert.NilError(t, err)
	info, err := os.Stat(defaultPath)
	assert.NilError(t, err)
	assert.Assert(t, !info.IsDir())
	assert.Equal(t, filepath.Dir(defaultPath), logDir)
	assert.Assert(t, server.TaskVerification.EventStore != nil)
	assert.Assert(t, server.TaskVerification.RunStore == server.PersistenceStore)
	assert.Assert(t, server.PersistenceStore != nil)
	assert.Equal(t, server.TaskVerification.TemplateDir, filepath.Join(mustGetwd(t), "task-templates"))
}

func TestNewCentianServer_UsesEmbeddedTaskTemplatesWhenDirectoryMissing(t *testing.T) {
	authDisabled := false
	logDir := t.TempDir()
	workDir := t.TempDir()
	originalWD := mustGetwd(t)
	assert.NilError(t, os.Chdir(workDir))
	t.Cleanup(func() {
		assert.NilError(t, os.Chdir(originalWD))
	})
	t.Setenv("CENTIAN_LOG_DIR", logDir)

	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
		Gateways: map[string]*config.GatewayConfig{},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})

	summaries, err := server.TaskVerification.ListTemplates()
	assert.NilError(t, err)
	assert.Equal(t, server.TaskVerification.TemplateDir, filepath.Join(mustGetwd(t), "task-templates"))
	assert.Equal(t, len(summaries), 3)
	assert.Equal(t, summaries[0].ID, "guided_tdd_workflow")
	assert.Equal(t, summaries[1].ID, "minimal")
	assert.Equal(t, summaries[2].ID, "simple_tdd")
}

func TestCentianServerSetup_OmitsAPIRoutesWhenEventStorageDisabled(t *testing.T) {
	// Given: a config with event storage disabled
	authDisabled := false
	eventStorageEnabled := false
	uiEnabled := true
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
			Capabilities: &config.CapabilitiesSettings{
				EventStorage: &config.EventStorageCapabilitySettings{
					Enabled: &eventStorageEnabled,
				},
				UI: &config.UICapabilitySettings{
					Enabled: &uiEnabled,
				},
			},
		},
		Gateways: map[string]*config.GatewayConfig{},
	}
	t.Setenv("HOME", t.TempDir())

	proxy, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)

	// When: setting up the proxy
	err = proxy.Setup()
	assert.NilError(t, err)

	// Then: the task run API routes are not registered
	apiListReq, _ := http.NewRequest(http.MethodGet, "http://example.com/api/task-runs", http.NoBody)
	_, apiListPattern := proxy.Mux.Handler(apiListReq)
	assert.Equal(t, apiListPattern, "")

	validRunID := "tr_1742947200123_0000000001"
	apiEventsReq, _ := http.NewRequest(http.MethodGet, "http://example.com/api/task-runs/"+validRunID+"/events", http.NoBody)
	_, apiEventsPattern := proxy.Mux.Handler(apiEventsReq)
	assert.Equal(t, apiEventsPattern, "")

	// Then: the embedded UI is not registered without persistence backing it
	uiIndexReq, _ := http.NewRequest(http.MethodGet, "http://example.com/ui", http.NoBody)
	_, uiIndexPattern := proxy.Mux.Handler(uiIndexReq)
	assert.Equal(t, uiIndexPattern, "")
}

func TestCentianServerSetup_OmitsUIRoutesWhenUIDisabled(t *testing.T) {
	authDisabled := false
	uiEnabled := false
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
			Capabilities: &config.CapabilitiesSettings{
				UI: &config.UICapabilitySettings{
					Enabled: &uiEnabled,
				},
			},
		},
		Gateways: map[string]*config.GatewayConfig{},
	}
	t.Setenv("HOME", t.TempDir())

	proxy, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)

	err = proxy.Setup()
	assert.NilError(t, err)

	apiListReq, _ := http.NewRequest(http.MethodGet, "http://example.com/api/task-runs", http.NoBody)
	apiListHandler, apiListPattern := proxy.Mux.Handler(apiListReq)
	assert.Assert(t, apiListHandler != nil)
	assert.Equal(t, apiListPattern, "GET /api/task-runs")

	uiIndexReq, _ := http.NewRequest(http.MethodGet, "http://example.com/ui", http.NoBody)
	_, uiIndexPattern := proxy.Mux.Handler(uiIndexReq)
	assert.Equal(t, uiIndexPattern, "")
}

func TestNewCentianServer_UsesConfiguredTaskTemplatesPath(t *testing.T) {
	authDisabled := false
	t.Setenv("HOME", t.TempDir())
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
			Capabilities: &config.CapabilitiesSettings{
				TaskVerification: &config.TaskVerificationCapabilitySettings{
					TemplatesPath: "custom-templates",
				},
			},
		},
		Gateways: map[string]*config.GatewayConfig{},
	}

	server, err := NewCentianServer(globalConfig)
	assert.NilError(t, err)
	t.Cleanup(func() {
		for _, closeErr := range server.Close() {
			assert.NilError(t, closeErr)
		}
	})

	assert.Equal(t, server.TaskVerification.TemplateDir, filepath.Join(mustGetwd(t), "custom-templates"))
}

func mustGetwd(t *testing.T) string {
	t.Helper()

	workingDir, err := os.Getwd()
	assert.NilError(t, err)
	return workingDir
}
