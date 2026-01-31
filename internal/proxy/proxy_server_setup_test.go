package proxy

import (
	"net/http"
	"testing"

	"github.com/CentianAI/centian-cli/internal/config"
	"gotest.tools/assert"
)

func TestCentianProxySetup_RegistersHandlers(t *testing.T) {
	// Given: a global config with auth disabled and a gateway
	authDisabled := false
	enabled := true
	disabled := false
	globalConfig := &config.GlobalConfig{
		Name:        "Test",
		Version:     "1.0.0",
		AuthEnabled: &authDisabled,
		Proxy: &config.ProxySettings{
			Port:    "9000",
			Timeout: 10,
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

	proxy, err := NewCentianProxy(globalConfig)
	assert.NilError(t, err)

	// When: setting up the proxy
	err = proxy.Setup()

	// Then: aggregated and single endpoints are registered
	assert.NilError(t, err)

	aggregatedReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway", nil)
	aggregatedHandler, aggregatedPattern := proxy.Mux.Handler(aggregatedReq)
	assert.Assert(t, aggregatedHandler != nil)
	assert.Equal(t, aggregatedPattern, "/mcp/gateway")

	singleReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/enabled", nil)
	singleHandler, singlePattern := proxy.Mux.Handler(singleReq)
	assert.Assert(t, singleHandler != nil)
	assert.Equal(t, singlePattern, "/mcp/gateway/enabled")

	// Then: disabled endpoint is not registered
	disabledReq, _ := http.NewRequest(http.MethodPost, "http://example.com/mcp/gateway/disabled", nil)
	_, disabledPattern := proxy.Mux.Handler(disabledReq)
	assert.Equal(t, disabledPattern, "")
}
