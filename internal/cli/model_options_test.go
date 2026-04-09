package cli

import "testing"

func TestNormalizeCLIModel(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		input    string
		expected string
	}{
		{name: "codex mini typo", agent: "codex", input: "gpt5.4-mini", expected: "gpt-5.4-mini"},
		{name: "codex full", agent: "codex", input: "gpt5.4", expected: "gpt-5.4"},
		{name: "codex ollama gemma alias", agent: "codex-ollama", input: "Gemma4", expected: "gemma4"},
		{name: "codex ollama custom passthrough", agent: "codex-ollama", input: "my-profile", expected: "my-profile"},
		{name: "claude alias", agent: "claude", input: "Sonnet", expected: "sonnet"},
		{name: "gemini pro latest", agent: "gemini", input: "pro", expected: "gemini-3.1-pro-preview"},
		{name: "gemini flash latest", agent: "gemini", input: "flash", expected: "gemini-3-flash-preview"},
		{name: "gemini flash 2.5", agent: "gemini", input: "2.5-flash", expected: "gemini-2.5-flash"},
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
