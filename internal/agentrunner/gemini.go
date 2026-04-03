package agentrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type geminiAdapter struct {
	model string
}

func (geminiAdapter) name() string { return AgentGemini }

func (geminiAdapter) isAvailable() error {
	_, err := exec.LookPath("gemini")
	return err
}

func (g geminiAdapter) writeConfig(layout *demoLayout) error {
	content, err := asset("gemini_settings.json")
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "__MCP_URL__", layout.MCPURL)
	var payload any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return fmt.Errorf("parse gemini settings: %w", err)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gemini settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(layout.GeminiConfig), 0o750); err != nil {
		return fmt.Errorf("create gemini config dir: %w", err)
	}
	if err := os.WriteFile(layout.GeminiConfig, encoded, 0o600); err != nil {
		return fmt.Errorf("write gemini settings: %w", err)
	}
	return nil
}

func (geminiAdapter) env(*demoLayout) []string { return nil }

func (g geminiAdapter) command(_ *demoLayout, prompt string) ([]string, error) {
	command := []string{
		"gemini",
		"-p",
		prompt,
		// "--debug", // add this back in if there is Gemini trouble again
		"--output-format", "json",
		"--sandbox",
		"--approval-mode", "default",
		"--allowed-mcp-server-names", "centian",
	}
	model := strings.TrimSpace(g.model)
	if model != "" {
		command = append(command, "--model", model)
	}
	return command, nil
}
