package proxy

import (
	"errors"
	"fmt"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/processor"
)

// ProcessingController is used to call the main processing loop for any MCP transport method.
type ProcessingController struct {
	processors []processor.ProcessorInterface
}

// NewProcessingController returns a new ProcessingController using the provided processor configs.
func NewProcessingController(processorConfigs []*config.ProcessorConfig) (*ProcessingController, error) {
	result := &ProcessingController{
		processors: make([]processor.ProcessorInterface, 0),
	}
	for _, config := range processorConfigs {
		if config.IsToolSurfaceProcessor() {
			continue
		}
		p, err := processor.NewProcessor(config)
		if err == nil {
			result.processors = append(result.processors, p)
			continue
		}
		if errors.Is(err, processor.ErrProcessorDisabled) {
			if config.Required {
				return nil, fmt.Errorf("unable to configure required processor '%s', Error: %w", config.Name, err)
			}
			common.LogInfo("processor '%s' is disabled, skipping", config.Name)
			continue // we do nothing here
		}
		// Error cases
		if config.Required {
			return nil, fmt.Errorf("unable to configure required processor '%s', Error: %w", config.Name, err)
		}
		common.LogWarn("unable to configure processor '%s': %s", config.Name, err.Error())
	}
	return result, nil
}

// GetInput uses the provided ProcessorConfig and CallContext to create the input map for the processor.
func GetInput(processorConfig *config.ProcessorConfig, callCtx CallContext) *processor.DataContext {
	input := &processor.DataContext{Version: processor.CurrentDataContextVersion}
	for _, part := range processorConfig.GetParts() {
		handler, ok := callCtx.GetHandler(part) // TODO: we could have a "GetParts"
		if ok {
			handler.AttachPart(callCtx, input)
		}
	}
	return input
}

// ApplyResult takes the result from the processor and processes
// it in order for each configured part of the ProcessorConfig.
func ApplyResult(processorConfig *config.ProcessorConfig, result *processor.DataContext, callCtx CallContext) error {
	for _, part := range processorConfig.GetParts() {
		handler, ok := callCtx.GetHandler(part)
		if !ok {
			return fmt.Errorf("unable to find handler for part: %s", part)
		}
		// TODO: think about only providing the part for the handler (less flexible)
		// resultPart, err := GetResultPart(result, part)
		// if err != nil {
		// 	return err
		// }
		if err := handler.Apply(callCtx, result); err != nil {
			return err
		}
	}
	return nil
}

func handleProcessorFailure(processorConfig *config.ProcessorConfig, phase string, err error) error {
	common.LogError("processor '%s' failed during %s: %v", processorConfig.Name, phase, err)
	if processorConfig.Required {
		return err
	}
	return nil
}

// Process runs all processors on the CallContext using handlers to build input and apply results.
func (ep *ProcessingController) Process(callCtx CallContext) error {
	// Process through each processor
	for _, processor := range ep.processors {
		processorConfig := processor.GetConfig()
		// 1. Build input from handlers based on processor's configured parts
		input := GetInput(processorConfig, callCtx)

		// 2. Execute processor
		output, err := processor.Process(input)
		if err != nil {
			if err := handleProcessorFailure(processorConfig, "execution", err); err != nil {
				return err
			}
			continue
		}

		// 3. Apply results back via handlers
		if err := ApplyResult(processorConfig, output, callCtx); err != nil {
			if err := handleProcessorFailure(processorConfig, "apply", err); err != nil {
				return err
			}
			continue
		}
		// Note: callCtx might be modified here, the modified version
		// then is also provided to the next processor

		// Further, there is currently no status check here, so even if processor returned
		// an error status subsequent processors are still triggered
		// (Note the difference between error during processing and error status)
		// This can be intentional - e.g.:
		// 1. payload eval processor -> fails
		// 2. logging processor -> logs status, in this case the failure

		// In the future, this might be changed, for example to make it configurable which
		// processors are run in case of an early return or error
	}

	// Log after processing
	if err := callCtx.GetLogHandler().Log(callCtx); err != nil {
		common.LogError("failed to write MCP event log: %v", err)
		// TODO: double check if we need to do something here
	}

	return nil
}
