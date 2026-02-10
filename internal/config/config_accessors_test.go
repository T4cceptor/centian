package config

import (
	"testing"

	"github.com/T4cceptor/centian/internal/common"
	"gotest.tools/assert"
)

func TestProcessorConfigGetParts(t *testing.T) {
	t.Run("returns default payload part when empty", func(t *testing.T) {
		cfg := &ProcessorConfig{}
		assert.DeepEqual(t, cfg.GetParts(), []string{"payload"})
	})

	t.Run("returns configured parts when provided", func(t *testing.T) {
		cfg := &ProcessorConfig{Parts: []string{"payload", "meta", "routing"}}
		assert.DeepEqual(t, cfg.GetParts(), []string{"payload", "meta", "routing"})
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
