package secrettokenredactor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestProcessJSONRedactsKnownSecretFamilies(t *testing.T) {
	input := `{
		"version": "1.0",
		"event": {"direction": "[SERVER -> CLIENT]", "success": true},
		"payload": {
			"result": {
				"content": [
					{"type": "text", "text": "openai sk-abcdefghijklmnopqrstuvwxyz github ghp_abcdefghijklmnopqrstuvwxyz aws AKIA1234567890ABCDEF"}
				]
			}
		}
	}`

	output := runSecretProcessor(t, input, config.BuiltinRedactionScopeBoth)
	text := output["payload"].(map[string]any)["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)

	assert.Assert(t, strings.Contains(text, "[REDACTED_OPENAI_API_KEY]"))
	assert.Assert(t, strings.Contains(text, "[REDACTED_GITHUB_TOKEN]"))
	assert.Assert(t, strings.Contains(text, "[REDACTED_AWS_ACCESS_KEY]"))
	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["severity"], "high")
}

func TestProcessJSONRedactsRequestToken(t *testing.T) {
	input := `{
		"version": "1.0",
		"event": {"direction": "[CLIENT -> SERVER]", "success": true},
		"payload": {
			"request": {
				"Params": {
					"name": "tool",
					"arguments": {
						"authorization": "Bearer abcdefghijklmnopqrstuvwxyz"
					}
				}
			}
		}
	}`

	output := runSecretProcessor(t, input, config.BuiltinRedactionScopeBoth)
	args := output["payload"].(map[string]any)["request"].(map[string]any)["Params"].(map[string]any)["arguments"].(map[string]any)
	assert.Equal(t, args["authorization"], "[REDACTED_BEARER_TOKEN]")
}

func runSecretProcessor(t *testing.T, input string, scope string) map[string]any {
	t.Helper()

	output, err := ProcessJSON([]byte(input), &config.BuiltinProcessorSettings{
		Processor: config.BuiltinSecretTokenRedactor,
		Mode:      config.BuiltinRedactionModeRedact,
		Scope:     scope,
	})
	assert.NilError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(output, &decoded)
	assert.NilError(t, err)
	return decoded
}
