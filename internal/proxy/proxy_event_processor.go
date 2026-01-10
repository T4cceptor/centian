package proxy

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/CentianAI/centian-cli/internal/common"
	"github.com/CentianAI/centian-cli/internal/logging"
	"github.com/CentianAI/centian-cli/internal/processor"
)

// EventProcessor is used to call the main processing loop for any MCP transport method
type EventProcessor struct {
	logger              *logging.Logger
	processorChain      *processor.Chain
	logBeforeProcessing bool
	logAfterProcessing  bool
}

// NewEventProcessor returns a new EventProcessor with the provided logger and processors
func NewEventProcessor(logger *logging.Logger, processors *processor.Chain) *EventProcessor {
	return &EventProcessor{
		logger:              logger,
		processorChain:      processors,
		logBeforeProcessing: true,
		logAfterProcessing:  true,
	}
}

// Process starts the main event loop processing, including logging and any configured processors
func (ep *EventProcessor) Process(event common.McpEventInterface) error {
	// Log before processing
	if ep.logBeforeProcessing {
		if err := ep.logger.LogMcpEvent(event); err != nil {
			common.LogError(err.Error())
			event.GetBaseEvent().ProcessingErrors["processor_log_error"] = err
		}
	}

	// Apply processors in order (only if there are actually processors configured)
	if ep.processorChain != nil && ep.processorChain.HasProcessors() && event.HasContent() {
		outputLine := event.RawMessage()
		result, err := ep.processorChain.Execute(event)

		// TODO: standardize those logs
		switch {
		case err != nil:
			// Failed to execute processor chain
			fmt.Fprintf(os.Stderr, "[PROCESSOR-ERROR] Failed to execute response processors: %v\n", err)
			// Fall through and forward original response
		case result.Status >= 400:
			// Processor rejected or errored - send MCP error to client
			fmt.Fprintf(os.Stderr, "[PROCESSOR-REJECT] Response rejected with status %d\n", result.Status)

			// Extract request ID from original message
			var msgData map[string]interface{}
			if err := json.Unmarshal([]byte(outputLine), &msgData); err != nil {
				fmt.Fprintf(os.Stderr, "[PROCESSOR-ERROR] Failed to parse response JSON for error response: %v\n", err)
				// Fall through and forward original response
			} else {
				// Format MCP error response
				errorResponse, err := processor.FormatMCPError(result, msgData["id"])
				if err != nil {
					fmt.Fprintf(os.Stderr, "[PROCESSOR-ERROR] Failed to format MCP error: %v\n", err)
					// Fall through and forward original response
				} else {
					// Send error response to client instead of original response
					outputLine = errorResponse
				}
			}
		default:
			// Status 200 - processor passed, use modified payload
			modifiedJSON, err := json.Marshal(result.ModifiedPayload)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[PROCESSOR-ERROR] Failed to marshal modified response: %v\n", err)
				// Fall through and forward original response
			} else {
				outputLine = string(modifiedJSON)
				fmt.Fprintf(os.Stderr, "[PROCESSOR-MODIFIED] Response modified by processors\n")
			}
		}

		// TODO: result should also dictate what happens with the
		// event -> are we returning it or are the forwarding it ?!
		// We likely need a field indicating how to proceed with the event!
		if outputLine != event.RawMessage() {
			event.SetModified(true)
		}
		event.SetRawMessage(outputLine)
	}

	// Log after processing
	if ep.logAfterProcessing {
		if err := ep.logger.LogMcpEvent(event); err != nil {
			common.LogError(err.Error())
			event.GetBaseEvent().ProcessingErrors["processor_log_error"] = err
		}
	}
	return nil
}
