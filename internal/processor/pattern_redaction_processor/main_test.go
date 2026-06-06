package patternredactionprocessor

import (
	"encoding/json"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestProcessJSONRedactsConfiguredRules(t *testing.T) {
	input := `{
		"version": "1.0",
		"event": {"direction": "[SERVER -> CLIENT]", "success": true},
		"payload": {
			"result": {
				"content": [
					{"type": "text", "text": "token it_abcdefghijklmnopqrst and id cust-12345"}
				]
			}
		}
	}`

	output := runPatternProcessor(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinPatternRedactionProcessor,
		Mode:      config.BuiltinRedactionModeRedact,
		Scope:     config.BuiltinRedactionScopeBoth,
		Rules: []config.BuiltinRedactionRule{
			{Name: "internal_token", Pattern: `it_[a-z]{20}`, Replacement: "[REDACTED_INTERNAL_TOKEN]"},
			{Name: "customer_id", Pattern: `cust-\d+`, Replacement: "[REDACTED_CUSTOMER_ID]"},
		},
	})

	result := output["payload"].(map[string]any)["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"]
	assert.Equal(t, text, "token [REDACTED_INTERNAL_TOKEN] and id [REDACTED_CUSTOMER_ID]")

	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["processor"], config.BuiltinPatternRedactionProcessor)
	assert.Equal(t, report["action"], "redacted")
	details := report["details"].(map[string]any)
	assert.Equal(t, details["match_count"], float64(2))
}

func TestProcessJSONPassesBenignText(t *testing.T) {
	input := `{
		"version": "1.0",
		"payload": {
			"result": {
				"content": [
					{"type": "text", "text": "no sensitive content"}
				]
			}
		}
	}`

	output := runPatternProcessor(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinPatternRedactionProcessor,
		Mode:      config.BuiltinRedactionModeRedact,
		Scope:     config.BuiltinRedactionScopeBoth,
		Rules: []config.BuiltinRedactionRule{
			{Name: "internal_token", Pattern: `it_[a-z]{20}`, Replacement: "[REDACTED_INTERNAL_TOKEN]"},
		},
	})

	if _, ok := output["annotations"]; ok {
		t.Fatalf("expected no annotations for benign text, got %#v", output["annotations"])
	}
}

func runPatternProcessor(t *testing.T, input string, settings *config.BuiltinProcessorSettings) map[string]any {
	t.Helper()

	output, err := ProcessJSON([]byte(input), settings)
	assert.NilError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(output, &decoded)
	assert.NilError(t, err)
	return decoded
}
