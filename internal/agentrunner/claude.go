package agentrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type claudeAdapter struct {
	model string
}

func (claudeAdapter) name() string { return AgentClaude }

func (claudeAdapter) isAvailable() error {
	_, err := exec.LookPath("claude")
	return err
}

func (c claudeAdapter) writeConfig(layout *demoLayout) error {
	content, err := asset("claude_mcp_config.json")
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "__MCP_URL__", layout.MCPURL)
	var payload any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return fmt.Errorf("parse claude MCP config: %w", err)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode claude MCP config: %w", err)
	}
	if err := os.WriteFile(layout.ClaudeConfig, encoded, 0o600); err != nil {
		return fmt.Errorf("write claude_mcp_config.json: %w", err)
	}
	return nil
}

func (c claudeAdapter) command(layout *demoLayout, _ string) ([]string, error) {
	command := []string{
		"claude",
		"-p",
		"--output-format", "json",
		"--no-session-persistence",
		"--permission-mode", "default",
		"--allowedTools", "mcp__centian__*",
		"--mcp-config", layout.ClaudeConfig,
		"--strict-mcp-config",
		"--tools", "",
	}
	model := strings.TrimSpace(c.model)
	if model != "" {
		command = append(command, "--model", model)
	}
	return command, nil
}
