package agentrunner

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// RunOptions configures one headless agent invocation against a Centian MCP URL.
type RunOptions struct {
	Agent            string
	ArtifactRoot     string
	WorkspacePath    string
	MCPURL           string
	Prompt           string
	Timeout          time.Duration
	ClaudeModel      string
	GeminiModel      string
	CodexModel       string
	CodexOllamaModel string
	CodexConfigPath  string
	Stdout           io.Writer
	Stderr           io.Writer
}

// RunResult describes one completed agent invocation.
type RunResult struct {
	Agent         string
	ArtifactRoot  string
	WorkspacePath string
	StdoutPath    string
	StderrPath    string
	SelectedModel string
}

// Run launches one supported agent against the provided MCP URL.
func Run(ctx context.Context, opts *RunOptions) (*RunResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("run options are required")
	}
	if strings.TrimSpace(opts.Agent) == "" {
		return nil, fmt.Errorf("agent is required")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		return nil, fmt.Errorf("artifact root is required")
	}
	if strings.TrimSpace(opts.WorkspacePath) == "" {
		return nil, fmt.Errorf("workspace path is required")
	}
	if strings.TrimSpace(opts.MCPURL) == "" {
		return nil, fmt.Errorf("MCP URL is required")
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	layout := &demoLayout{
		RootPath:        opts.ArtifactRoot,
		WorkspacePath:   opts.WorkspacePath,
		AgentStdoutPath: filepath.Join(opts.ArtifactRoot, "agent.stdout.log"),
		AgentStderrPath: filepath.Join(opts.ArtifactRoot, "agent.stderr.log"),
		ClaudeConfig:    filepath.Join(opts.ArtifactRoot, "claude_mcp_config.json"),
		GeminiConfig:    filepath.Join(opts.WorkspacePath, ".gemini", "settings.json"),
		CodexConfig:     filepath.Join(opts.ArtifactRoot, "codex-home", "config.toml"),
		MCPURL:          opts.MCPURL,
	}

	adapter, err := selectAdapterForAgent(
		opts.Agent,
		opts.ClaudeModel,
		opts.GeminiModel,
		opts.CodexModel,
		opts.CodexOllamaModel,
		opts.CodexConfigPath,
	)
	if err != nil {
		return nil, err
	}
	if err := adapter.isAvailable(); err != nil {
		return nil, fmt.Errorf("%s is not available: %w", adapter.name(), err)
	}
	if err := adapter.writeConfig(layout); err != nil {
		return nil, err
	}
	defer func() {
		if err := adapter.cleanup(layout); err != nil && opts.Stderr != nil {
			_, _ = fmt.Fprintf(opts.Stderr, "warning: cleanup %s artifacts: %v\n", adapter.name(), err)
		}
	}()

	result := &RunResult{
		Agent:         adapter.name(),
		ArtifactRoot:  opts.ArtifactRoot,
		WorkspacePath: opts.WorkspacePath,
		StdoutPath:    layout.AgentStdoutPath,
		StderrPath:    layout.AgentStderrPath,
		SelectedModel: selectedModelForAgent(adapter.name(), opts),
	}

	if err := runAgentPrompt(ctx, adapter, layout, opts.Prompt, opts.Stdout, opts.Stderr, opts.Timeout); err != nil {
		return result, err
	}
	return result, nil
}

func selectAdapterForAgent(agent, claudeModel, geminiModel, codexModel, codexOllamaModel, codexConfigPath string) (agentAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case AgentClaude:
		return claudeAdapter{model: claudeModel}, nil
	case AgentGemini:
		return geminiAdapter{model: geminiModel}, nil
	case AgentCodex:
		return codexAdapter{model: codexModel, baseConfigPath: codexConfigPath}, nil
	case AgentCodexOllama:
		return codexOllamaAdapter{model: codexOllamaModel, baseConfigPath: codexConfigPath}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; v1 supports %q, %q, %q, and %q", agent, AgentClaude, AgentGemini, AgentCodex, AgentCodexOllama)
	}
}

func selectedModelForAgent(agent string, opts *RunOptions) string {
	switch agent {
	case AgentClaude:
		return strings.TrimSpace(opts.ClaudeModel)
	case AgentGemini:
		return strings.TrimSpace(opts.GeminiModel)
	case AgentCodex:
		return strings.TrimSpace(opts.CodexModel)
	case AgentCodexOllama:
		return selectedCodexOllamaModelLabel(opts.CodexOllamaModel, opts.CodexConfigPath)
	default:
		return ""
	}
}
