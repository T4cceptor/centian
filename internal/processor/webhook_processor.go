package processor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/T4cceptor/centian/internal/common"
	"github.com/T4cceptor/centian/internal/config"
)

// WebhookProcessor performs synchronous HTTP POST execution against a remote processor.
type WebhookProcessor struct {
	HTTPClient *http.Client
	config     *config.ProcessorConfig
}

// NewWebhookProcessor creates a new WebhookProcessor.
func NewWebhookProcessor(c *config.ProcessorConfig) (*WebhookProcessor, error) {
	return &WebhookProcessor{
		HTTPClient: http.DefaultClient,
		config:     c,
	}, nil
}

// GetConfig returns the attached ProcessorConfig.
func (w *WebhookProcessor) GetConfig() *config.ProcessorConfig {
	return w.config
}

// Process sends the reduced processor input DTO to the configured webhook and
// decodes the synchronous JSON response into a DataContext.
func (w *WebhookProcessor) Process(input *DataContext) (*DataContext, error) {
	settings, err := config.ParseWebhookProcessorSettings(w.config)
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(w.config.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := w.newRequest(ctx, settings, input)
	if err != nil {
		return nil, err
	}

	client := w.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	common.LogDebug("[PROCESSOR:WEBHOOK] '%s': POST %s", w.config.Name, settings.URL)
	resp, err := client.Do(req)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("processor '%s' timed out after %d seconds", w.config.Name, w.config.Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("processor '%s' webhook request failed: %w", w.config.Name, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	output, err := w.decodeResponse(resp)
	if err != nil {
		return nil, err
	}

	if input != nil && input.Payload != nil && output.Payload == nil {
		common.LogWarn("[PROCESSOR:WEBHOOK] '%s': input contained a payload but the output does not; "+
			"request/result modifications will be skipped. Ensure the webhook output includes a \"payload\" field.", w.config.Name)
	}

	return output, nil
}

func (w *WebhookProcessor) newRequest(
	ctx context.Context,
	settings *config.WebhookProcessorSettings,
	input *DataContext,
) (*http.Request, error) {
	body, err := marshalProcessorInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processor input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("processor '%s': failed to create webhook request: %w", w.config.Name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range expandProcessorHeaders(settings.Headers) {
		req.Header.Set(key, value)
	}
	return req, nil
}

func (w *WebhookProcessor) decodeResponse(resp *http.Response) (*DataContext, error) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("processor '%s': failed to read webhook response: %w", w.config.Name, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errorMsg := fmt.Sprintf("processor '%s' webhook returned HTTP %d", w.config.Name, resp.StatusCode)
		if len(responseBody) > 0 {
			errorMsg = fmt.Sprintf("%s\nbody: %s", errorMsg, string(responseBody))
		}
		return nil, fmt.Errorf("%s", errorMsg)
	}

	return decodeProcessorJSONOutput(w.config.Name, responseBody)
}
