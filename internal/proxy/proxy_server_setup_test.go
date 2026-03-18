package proxy

import (
	"net/http"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

// mockGatewayProvider implements gateway.GatewayProvider for tests.
type mockGatewayProvider struct {
	file *config.GatewayFile
}

func (m *mockGatewayProvider) LoadGatewayFile() (*config.GatewayFile, error) {
	return m.file, nil
}

func (m *mockGatewayProvider) SaveGatewayFile(f *config.GatewayFile) error {
	m.file = f
	return nil
}

func newMockProvider(gf *config.GatewayFile) *mockGatewayProvider {
	return &mockGatewayProvider{file: gf}
}

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

func TestCentianServerSetup_RegistersHandlers(t *testing.T) {
	// Given: a server config with auth disabled and a gateway file
	authDisabled := false
	enabled := true
	disabled := false
	serverConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
		},
	}
	gf := &config.GatewayFile{
		Version: "1.0.0",
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

	proxy, err := NewCentianServer(serverConfig, newMockProvider(gf))
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
		Config: &config.GlobalConfig{},
	}
	gatewayConfig := &config.GatewayConfig{
		MCPServers: map[string]*config.MCPServerConfig{"server1": {Command: "node"}},
		Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
	}
	endpoint := NewSingleEndpoint("server1", "/mcp/gateway/server1", gatewayConfig)
	endpoint.server = server
	// Simulate what buildGatewayMux does: populate endpoint's globalProcessors field.
	endpoint.globalProcessors = []*config.ProcessorConfig{testEchoProcessorConfig("global-proc")}

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
	// Given: a server config with gateway-level processors on one of its gateways
	t.Setenv("HOME", t.TempDir())
	authDisabled := false
	enabled := true
	serverConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9001",
			Timeout: 10,
		},
	}
	gf := &config.GatewayFile{
		Version: "1.0.0",
		Gateways: map[string]*config.GatewayConfig{
			"gateway": {
				MCPServers: map[string]*config.MCPServerConfig{
					"server1": {Command: "node", Enabled: &enabled},
				},
				Processors: []*config.ProcessorConfig{testEchoProcessorConfig("gateway-proc")},
			},
		},
	}

	centianServer, err := NewCentianServer(serverConfig, newMockProvider(gf))
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
