package processor

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
)

type ProcessorInterface interface {
	// Process takes an input and performs the configured action
	// - e.g. calling a CLI command, executing a webhook, etc.
	//
	// error indicates a failure to process the input.
	Process(input map[string]any) (map[string]any, error)

	// GetConfig returns the processors config
	GetConfig() *config.ProcessorConfig
}

var (
	_ ProcessorInterface = (*CLIProcessor)(nil)
)

// NewProcessor creates a new processor using the provided ProcessorConfig
//
// Currently, it only supports "cli" processors
func NewProcessor(processorConfig *config.ProcessorConfig) (ProcessorInterface, error) {
	// Validate processor is enabled.
	if !processorConfig.Enabled {
		return nil, fmt.Errorf("processor '%s' is disabled", processorConfig.Name)
	}

	// Only CLI processors supported in v1.
	// TODO: once the list of processors expands we might want to refactor this into a
	// map[string]func or some other more suitable structure
	if processorConfig.Type == "cli" {
		processor, err := NewCLIProcessor(processorConfig)
		return processor, err
	}
	return nil, fmt.Errorf("unsupported processor type '%s'", processorConfig.Type)
}
