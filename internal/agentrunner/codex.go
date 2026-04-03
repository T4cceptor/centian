package agentrunner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type codexAdapter struct {
	model string
}

func (codexAdapter) name() string { return AgentCodex }

func (codexAdapter) isAvailable() error {
	_, err := exec.LookPath("codex")
	return err
}

func (c codexAdapter) writeConfig(layout *demoLayout) error {
	content, err := asset("codex_config.toml")
	if err != nil {
		return err
	}
	modelBlock := ""
	if model := strings.TrimSpace(c.model); model != "" {
		modelBlock = `model = "` + model + `"`
	}
	content = strings.NewReplacer(
		"__MODEL_BLOCK__", modelBlock,
		"__MCP_URL__", layout.MCPURL,
		"__WORK_DIR__", layout.WorkspacePath,
	).Replace(content)
	if err := os.MkdirAll(filepath.Dir(layout.CodexConfig), 0o750); err != nil {
		return fmt.Errorf("create codex config dir: %w", err)
	}
	if err := copyCodexAuth(filepath.Dir(layout.CodexConfig)); err != nil {
		return err
	}
	if err := os.WriteFile(layout.CodexConfig, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write codex config.toml: %w", err)
	}
	return nil
}

func copyCodexAuth(codexHome string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home dir: %w", err)
	}
	sourcePath := filepath.Join(homeDir, ".codex", "auth.json")
	authData, err := os.ReadFile(sourcePath)
	switch {
	case err == nil:
		targetPath := filepath.Join(codexHome, "auth.json")
		if err := os.WriteFile(targetPath, authData, 0o600); err != nil {
			return fmt.Errorf("write codex auth.json: %w", err)
		}
		return nil
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("read codex auth.json: %w", err)
	}
}

func (codexAdapter) env(layout *demoLayout) []string {
	return []string{"CODEX_HOME=" + filepath.Dir(layout.CodexConfig)}
}

func (codexAdapter) cleanup(layout *demoLayout) error {
	return os.RemoveAll(filepath.Dir(layout.CodexConfig))
}

func (c codexAdapter) command(layout *demoLayout, _ string) ([]string, error) {
	command := []string{
		"codex",
		"exec",
		"--skip-git-repo-check",
		"--json",
		"-C", layout.WorkspacePath,
		"-o", filepath.Join(layout.RootPath, "codex_output.txt"),
		"-",
	}
	return command, nil
}
