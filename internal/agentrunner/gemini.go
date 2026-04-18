package agentrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// geminiAdapter runs the Gemini CLI against the demo-local MCP server.
type geminiAdapter struct {
	model string
}

// name returns the public agent identifier for the Gemini CLI.
func (geminiAdapter) name() string { return AgentGemini }

// isAvailable checks whether the Gemini CLI is installed on PATH.
func (geminiAdapter) isAvailable() error {
	_, err := exec.LookPath("gemini")
	return err
}

// writeConfig renders the Gemini settings file that points at the demo-local Centian endpoint.
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

// env returns extra environment variables required by the Gemini CLI.
func (geminiAdapter) env(*demoLayout) []string { return nil }

// cleanup removes any adapter-specific runtime artifacts after the run.
func (geminiAdapter) cleanup(*demoLayout) error { return nil }

// command builds the non-interactive Gemini CLI invocation for one run.
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
