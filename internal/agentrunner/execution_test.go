package agentrunner

import "testing"

func TestNormalizeModelForAgent(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		input    string
		expected string
	}{
		{name: "codex alias", agent: AgentCodex, input: "gpt5.4", expected: "gpt-5.4"},
		{name: "codex mini alias", agent: AgentCodex, input: "gpt5.4-mini", expected: "gpt-5.4-mini"},
		{name: "claude alias", agent: AgentClaude, input: "Sonnet", expected: "sonnet"},
		{name: "gemini alias", agent: AgentGemini, input: "flash", expected: "gemini-3-flash-preview"},
		{name: "custom passthrough", agent: AgentCodex, input: "custom-model", expected: "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := NormalizeModelForAgent(tt.agent, tt.input); actual != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestNormalizeExecutionOptionsAppliesDefaultsAndAliases(t *testing.T) {
	exec, err := NormalizeExecutionOptions(AgentExecutionOptions{
		Agent: AgentClaude,
		Model: "Sonnet",
	})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions: %v", err)
	}
	if exec.Model != "sonnet" {
		t.Fatalf("expected normalized model, got %q", exec.Model)
	}

	exec, err = NormalizeExecutionOptions(AgentExecutionOptions{Agent: AgentGemini})
	if err != nil {
		t.Fatalf("NormalizeExecutionOptions default gemini: %v", err)
	}
	if exec.Model != DefaultGeminiModel {
		t.Fatalf("expected default gemini model %q, got %q", DefaultGeminiModel, exec.Model)
	}
}

func TestNormalizeExecutionOptionsRejectsInvalidProfileUsage(t *testing.T) {
	_, err := NormalizeExecutionOptions(AgentExecutionOptions{
		Agent:   AgentCodex,
		Profile: "local-oss",
	})
	if err == nil || err.Error() != `profile is only supported for "codex-ollama"` {
		t.Fatalf("expected invalid profile error, got %v", err)
	}
}

func TestNormalizeExecutionOptionsRejectsCodexOllamaModelAndRequiresProfile(t *testing.T) {
	_, err := NormalizeExecutionOptions(AgentExecutionOptions{
		Agent: AgentCodexOllama,
		Model: "gpt-oss-20b",
	})
	if err == nil || err.Error() != "codex-ollama does not support model selection; use profile" {
		t.Fatalf("expected codex-ollama model rejection, got %v", err)
	}

	_, err = NormalizeExecutionOptions(AgentExecutionOptions{Agent: AgentCodexOllama})
	if err == nil || err.Error() != "codex-ollama requires profile" {
		t.Fatalf("expected missing profile error, got %v", err)
	}
}

func TestSelectedModelForExecution(t *testing.T) {
	if actual := SelectedModelForExecution(AgentExecutionOptions{Agent: AgentCodex, Model: "gpt-5.4-mini"}); actual != "gpt-5.4-mini" {
		t.Fatalf("expected codex model, got %q", actual)
	}
	if actual := SelectedModelForExecution(AgentExecutionOptions{Agent: AgentCodexOllama, Profile: "local-oss"}); actual != "local-oss" {
		t.Fatalf("expected codex-ollama profile, got %q", actual)
	}
}
