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
)

var DisabledProcessor = errors.New("processor is disabled")

// NewProcessor creates a new processor using the provided ProcessorConfig
//
// Currently, it only supports "cli" processors and returns an error if Type != "cli".
func NewProcessor(processorConfig *config.ProcessorConfig) (ProcessorInterface, error) {
	// Validate processor is enabled.
	if !processorConfig.Enabled {
		return nil, DisabledProcessor
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
