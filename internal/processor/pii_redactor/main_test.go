package piiredactor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestProcessJSONRedactsDeterministicPII(t *testing.T) {
	input := `{
		"version": "1.0",
		"event": {"direction": "[SERVER -> CLIENT]", "success": true},
		"payload": {
			"result": {
				"content": [
					{"type": "text", "text": "email jane@example.com phone +1 415 555 0199 iban DE89370400440532013000 card 4111 1111 1111 1111"}
				]
			}
		}
	}`

	output := runPIIProcessor(t, input)
	text := output["payload"].(map[string]any)["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)

	assert.Assert(t, strings.Contains(text, "[REDACTED_EMAIL]"))
	assert.Assert(t, strings.Contains(text, "[REDACTED_PHONE]"))
	assert.Assert(t, strings.Contains(text, "[REDACTED_IBAN]"))
	assert.Assert(t, strings.Contains(text, "[REDACTED_CARD_LIKE_NUMBER]"))
	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["category"], "privacy")
}

func TestProcessJSONPassesBenignText(t *testing.T) {
	input := `{
		"version": "1.0",
		"payload": {
			"result": {
				"content": [
					{"type": "text", "text": "release 1.2.3 finished on 2026-06-06 with build id 12345"}
				]
			}
		}
	}`

	output := runPIIProcessor(t, input)
	if _, ok := output["annotations"]; ok {
		t.Fatalf("expected no annotations for benign text, got %#v", output["annotations"])
	}
}

func runPIIProcessor(t *testing.T, input string) map[string]any {
	t.Helper()

	output, err := ProcessJSON([]byte(input), &config.BuiltinProcessorSettings{
		Processor: config.BuiltinPIIRedactor,
		Mode:      config.BuiltinRedactionModeRedact,
		Scope:     config.BuiltinRedactionScopeResponse,
	})
	assert.NilError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(output, &decoded)
	assert.NilError(t, err)
	return decoded
}
