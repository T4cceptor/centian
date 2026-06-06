package toolcallguard

import (
	"encoding/json"
	"testing"

	"github.com/T4cceptor/centian/internal/config"
	"gotest.tools/assert"
)

func TestProcessJSONBlocksConfiguredRule(t *testing.T) {
	input := requestInput(t, "crm___delete_user", map[string]any{"id": "u_123"})

	output := runToolCallGuard(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinToolCallGuard,
		Mode:      config.BuiltinToolGuardModeBlock,
		GuardRules: []config.BuiltinToolGuardRule{
			{
				Name:         "block_delete_user_tool",
				Severity:     "high",
				Message:      "User deletion is not allowed through this gateway.",
				ToolPatterns: []string{"crm___delete_user", "delete_user"},
			},
		},
	})

	result := output["payload"].(map[string]any)["result"].(map[string]any)
	assert.Equal(t, result["isError"], true)
	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["processor"], config.BuiltinToolCallGuard)
	assert.Equal(t, report["action"], "blocked")
	assert.Equal(t, report["category"], "policy")
	assert.Equal(t, report["severity"], "high")
}

func TestProcessJSONAnnotatesConfiguredRule(t *testing.T) {
	input := requestInput(t, "deploy___run", map[string]any{"environment": "prod"})

	output := runToolCallGuard(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinToolCallGuard,
		Mode:      config.BuiltinToolGuardModeAnnotate,
		GuardRules: []config.BuiltinToolGuardRule{
			{
				Name:         "block_prod_environment",
				ToolPatterns: []string{"deploy___*"},
				ArgumentRules: []config.BuiltinToolGuardArgumentRule{
					{Path: "environment", Pattern: "^prod$"},
				},
			},
		},
	})

	payload := output["payload"].(map[string]any)
	if _, ok := payload["result"]; ok {
		t.Fatalf("expected annotate mode to leave result unset, got %#v", payload["result"])
	}
	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["action"], "annotated")
}

func TestProcessJSONDangerousCommandsPresetBlocksRepresentativeCommand(t *testing.T) {
	input := requestInput(t, "desktop_commander___exec", map[string]any{
		"command": "rm -rf /tmp/build",
	})

	output := runToolCallGuard(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinToolCallGuard,
		Mode:      config.BuiltinToolGuardModeBlock,
		Presets:   []string{config.BuiltinToolGuardPresetDangerousCommands},
	})

	result := output["payload"].(map[string]any)["result"].(map[string]any)
	assert.Equal(t, result["isError"], true)
	report := output["annotations"].(map[string]any)["reports"].([]any)[0].(map[string]any)
	assert.Equal(t, report["severity"], "high")
	details := report["details"].(map[string]any)
	assert.Equal(t, details["matched_rule"], "dangerous_rm_rf")
}

func TestProcessJSONDangerousCommandsPresetPassesBenignShellCommand(t *testing.T) {
	input := requestInput(t, "desktop_commander___exec", map[string]any{
		"command": "ls -la /tmp",
	})

	output := runToolCallGuard(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinToolCallGuard,
		Mode:      config.BuiltinToolGuardModeBlock,
		Presets:   []string{config.BuiltinToolGuardPresetDangerousCommands},
	})

	if _, ok := output["annotations"]; ok {
		t.Fatalf("expected no annotation for benign shell command, got %#v", output["annotations"])
	}
}

func TestProcessJSONDangerousCommandsPresetIgnoresNonShellToolText(t *testing.T) {
	input := requestInput(t, "notes___create", map[string]any{
		"command": "rm -rf /",
	})

	output := runToolCallGuard(t, input, &config.BuiltinProcessorSettings{
		Processor: config.BuiltinToolCallGuard,
		Mode:      config.BuiltinToolGuardModeBlock,
		Presets:   []string{config.BuiltinToolGuardPresetDangerousCommands},
	})

	if _, ok := output["annotations"]; ok {
		t.Fatalf("expected no annotation for non-shell tool, got %#v", output["annotations"])
	}
}

func requestInput(t *testing.T, toolName string, args map[string]any) string {
	t.Helper()
	rawArgs, err := json.Marshal(args)
	assert.NilError(t, err)

	input := map[string]any{
		"version": "1.0",
		"payload": map[string]any{
			"request": map[string]any{
				"Params": map[string]any{
					"name":      toolName,
					"arguments": json.RawMessage(rawArgs),
				},
			},
		},
		"routing": map[string]any{
			"tool_name":          toolName,
			"original_tool_name": toolName,
		},
	}
	raw, err := json.Marshal(input)
	assert.NilError(t, err)
	return string(raw)
}

func runToolCallGuard(t *testing.T, input string, settings *config.BuiltinProcessorSettings) map[string]any {
	t.Helper()

	output, err := ProcessJSON([]byte(input), settings)
	assert.NilError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(output, &decoded)
	assert.NilError(t, err)
	return decoded
}
