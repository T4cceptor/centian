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
	if err := os.WriteFile(layout.CodexConfig, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write codex config.toml: %w", err)
	}
	return nil
}

func (codexAdapter) env(layout *demoLayout) []string {
	return []string{"CODEX_HOME=" + filepath.Dir(layout.CodexConfig)}
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
