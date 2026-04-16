package agentrunner

import (
	"fmt"
	"strings"
)

// AgentExecutionOptions configures one concrete agent invocation.
type AgentExecutionOptions struct {
	Agent           string
	Model           string
	Profile         string
	CodexConfigPath string
}

// NormalizeModelForAgent applies known CLI/internal aliases for one agent model.
func NormalizeModelForAgent(agent, model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}
	normalized := strings.ToLower(trimmed)
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case AgentCodex:
		switch normalized {
		case "gpt5.4", "gpt-5.4":
			return "gpt-5.4"
		case "gpt5.4-mini", "gpt-5.4-mini":
			return "gpt-5.4-mini"
		}
	case AgentClaude:
		switch normalized {
		case "haiku", "sonnet", "opus":
			return normalized
		}
	case AgentGemini:
		switch normalized {
		case "pro", "gemini-3.1-pro", "gemini-3.1-pro-preview":
			return "gemini-3.1-pro-preview"
		case "flash", "gemini-3-flash", "gemini-3-flash-preview":
			return "gemini-3-flash-preview"
		case "2.5-flash", "gemini-2.5-flash":
			return "gemini-2.5-flash"
		}
	}
	return trimmed
}

// NormalizeExecutionOptions trims one execution config, applies per-agent
// defaults, and validates model/profile compatibility.
func NormalizeExecutionOptions(opts AgentExecutionOptions) (AgentExecutionOptions, error) {
	opts.Agent = strings.ToLower(strings.TrimSpace(opts.Agent))
	opts.Model = NormalizeModelForAgent(opts.Agent, opts.Model)
	opts.Profile = strings.TrimSpace(opts.Profile)
	opts.CodexConfigPath = strings.TrimSpace(opts.CodexConfigPath)

	switch opts.Agent {
	case AgentClaude:
		if opts.Profile != "" {
			return AgentExecutionOptions{}, fmt.Errorf("profile is only supported for %q", AgentCodexOllama)
		}
		if opts.Model == "" {
			opts.Model = DefaultClaudeModel
		}
	case AgentGemini:
		if opts.Profile != "" {
			return AgentExecutionOptions{}, fmt.Errorf("profile is only supported for %q", AgentCodexOllama)
		}
		if opts.Model == "" {
			opts.Model = DefaultGeminiModel
		}
	case AgentCodex:
		if opts.Profile != "" {
			return AgentExecutionOptions{}, fmt.Errorf("profile is only supported for %q", AgentCodexOllama)
		}
		if opts.Model == "" {
			opts.Model = DefaultCodexModel
		}
	case AgentCodexOllama:
		if opts.Model != "" {
			return AgentExecutionOptions{}, fmt.Errorf("%s does not support model selection; use profile", AgentCodexOllama)
		}
		if opts.Profile == "" {
			return AgentExecutionOptions{}, fmt.Errorf("%s requires profile", AgentCodexOllama)
		}
	default:
		return AgentExecutionOptions{}, fmt.Errorf("unsupported agent %q; v1 supports %q, %q, %q, and %q", opts.Agent, AgentClaude, AgentGemini, AgentCodex, AgentCodexOllama)
	}

	return opts, nil
}

// SelectedModelForExecution returns the persisted model/profile label for one
// normalized execution.
func SelectedModelForExecution(opts AgentExecutionOptions) string {
	switch strings.ToLower(strings.TrimSpace(opts.Agent)) {
	case AgentCodexOllama:
		return strings.TrimSpace(opts.Profile)
	default:
		return strings.TrimSpace(opts.Model)
	}
}
