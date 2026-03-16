package processor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/assert"
)

func TestNewWebhookProcessorAndGetConfig(t *testing.T) {
	cfg := &config.ProcessorConfig{
		Name:    "demo-webhook",
		Type:    "webhook",
		Enabled: true,
		Timeout: 5,
		Config: map[string]interface{}{
			"url": "http://example.com/processor",
		},
	}

	p, err := NewWebhookProcessor(cfg)
	assert.NilError(t, err)
	assert.Assert(t, p.GetConfig() == cfg)
	assert.Assert(t, p.HTTPClient != nil)
}

func TestWebhookProcessorProcess(t *testing.T) {
	makeContext := func() *DataContext {
		return &DataContext{
			Version: "1.0",
			Payload: &PayloadPart{
				Request: &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{
						Name:      "tool-a",
						Arguments: json.RawMessage(`{"a":1}`),
						Meta:      mcp.Meta{"progressToken": "token-1"},
					},
				},
			},
		}
	}

	t.Run("success roundtrip with headers and DTO payload", func(t *testing.T) {
		originalEnv := os.Getenv("PROCESSOR_AUTH_TOKEN")
		t.Setenv("PROCESSOR_AUTH_TOKEN", "secret-token")
		t.Cleanup(func() {
			_ = os.Setenv("PROCESSOR_AUTH_TOKEN", originalEnv)
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, http.MethodPost)
			assert.Equal(t, r.Header.Get("Content-Type"), "application/json")
			assert.Equal(t, r.Header.Get("Authorization"), "Bearer secret-token")
			assert.Equal(t, r.Header.Get("X-Trace"), "trace-1")

			var payload map[string]any
			assert.NilError(t, json.NewDecoder(r.Body).Decode(&payload))
			requestPayload := payload["payload"].(map[string]any)
			request := requestPayload["request"].(map[string]any)
			params := request["Params"].(map[string]any)
			assert.Equal(t, params["name"], "tool-a")
			assert.DeepEqual(t, params["_meta"], map[string]any{"progressToken": "token-1"})

			w.Header().Set("Content-Type", "application/json")
			assert.NilError(t, json.NewEncoder(w).Encode(map[string]any{
				"payload": map[string]any{
					"request": map[string]any{
						"Params": map[string]any{
							"name":      "tool-b",
							"arguments": json.RawMessage(`{"b":2}`),
						},
					},
				},
			}))
		}))
		defer server.Close()

		cfg := &config.ProcessorConfig{
			Name:    "webhook-ok",
			Type:    "webhook",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"url": server.URL,
				"headers": map[string]interface{}{
					"Authorization": "Bearer ${PROCESSOR_AUTH_TOKEN}",
					"X-Trace":       "trace-1",
				},
			},
		}

		p := &WebhookProcessor{HTTPClient: server.Client(), config: cfg}
		out, err := p.Process(makeContext())
		assert.NilError(t, err)
		assert.Assert(t, out != nil)
		assert.Assert(t, out.Payload != nil)
		assert.Assert(t, out.Payload.Request != nil)
		assert.Equal(t, out.Payload.Request.Params.Name, "tool-b")
		assert.Equal(t, string(out.Payload.Request.Params.Arguments), `{"b":2}`)
	})

	t.Run("http error returns execution failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "broken", http.StatusInternalServerError)
		}))
		defer server.Close()

		cfg := &config.ProcessorConfig{
			Name:    "webhook-http-fail",
			Type:    "webhook",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"url": server.URL,
			},
		}

		p := &WebhookProcessor{HTTPClient: server.Client(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "HTTP 500"))
	})

	t.Run("invalid json output returns error with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer server.Close()

		cfg := &config.ProcessorConfig{
			Name:    "webhook-invalid-json",
			Type:    "webhook",
			Enabled: true,
			Timeout: 5,
			Config: map[string]interface{}{
				"url": server.URL,
			},
		}

		p := &WebhookProcessor{HTTPClient: server.Client(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "returned invalid JSON"))
		assert.Assert(t, strings.Contains(err.Error(), "stdout: not-json"))
	})

	t.Run("timeout returns timeout error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		cfg := &config.ProcessorConfig{
			Name:    "webhook-timeout",
			Type:    "webhook",
			Enabled: true,
			Timeout: 1,
			Config: map[string]interface{}{
				"url": server.URL,
			},
		}

		p := &WebhookProcessor{HTTPClient: server.Client(), config: cfg}
		_, err := p.Process(makeContext())
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "timed out"))
	})
}
