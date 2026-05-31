package processor

import (
	"errors"
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
)

// ProcessorInterface interfaces all Processor implementations.
//
//nolint:revive // Ignoring idea to calling this "Interface"
type ProcessorInterface interface {
	// Process takes an input and performs the configured action
	// - e.g. calling a CLI command, executing a webhook, etc.
	//
	// error indicates a failure to process the input
	// - it's not to be confused with an error status of
	// the payload (e.g. by evaluation processors).
	Process(input *DataContext) (*DataContext, error)

	// GetConfig returns the processors config
	GetConfig() *config.ProcessorConfig
}

var (
	_ ProcessorInterface = (*CLIProcessor)(nil)
	_ ProcessorInterface = (*WebhookProcessor)(nil)
	_ ProcessorInterface = (*BuiltinProcessor)(nil)
)

// ErrProcessorDisabled indicates processor creation was requested for a disabled processor config.
var ErrProcessorDisabled = errors.New("processor is disabled")

// NewProcessor creates a new processor using the provided ProcessorConfig.
func NewProcessor(processorConfig *config.ProcessorConfig) (ProcessorInterface, error) {
	// Validate processor is enabled.
	if !processorConfig.Enabled {
		return nil, ErrProcessorDisabled
	}

	switch processorConfig.Type {
	case string(config.CLIProcessor):
		processor, err := NewCLIProcessor(processorConfig)
		return processor, err
	case string(config.WebhookProcessor):
		processor, err := NewWebhookProcessor(processorConfig)
		return processor, err
	case string(config.BuiltinProcessor):
		processor, err := NewBuiltinProcessor(processorConfig)
		return processor, err
	}
	return nil, fmt.Errorf("unsupported processor type '%s'", processorConfig.Type)
}
