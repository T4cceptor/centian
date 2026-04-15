package agentrunner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	codexHostedConfigAsset  = "codex_config.toml"
	codexOllamaConfigAsset  = "codex_ollama_config.toml"
	codexOllamaGemmaModel   = "gemma4"
	codexOllamaGemmaProfile = "gemma4-local"
	codexOllamaQwenModel    = "qwen3.5"
	codexOllamaQwenProfile  = "qwen3.5-local"
)

// codexAdapter runs the hosted Codex CLI against the demo-local MCP server.
type codexAdapter struct {
	model          string
	baseConfigPath string
}

// codexOllamaAdapter runs Codex OSS mode against a local Ollama-backed profile.
type codexOllamaAdapter struct {
	model          string
	baseConfigPath string
}

// name returns the public agent identifier for the hosted Codex CLI.
func (codexAdapter) name() string { return AgentCodex }

// name returns the public agent identifier for Codex OSS mode via Ollama.
func (codexOllamaAdapter) name() string { return AgentCodexOllama }

// isAvailable checks whether the Codex CLI is installed on PATH.
func (codexAdapter) isAvailable() error {
	_, err := exec.LookPath("codex")
	return err
}

// isAvailable checks whether both the Codex CLI and Ollama are installed on PATH.
func (codexOllamaAdapter) isAvailable() error {
	_, err := exec.LookPath("codex")
	if err == nil {
		_, err = exec.LookPath("ollama")
	}
	return err
}

// writeConfig renders the runtime Codex config for hosted model usage.
func (c codexAdapter) writeConfig(layout *demoLayout) error {
	return writeCodexRuntimeConfig(layout, c.baseConfigPath, codexHostedConfigAsset, c.model)
}

// writeConfig renders the runtime Codex config for local Ollama-backed OSS usage.
func (c codexOllamaAdapter) writeConfig(layout *demoLayout) error {
	return writeCodexRuntimeConfig(layout, c.baseConfigPath, codexOllamaConfigAsset, "")
}

// writeCodexRuntimeConfig loads a base config, patches MCP/project settings, and writes CODEX_HOME.
func writeCodexRuntimeConfig(layout *demoLayout, baseConfigPath, defaultAssetName, model string) error {
	content, err := loadCodexConfigTemplate(baseConfigPath, defaultAssetName)
	if err != nil {
		return err
	}
	content = setCodexRootModel(content, model)
	content = patchCodexRuntimeConfig(content, layout.MCPURL, layout.WorkspacePath)
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

// loadCodexConfigTemplate loads either the caller-supplied Codex config or an embedded default.
func loadCodexConfigTemplate(baseConfigPath, defaultAssetName string) (string, error) {
	if path := strings.TrimSpace(baseConfigPath); path != "" {
		data, err := readCodexConfigFile(path)
		if err != nil {
			return "", fmt.Errorf("read codex config %q: %w", path, err)
		}
		return strings.ReplaceAll(string(data), "__MODEL_BLOCK__", ""), nil
	}
	content, err := asset(defaultAssetName)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(content, "__MODEL_BLOCK__", ""), nil
}

// patchCodexRuntimeConfig injects the demo-local MCP server and trusted project block.
func patchCodexRuntimeConfig(content, mcpURL, workDir string) string {
	content = strings.TrimRight(content, "\n")
	content = removeCodexTable(content, "[mcp_servers.centian]")
	content = removeCodexTable(content, `[projects.`+strconv.Quote(workDir)+`]`)

	runtimeBlock := strings.TrimSpace(fmt.Sprintf(`
[mcp_servers.centian]
url = %q
enabled = true
destructive_enabled = false
open_world_enabled = false
default_tools_approval_mode = "auto"

[projects.%s]
trust_level = "trusted"
destructive_enabled = false
open_world_enabled = false
default_tools_approval_mode = "auto"
`, mcpURL, strconv.Quote(workDir)))

	if content == "" {
		return runtimeBlock + "\n"
	}
	return content + "\n\n" + runtimeBlock + "\n"
}

// removeCodexTable removes one TOML table so runtime settings can replace it deterministically.
func removeCodexTable(content, header string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if skip && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			skip = false
		}
		if !skip && trimmed == header {
			skip = true
			continue
		}
		if skip {
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// setCodexRootModel writes or replaces the top-level model field when a hosted model override is set.
func setCodexRootModel(content, model string) string {
	content = strings.TrimRight(content, "\n")
	model = strings.TrimSpace(model)
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines)+1)
	inTopLevel := true
	modelWritten := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inTopLevel && model != "" && !modelWritten {
				result = append(result, `model = `+strconv.Quote(model))
				modelWritten = true
			}
			inTopLevel = false
		}
		if inTopLevel && strings.HasPrefix(trimmed, "model =") {
			if model == "" {
				result = append(result, line)
				continue
			}
			if !modelWritten {
				result = append(result, `model = `+strconv.Quote(model))
				modelWritten = true
			}
			continue
		}
		result = append(result, line)
	}
	if inTopLevel && model != "" && !modelWritten {
		result = append([]string{`model = ` + strconv.Quote(model)}, result...)
	}
	return strings.TrimSpace(strings.Join(result, "\n")) + "\n"
}

// copyCodexAuth mirrors the user's Codex auth.json into the demo-specific CODEX_HOME if present.
func copyCodexAuth(codexHome string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home dir: %w", err)
	}
	sourcePath := filepath.Join(homeDir, ".codex", "auth.json")
	//nolint:gosec // sourcePath is the fixed auth file under the current user's Codex home.
	authData, err := os.ReadFile(sourcePath)
	switch {
	case err == nil:
		targetPath := filepath.Join(codexHome, "auth.json")
		//nolint:gosec // targetPath is the fixed auth file inside the demo-specific CODEX_HOME.
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

// env points Codex at the demo-specific CODEX_HOME.
func (codexAdapter) env(layout *demoLayout) []string {
	return []string{"CODEX_HOME=" + filepath.Dir(layout.CodexConfig)}
}

// env points Codex OSS mode at the demo-specific CODEX_HOME.
func (codexOllamaAdapter) env(layout *demoLayout) []string {
	return []string{"CODEX_HOME=" + filepath.Dir(layout.CodexConfig)}
}

// cleanup removes the demo-specific CODEX_HOME tree after the run.
func (codexAdapter) cleanup(layout *demoLayout) error {
	return os.RemoveAll(filepath.Dir(layout.CodexConfig))
}

// cleanup removes the demo-specific CODEX_HOME tree after the run.
func (codexOllamaAdapter) cleanup(layout *demoLayout) error {
	return os.RemoveAll(filepath.Dir(layout.CodexConfig))
}

// command builds the non-interactive hosted Codex CLI invocation for one run.
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

// command builds the non-interactive Codex OSS invocation for one Ollama-backed run.
func (c codexOllamaAdapter) command(layout *demoLayout, _ string) ([]string, error) {
	profile, err := resolveCodexOllamaProfile(c.model, c.baseConfigPath)
	if err != nil {
		return nil, err
	}
	return []string{
		"codex",
		"exec",
		"--skip-git-repo-check",
		"--json",
		"-C", layout.WorkspacePath,
		"-o", filepath.Join(layout.RootPath, "codex_output.txt"),
		"--oss", // TODO: ollama-cloud models would require NOT to send this parameter instead
		"--profile",
		profile,
		"-",
	}, nil
}

// resolveCodexOllamaProfile chooses the Codex profile name to pass to `codex exec --profile`.
func resolveCodexOllamaProfile(model, baseConfigPath string) (string, error) {
	if profile, ok := codexOllamaProfileAlias(model); ok {
		return profile, nil
	}
	if profile := strings.TrimSpace(model); profile != "" {
		return profile, nil
	}
	if path := strings.TrimSpace(baseConfigPath); path != "" {
		profile, err := firstCodexProfileFromFile(path)
		if err != nil {
			return "", err
		}
		if profile != "" {
			return profile, nil
		}
	}
	return codexOllamaQwenProfile, nil
}

// selectedCodexOllamaModelLabel returns the persisted label describing the selected Codex OSS profile.
func selectedCodexOllamaModelLabel(model, baseConfigPath string) string {
	if value := strings.TrimSpace(model); value != "" {
		return value
	}
	if path := strings.TrimSpace(baseConfigPath); path != "" {
		profile, err := firstCodexProfileFromFile(path)
		if err == nil {
			return profile
		}
	}
	return DefaultCodexOllamaModel
}

// codexOllamaProfileAlias maps supported shorthand model aliases to embedded profile names.
func codexOllamaProfileAlias(model string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case codexOllamaGemmaModel:
		return codexOllamaGemmaProfile, true
	case codexOllamaQwenModel:
		return codexOllamaQwenProfile, true
	default:
		return "", false
	}
}

// firstCodexProfileFromFile returns the first profile name declared in a Codex config file.
func firstCodexProfileFromFile(path string) (string, error) {
	data, err := readCodexConfigFile(path)
	if err != nil {
		return "", fmt.Errorf("read codex config %q: %w", path, err)
	}
	return firstCodexProfileName(string(data)), nil
}

// readCodexConfigFile reads a caller-supplied Codex config file from disk.
func readCodexConfigFile(path string) ([]byte, error) {
	//nolint:gosec // The CLI intentionally reads a user-selected local Codex config path.
	return os.ReadFile(path)
}

// firstCodexProfileName extracts the first `[profiles.*]` table name from TOML-like content.
func firstCodexProfileName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[profiles.") || !strings.HasSuffix(trimmed, "]") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "[profiles."), "]")
		name = strings.TrimSpace(strings.Trim(name, `"`))
		if name != "" {
			return name
		}
	}
	return ""
}
