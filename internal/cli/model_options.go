package cli

import (
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/agentrunner"
	"github.com/T4cceptor/centian/internal/benchmarks"
)

const (
	codexModelStable     = "gpt-5.4"
	codexModelMini       = "gpt-5.4-mini"
	codexModelHelp       = codexModelStable + ", " + codexModelMini
	codexOllamaGemma     = "gemma4"
	codexOllamaQwen      = "qwen3.5"
	codexOllamaModelHelp = codexOllamaGemma + ", " + codexOllamaQwen + ", or custom Codex profile name"
	claudeModelHelp      = "haiku, sonnet, opus"
	geminiModelHelp      = "pro (gemini-3.1-pro-preview), flash (gemini-3-flash-preview), 2.5-flash (gemini-2.5-flash)"
)

func singleModelFlagUsage() string {
	return fmt.Sprintf("Model for selected agent. Codex: %s; Codex Ollama: %s; Claude: %s; Gemini: %s", codexModelHelp, codexOllamaModelHelp, claudeModelHelp, geminiModelHelp)
}

func normalizeCLIModel(agent, model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}
	normalized := strings.ToLower(trimmed)
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case agentrunner.AgentCodex:
		switch normalized {
		case "gpt5.4", "gpt-5.4":
			return codexModelStable
		case "gpt5.4-mini", codexModelMini:
			return codexModelMini
		}
	case agentrunner.AgentCodexOllama:
		switch normalized {
		case codexOllamaGemma:
			return codexOllamaGemma
		case codexOllamaQwen:
			return codexOllamaQwen
		}
	case agentrunner.AgentClaude:
		switch normalized {
		case "haiku", "sonnet", "opus":
			return normalized
		}
	case agentrunner.AgentGemini:
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

func benchmarkAgentModelsFromFlags(cmdModel string, agents []string, claudeModel, geminiModel, codexModel, codexOllamaModel string) (benchmarks.AgentModels, error) {
	models := benchmarks.AgentModels{
		Claude:      normalizeCLIModel(agentrunner.AgentClaude, claudeModel),
		Gemini:      normalizeCLIModel(agentrunner.AgentGemini, geminiModel),
		Codex:       normalizeCLIModel(agentrunner.AgentCodex, codexModel),
		CodexOllama: normalizeCLIModel(agentrunner.AgentCodexOllama, codexOllamaModel),
	}
	model := strings.TrimSpace(cmdModel)
	if model == "" {
		return models, nil
	}
	if len(agents) != 1 {
		return benchmarks.AgentModels{}, fmt.Errorf("--model can only be used with exactly one agent; use --claude-model, --gemini-model, --codex-model, or --codex-ollama-model for multi-agent runs")
	}
	agent := strings.ToLower(strings.TrimSpace(agents[0]))
	normalized := normalizeCLIModel(agent, model)
	switch agent {
	case agentrunner.AgentClaude:
		if models.Claude != "" {
			return benchmarks.AgentModels{}, fmt.Errorf("--model cannot be combined with --claude-model")
		}
		models.Claude = normalized
	case agentrunner.AgentGemini:
		if models.Gemini != "" {
			return benchmarks.AgentModels{}, fmt.Errorf("--model cannot be combined with --gemini-model")
		}
		models.Gemini = normalized
	case agentrunner.AgentCodex:
		if models.Codex != "" {
			return benchmarks.AgentModels{}, fmt.Errorf("--model cannot be combined with --codex-model")
		}
		models.Codex = normalized
	case agentrunner.AgentCodexOllama:
		if models.CodexOllama != "" {
			return benchmarks.AgentModels{}, fmt.Errorf("--model cannot be combined with --codex-ollama-model")
		}
		models.CodexOllama = normalized
	default:
		return benchmarks.AgentModels{}, fmt.Errorf("unsupported agent %q; cannot apply --model", agents[0])
	}
	return models, nil
}
