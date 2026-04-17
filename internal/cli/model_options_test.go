package cli

import (
	"testing"

	"github.com/T4cceptor/centian/internal/common"
)

func TestNormalizeCLIModel(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		input    string
		expected string
	}{
		{name: "codex mini typo", agent: "codex", input: "gpt5.4-mini", expected: common.ModelCodexGPT54Mini},
		{name: "codex full", agent: "codex", input: "gpt5.4", expected: common.ModelCodexGPT54},
		{name: "codex ollama custom passthrough", agent: "codex-ollama", input: "my-profile", expected: "my-profile"},
		{name: "claude alias", agent: "claude", input: "Sonnet", expected: common.ModelClaudeSonnet},
		{name: "gemini pro latest", agent: "gemini", input: "pro", expected: common.ModelGemini31ProPreview},
		{name: "gemini flash latest", agent: "gemini", input: "flash", expected: common.ModelGemini3FlashPreview},
		{name: "gemini flash 2.5", agent: "gemini", input: "2.5-flash", expected: common.ModelGemini25Flash},
		{name: "unknown passthrough", agent: "codex", input: "custom-model", expected: "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := normalizeCLIModel(tt.agent, tt.input); actual != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}
