package agentrunner

import (
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
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
		case "gpt5.4", common.ModelCodexGPT54:
			return common.ModelCodexGPT54
		case "gpt5.4-mini", common.ModelCodexGPT54Mini:
			return common.ModelCodexGPT54Mini
		}
	case AgentClaude:
		switch normalized {
		case common.ModelClaudeHaiku, common.ModelClaudeSonnet, common.ModelClaudeOpus:
			return normalized
		}
	case AgentGemini:
		switch normalized {
		case "pro", "gemini-3.1-pro", common.ModelGemini31ProPreview:
			return common.ModelGemini31ProPreview
		case "flash", "gemini-3-flash", common.ModelGemini3FlashPreview:
			return common.ModelGemini3FlashPreview
		case "2.5-flash", common.ModelGemini25Flash:
			return common.ModelGemini25Flash
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

	if opts.Agent != AgentCodexOllama && opts.Profile != "" {
		// return error if selected agent is NOT codex-ollama, and profile is available
		return AgentExecutionOptions{}, fmt.Errorf("profile is only supported for %q", AgentCodexOllama)
	}

	switch opts.Agent {
	case AgentClaude:
		if opts.Model == "" {
			opts.Model = DefaultClaudeModel
		}
	case AgentGemini:
		if opts.Model == "" {
			opts.Model = DefaultGeminiModel
		}
	case AgentCodex:
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
