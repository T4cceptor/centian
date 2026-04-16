package cli

import (
	"fmt"
	"strings"

	"github.com/T4cceptor/centian/internal/agentrunner"
)

const (
	codexModelStable       = "gpt-5.4"
	codexModelMini         = "gpt-5.4-mini"
	codexModelHelp         = codexModelStable + ", " + codexModelMini
	codexOllamaProfileHelp = "profile name from the supplied Codex config"
	claudeModelHelp        = "haiku, sonnet, opus"
	geminiModelHelp        = "pro (gemini-3.1-pro-preview), flash (gemini-3-flash-preview), 2.5-flash (gemini-2.5-flash)"
)

func singleModelFlagUsage() string {
	return fmt.Sprintf("Model for selected agent. Codex: %s; Claude: %s; Gemini: %s", codexModelHelp, claudeModelHelp, geminiModelHelp)
}

func normalizeCLIModel(agent, model string) string {
	return agentrunner.NormalizeModelForAgent(agent, model)
}

func benchmarkExecutionsFromFlags(cmdModel, cmdProfile string, agents []string, claudeModel, geminiModel, codexModel, codexConfigPath string) ([]agentrunner.AgentExecutionOptions, error) {
	inputs := &benchmarkExecutionFlagInputs{
		profile:         cmdProfile,
		claudeModel:     claudeModel,
		geminiModel:     geminiModel,
		codexModel:      codexModel,
		codexConfigPath: codexConfigPath,
	}
	executions := buildBenchmarkExecutions(agents, inputs)
	profile := strings.TrimSpace(cmdProfile)
	model := strings.TrimSpace(cmdModel)
	if profile == "" && model == "" {
		return normalizeExecutions(executions)
	}
	if err := validateBenchmarkProfileOverride(profile, executions); err != nil {
		return nil, err
	}
	if model == "" {
		return normalizeExecutions(executions)
	}
	if err := applyBenchmarkSingleModelOverride(model, agents, executions); err != nil {
		return nil, err
	}
	return normalizeExecutions(executions)
}

type benchmarkExecutionFlagInputs struct {
	profile         string
	claudeModel     string
	geminiModel     string
	codexModel      string
	codexConfigPath string
}

func buildBenchmarkExecutions(agents []string, inputs *benchmarkExecutionFlagInputs) []agentrunner.AgentExecutionOptions {
	executions := make([]agentrunner.AgentExecutionOptions, 0, len(agents))
	for _, agent := range agents {
		executions = append(executions, buildBenchmarkExecution(strings.ToLower(strings.TrimSpace(agent)), inputs))
	}
	return executions
}

func buildBenchmarkExecution(agent string, inputs *benchmarkExecutionFlagInputs) agentrunner.AgentExecutionOptions {
	exec := agentrunner.AgentExecutionOptions{Agent: agent}
	switch agent {
	case agentrunner.AgentClaude:
		exec.Model = normalizeCLIModel(agent, inputs.claudeModel)
	case agentrunner.AgentGemini:
		exec.Model = normalizeCLIModel(agent, inputs.geminiModel)
	case agentrunner.AgentCodex:
		exec.Model = normalizeCLIModel(agent, inputs.codexModel)
		exec.CodexConfigPath = strings.TrimSpace(inputs.codexConfigPath)
	case agentrunner.AgentCodexOllama:
		exec.Profile = strings.TrimSpace(inputs.profile)
		exec.CodexConfigPath = strings.TrimSpace(inputs.codexConfigPath)
	}
	return exec
}

func validateBenchmarkProfileOverride(profile string, executions []agentrunner.AgentExecutionOptions) error {
	if profile == "" {
		return nil
	}
	for _, exec := range executions {
		if exec.Agent == agentrunner.AgentCodexOllama {
			return nil
		}
	}
	return fmt.Errorf("--profile can only be used when --agent codex-ollama is selected")
}

func applyBenchmarkSingleModelOverride(model string, agents []string, executions []agentrunner.AgentExecutionOptions) error {
	if len(agents) != 1 {
		return fmt.Errorf("--model can only be used with exactly one non-codex-ollama agent; use --claude-model, --gemini-model, or --codex-model for multi-agent runs")
	}
	agent := executions[0].Agent
	normalized := normalizeCLIModel(agent, model)
	switch agent {
	case agentrunner.AgentClaude:
		if executions[0].Model != "" {
			return fmt.Errorf("--model cannot be combined with --claude-model")
		}
	case agentrunner.AgentGemini:
		if executions[0].Model != "" {
			return fmt.Errorf("--model cannot be combined with --gemini-model")
		}
	case agentrunner.AgentCodex:
		if executions[0].Model != "" {
			return fmt.Errorf("--model cannot be combined with --codex-model")
		}
	case agentrunner.AgentCodexOllama:
		return fmt.Errorf("--model is not supported for codex-ollama; use --profile")
	default:
		return fmt.Errorf("unsupported agent %q; cannot apply --model/--profile", agents[0])
	}
	executions[0].Model = normalized
	return nil
}

func normalizeExecutions(executions []agentrunner.AgentExecutionOptions) ([]agentrunner.AgentExecutionOptions, error) {
	normalized := make([]agentrunner.AgentExecutionOptions, 0, len(executions))
	for _, exec := range executions {
		item, err := agentrunner.NormalizeExecutionOptions(exec)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}
