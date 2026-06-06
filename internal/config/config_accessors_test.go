package config

import (
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
)

func TestProcessorConfigGetParts(t *testing.T) {
	t.Run("returns default payload part when empty", func(t *testing.T) {
		cfg := &ProcessorConfig{}
		assert.DeepEqual(t, cfg.GetParts(), []string{"payload", "meta"})
	})

	t.Run("returns configured parts when provided", func(t *testing.T) {
		cfg := &ProcessorConfig{Parts: []string{"payload", "meta", "routing", "annotations"}}
		assert.DeepEqual(t, cfg.GetParts(), []string{"payload", "meta", "routing", "annotations"})
	})
}

func TestMCPServerConfigGetTransport(t *testing.T) {
	t.Run("returns http transport when only url is set", func(t *testing.T) {
		cfg := &MCPServerConfig{URL: "https://example.com/mcp"}
		assert.Equal(t, cfg.GetTransport(), common.HTTPTransport)
	})

	t.Run("returns stdio transport when only command is set", func(t *testing.T) {
		cfg := &MCPServerConfig{Command: "python3"}
		assert.Equal(t, cfg.GetTransport(), common.StdioTransport)
	})

	t.Run("returns invalid transport when both url and command are set", func(t *testing.T) {
		cfg := &MCPServerConfig{URL: "https://example.com/mcp", Command: "python3"}
		assert.Equal(t, cfg.GetTransport(), common.InvalidTransport)
	})

	t.Run("returns unknown transport when both are empty", func(t *testing.T) {
		cfg := &MCPServerConfig{}
		assert.Equal(t, cfg.GetTransport(), common.UnknownTransport)
	})
}

func TestGetAuthBackend(t *testing.T) {
	t.Run("returns empty values when unset", func(t *testing.T) {
		// Given: a config without an auth backend block
		cfg := &GlobalConfig{}
		// When: reading the backend
		bt, store := cfg.GetAuthBackend()
		// Then: both are empty (callers resolve defaults)
		assert.Equal(t, bt, "")
		assert.Equal(t, store, "")
	})

	t.Run("returns configured values", func(t *testing.T) {
		// Given: a config with an explicit auth backend block
		cfg := &GlobalConfig{AuthBackend: &AuthBackendSettings{Type: "file", Store: "/tmp/keys.json"}}
		// When: reading the backend
		bt, store := cfg.GetAuthBackend()
		// Then: the configured values are returned verbatim
		assert.Equal(t, bt, "file")
		assert.Equal(t, store, "/tmp/keys.json")
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		// Given: a nil config
		var cfg *GlobalConfig
		// When/Then: reading the backend does not panic and yields empties
		bt, store := cfg.GetAuthBackend()
		assert.Equal(t, bt, "")
		assert.Equal(t, store, "")
	})
}
