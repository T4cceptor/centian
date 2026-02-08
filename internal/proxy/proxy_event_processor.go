package proxy

import (
	"fmt"
	"os"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/processor"
)

// ProcessingController is used to call the main processing loop for any MCP transport method.
type ProcessingController struct {
	logger              *logging.Logger
	processors          []processor.ProcessorInterface
	logBeforeProcessing bool
	logAfterProcessing  bool
}

// TODO: checkme!
/*
- processors should have an internal state -> this means they should replace EventProcessor in proxy_server - see line 600
- then in ProcessCall we just iterate all Processors and call "Process" one by one
	- the question here is: where does the logging take place, this is currently done in the "Process" method below
	- maybe a ProcessController would be good -> this replaces "EventProcessor"

*/

// NewEventProcessor returns a new EventProcessor with the provided logger and processors.
func NewEventProcessor(processorConfigs []*config.ProcessorConfig) *ProcessingController {
	result := &ProcessingController{
		processors:          make([]processor.ProcessorInterface, 0),
		logBeforeProcessing: true, // TODO: make configurable
		logAfterProcessing:  true, // TODO: make configurable
	}
	for _, config := range processorConfigs {
		processor, err := processor.NewProcessor(config)
		if err != nil {
			// TODO: log
			// TODO: need to double check if processor was mandatory, if so we abort
			continue
		}
		result.processors = append(result.processors, processor)
	}
	return result
}

// GetInput uses the provided ProcessorConfig and CallContext to create the input map for the processor
func GetInput(processorConfig *config.ProcessorConfig, callCtx CallContext) map[string]any {
	input := make(map[string]any)
	for _, part := range processorConfig.GetParts() {
		handler, ok := callCtx.GetHandler(part) // TODO: we could have a "GetParts"
		if ok {
			input[part] = handler.Get(callCtx)
		}
	}
	return input
}

func GetResultPart(result map[string]any, part string) (any, error) {
	res, ok := result[part]
	if !ok || res == nil {
		return nil, fmt.Errorf("unable to retrieve part '%s' from result (%v)", part, result)
	}
	return res, nil
}

// ApplyResult takes the result from the processor and processes it in order for each configured part of the ProcessorConfig
func ApplyResult(processorConfig *config.ProcessorConfig, result map[string]any, callCtx CallContext) error {
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

// Process runs all processors on the CallContext using handlers to build input and apply results.
func (ep *ProcessingController) Process(callCtx CallContext) error {
	// Log before processing
	if ep.logBeforeProcessing {
		if err := callCtx.GetLogHandler().Log(callCtx); err != nil {
			fmt.Fprintf(os.Stderr, "[LOG-ERROR] %v\n", err)
			// TODO: double check if we need to do something here
		}
	}

	// Process through each processor
	if ep.processors != nil && len(ep.processors) > 0 {
		for _, processor := range ep.processors {
			processorConfig := processor.GetConfig()
			// 1. Build input from handlers based on processor's configured parts
			input := GetInput(processorConfig, callCtx)

			// 2. Execute processor
			output, err := processor.Process(input)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[PROCESSOR-ERROR] %s: %v\n", processorConfig.Name, err)
				return err
			}

			// 3. Apply results back via handlers
			if err := ApplyResult(processorConfig, output, callCtx); err != nil {
				fmt.Fprintf(os.Stderr, "[PROCESSOR-APPLY-ERROR] %s: %v\n", processorConfig.Name, err)
				return err // TODO: double check if this makes sense!
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
	}

	// Log after processing
	if ep.logAfterProcessing {
		if err := callCtx.GetLogHandler().Log(callCtx); err != nil {
			fmt.Fprintf(os.Stderr, "[LOG-ERROR] %v\n", err)
			// TODO: double check if we need to do something here
		}
	}

	return nil
}
