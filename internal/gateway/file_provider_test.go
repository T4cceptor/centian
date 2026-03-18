package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/gateway"
	"gotest.tools/assert"
)

func makeServerConfig(path string) *config.ServerConfig {
	return &config.ServerConfig{
		Version: "1.0.0",
		GatewayProvider: &config.GatewayProviderConfig{
			Type: "file",
			Path: path,
		},
	}
}

func TestFileGatewayProvider_LoadsValidFile(t *testing.T) {
	// Given: a temp directory with a valid gateways.json
	dir := t.TempDir()
	path := filepath.Join(dir, "gateways.json")
	gf := &config.GatewayFile{
		Version: "1.0.0",
		Gateways: map[string]*config.GatewayConfig{
			"mygw": {
				MCPServers: map[string]*config.MCPServerConfig{
					"srv": {Command: "node"},
				},
			},
		},
	}
	assert.NilError(t, config.SaveGatewayFileToPath(path, gf))
	provider, err := gateway.NewFileGatewayProvider(makeServerConfig(path))
	assert.NilError(t, err)

	// When: loading the gateway file
	loaded, err := provider.LoadGatewayFile()

	// Then: gateways are returned without error
	assert.NilError(t, err)
	assert.Equal(t, len(loaded.Gateways), 1)
	_, ok := loaded.Gateways["mygw"]
	assert.Assert(t, ok)
}

func TestFileGatewayProvider_ReturnsErrorWhenFileMissing(t *testing.T) {
	// Given: a provider pointing to a non-existent file
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	provider, err := gateway.NewFileGatewayProvider(makeServerConfig(path))
	assert.NilError(t, err)

	// When: loading the gateway file
	_, err = provider.LoadGatewayFile()

	// Then: error is returned
	assert.ErrorContains(t, err, "gateway file not found")
}

func TestFileGatewayProvider_ReturnsErrorOnInvalidJSON(t *testing.T) {
	// Given: a file with malformed JSON
	dir := t.TempDir()
	path := filepath.Join(dir, "gateways.json")
	assert.NilError(t, os.WriteFile(path, []byte("{not valid json"), 0o644))
	provider, err := gateway.NewFileGatewayProvider(makeServerConfig(path))
	assert.NilError(t, err)

	// When: loading the gateway file
	_, err = provider.LoadGatewayFile()

	// Then: parse error is returned
	assert.ErrorContains(t, err, "failed to parse gateway file")
}

func TestFileGatewayProvider_SaveAndReload_RoundTrip(t *testing.T) {
	// Given: a provider and a gateway file
	dir := t.TempDir()
	path := filepath.Join(dir, "gateways.json")
	provider, err := gateway.NewFileGatewayProvider(makeServerConfig(path))
	assert.NilError(t, err)
	original := &config.GatewayFile{
		Version: "1.0.0",
		Gateways: map[string]*config.GatewayConfig{
			"gw1": {
				MCPServers: map[string]*config.MCPServerConfig{
					"s1": {Command: "python3"},
				},
			},
		},
	}

	// When: saving then reloading
	assert.NilError(t, provider.SaveGatewayFile(original))
	loaded, err := provider.LoadGatewayFile()

	// Then: reloaded file matches original
	assert.NilError(t, err)
	assert.Equal(t, loaded.Version, original.Version)
	assert.Equal(t, len(loaded.Gateways), 1)
	srv, ok := loaded.Gateways["gw1"].MCPServers["s1"]
	assert.Assert(t, ok)
	assert.Equal(t, srv.Command, "python3")
}

func TestFileGatewayProvider_DefaultPath(t *testing.T) {
	// Given: a server config with no path set
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	provider, err := gateway.NewFileGatewayProvider(&config.ServerConfig{
		Version: "1.0.0",
		GatewayProvider: &config.GatewayProviderConfig{
			Type: "file",
			Path: "", // empty → default
		},
	})
	assert.NilError(t, err)

	// When: loading (file doesn't exist yet)
	_, err = provider.LoadGatewayFile()

	// Then: the error mentions the default path under HOME/.centian
	assert.ErrorContains(t, err, dir)
}
