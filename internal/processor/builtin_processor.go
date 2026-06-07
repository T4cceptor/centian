package processor

import (
	"fmt"

	"github.com/T4cceptor/centian/internal/config"
	patternredactionprocessor "github.com/T4cceptor/centian/internal/processor/pattern_redaction_processor"
	piiredactor "github.com/T4cceptor/centian/internal/processor/pii_redactor"
	promptinjectionguard "github.com/T4cceptor/centian/internal/processor/prompt_injection_guard"
	secrettokenredactor "github.com/T4cceptor/centian/internal/processor/secret_token_redactor"
	toolcallguard "github.com/T4cceptor/centian/internal/processor/tool_call_guard"
)

// BuiltinProcessor runs a processor implementation compiled into Centian.
type BuiltinProcessor struct {
	config *config.ProcessorConfig
}

// NewBuiltinProcessor creates a built-in processor from config.
func NewBuiltinProcessor(c *config.ProcessorConfig) (*BuiltinProcessor, error) {
	if _, err := config.ParseBuiltinProcessorSettings(c); err != nil {
		return nil, err
	}
	return &BuiltinProcessor{config: c}, nil
}

// GetConfig returns the attached ProcessorConfig.
func (b *BuiltinProcessor) GetConfig() *config.ProcessorConfig {
	return b.config
}

// Process runs the configured built-in processor against the input context.
func (b *BuiltinProcessor) Process(input *DataContext) (*DataContext, error) {
	settings, err := config.ParseBuiltinProcessorSettings(b.config)
	if err != nil {
		return nil, err
	}

	inputJSON, err := marshalProcessorInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processor input: %w", err)
	}

	var outputJSON []byte
	switch settings.Processor {
	case config.BuiltinPromptInjectionGuard:
		outputJSON, err = promptinjectionguard.ProcessJSON(inputJSON, settings.Mode)
	case config.BuiltinPatternRedactionProcessor:
		outputJSON, err = patternredactionprocessor.ProcessJSON(inputJSON, settings)
	case config.BuiltinSecretTokenRedactor:
		outputJSON, err = secrettokenredactor.ProcessJSON(inputJSON, settings)
	case config.BuiltinPIIRedactor:
		outputJSON, err = piiredactor.ProcessJSON(inputJSON, settings)
	case config.BuiltinToolCallGuard:
		outputJSON, err = toolcallguard.ProcessJSON(inputJSON, settings)
	default:
		return nil, fmt.Errorf("processor '%s': unsupported builtin processor %q", b.config.Name, settings.Processor)
	}
	if err != nil {
		return nil, err
	}

	output, err := decodeProcessorJSONOutput(b.config.Name, outputJSON)
	if err != nil {
		return nil, err
	}
	return output, nil
}
