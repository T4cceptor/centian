package processor

import (
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestNewProcessor(t *testing.T) {
	t.Run("returns error when processor is disabled", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "disabled-proc",
			Type:    string(config.CLIProcessor),
			Enabled: false,
		}

		processor, err := NewProcessor(cfg)

		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "is disabled"))
		assert.Assert(t, processor == nil)
	})

	t.Run("returns error for unsupported processor type", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "invalid-proc",
			Type:    "invalid",
			Enabled: true,
		}

		processor, err := NewProcessor(cfg)

		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "unsupported processor type"))
		assert.Assert(t, processor == nil)
	})

	t.Run("creates webhook processor when enabled and type is webhook", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "webhook-proc",
			Type:    string(config.WebhookProcessor),
			Enabled: true,
			Config: map[string]interface{}{
				"url": "http://example.com/processor",
			},
		}

		processor, err := NewProcessor(cfg)

		assert.NilError(t, err)
		assert.Assert(t, processor != nil)
		_, ok := processor.(*WebhookProcessor)
		assert.Assert(t, ok, "expected *WebhookProcessor")
		assert.Equal(t, processor.GetConfig(), cfg)
	})

	t.Run("creates CLI processor when enabled and type is cli", func(t *testing.T) {
		cfg := &config.ProcessorConfig{
			Name:    "cli-proc",
			Type:    string(config.CLIProcessor),
			Enabled: true,
			Config: map[string]interface{}{
				"command": "echo",
			},
		}

		processor, err := NewProcessor(cfg)

		assert.NilError(t, err)
		assert.Assert(t, processor != nil)
		_, ok := processor.(*CLIProcessor)
		assert.Assert(t, ok, "expected *CLIProcessor")
		assert.Equal(t, processor.GetConfig(), cfg)
	})
}
