package discovery

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestShowDiscoveryResults_NoServers(t *testing.T) {
	// Given: a result with no servers
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader(""))}
	result := &Result{Servers: []Server{}, Errors: []string{}}

	// When: showing results
	servers, err := ui.ShowDiscoveryResults(result)

	// Then: no servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestShowDiscoveryResults_WithServers(t *testing.T) {
	// Given: a result with servers and default import choice
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("a\n"))}
	result := &Result{Servers: []Server{{Name: "one", SourcePath: "/tmp/config.json"}}, Errors: []string{}}

	// When: showing results
	servers, err := ui.ShowDiscoveryResults(result)

	// Then: all servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
}

func TestPromptForImport_All(t *testing.T) {
	// Given: servers and "all" input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("a\n"))}
	servers := []Server{{Name: "one"}, {Name: "two"}}

	// When: prompting for import
	selected, err := ui.promptForImport(servers)

	// Then: all servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 2)
}

func TestPromptForImport_None(t *testing.T) {
	// Given: servers and "none" input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("n\n"))}
	servers := []Server{{Name: "one"}}

	// When: prompting for import
	selected, err := ui.promptForImport(servers)

	// Then: no servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 0)
}

func TestSelectServers(t *testing.T) {
	// Given: a list of servers and selection input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("1,3\n"))}
	servers := []Server{{Name: "one"}, {Name: "two"}, {Name: "three"}}

	// When: selecting servers
	selected, err := ui.selectServers(servers)

	// Then: the selected servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 2)
	assert.Equal(t, selected[0].Name, "one")
	assert.Equal(t, selected[1].Name, "three")
}

func TestSelectServers_InvalidSelection(t *testing.T) {
	// Given: invalid selection input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("invalid,5\n"))}
	servers := []Server{{Name: "one"}, {Name: "two"}}

	// When: selecting servers
	selected, err := ui.selectServers(servers)

	// Then: no servers are selected
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 0)
}

func TestPromptForReplacement(t *testing.T) {
	// Given: servers and confirmation input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("y\n"))}
	servers := []Server{{Name: "one"}, {Name: "two"}}

	// When: prompting for replacement
	selected, err := ui.promptForReplacement(servers)

	// Then: servers are marked for replacement
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 2)
	assert.Assert(t, selected[0].ReplacementMode)
	assert.Assert(t, selected[1].ReplacementMode)
}

func TestPromptForReplacement_Cancel(t *testing.T) {
	// Given: servers and cancel input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("n\n"))}
	servers := []Server{{Name: "one"}}

	// When: prompting for replacement
	selected, err := ui.promptForReplacement(servers)

	// Then: no servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 0)
}

func TestSelectAndReplace(t *testing.T) {
	// Given: servers from a single source path and selection input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("1\ny\n"))}
	servers := []Server{
		{Name: "one", SourcePath: "/tmp/config.json"},
		{Name: "two", SourcePath: "/tmp/config.json"},
	}

	// When: selecting configs to replace
	selected, err := ui.selectAndReplace(servers)

	// Then: all servers are marked for replacement
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 2)
	assert.Assert(t, selected[0].ReplacementMode)
	assert.Assert(t, selected[1].ReplacementMode)
}

func TestSelectAndReplace_Cancel(t *testing.T) {
	// Given: servers and cancel input
	ui := &UserInterface{reader: bufio.NewReader(strings.NewReader("1\nn\n"))}
	servers := []Server{
		{Name: "one", SourcePath: "/tmp/config.json"},
	}

	// When: selecting configs to replace
	selected, err := ui.selectAndReplace(servers)

	// Then: replacement mode is cleared
	assert.NilError(t, err)
	assert.Equal(t, len(selected), 1)
	assert.Assert(t, !selected[0].ReplacementMode)
}

func TestGenerateReplacementConfig(t *testing.T) {
	// Given: servers with known and unknown source paths
	claude := Server{Name: "a", SourcePath: filepath.Join("/tmp", ClaudeDesktopConfigFile)}
	unknown := Server{Name: "b", SourcePath: "/tmp/other.json"}

	// When: generating replacement configs
	claudeConfig := generateReplacementConfig(claude)
	unknownConfig := generateReplacementConfig(unknown)

	// Then: source types are detected
	assert.Equal(t, claudeConfig.SourceType, SourceTypeClaudeDesktop)
	assert.Equal(t, unknownConfig.SourceType, SourceTypeGenericMCP)
	assert.Assert(t, strings.Contains(claudeConfig.ProxyConfig, "centian"))
}

func TestUpdateConfigFile_ClaudeDesktop(t *testing.T) {
	// Given: a Claude Desktop config file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, ClaudeDesktopConfigFile)
	original := map[string]interface{}{
		ConfigKeyMCPServers: map[string]interface{}{
			"server": map[string]interface{}{"command": "node"},
		},
	}
	data, err := json.Marshal(original)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filePath, data, 0o644))

	// When: updating config file
	err = updateConfigFile(filePath, SourceTypeClaudeDesktop)

	// Then: file is updated and backup created
	assert.NilError(t, err)
	_, statErr := os.Stat(filePath + BackupFileSuffix)
	assert.NilError(t, statErr)

	updated := readJSONFile(t, filePath)
	servers := updated[ConfigKeyMCPServers].(map[string]interface{})
	assert.Assert(t, servers[CentianCommand] != nil)
}

func TestUpdateConfigFile_VSCodeMCP(t *testing.T) {
	// Given: a VS Code MCP config file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mcp.json")
	original := map[string]interface{}{
		ConfigKeyServers: map[string]interface{}{
			"server": map[string]interface{}{"command": "node"},
		},
	}
	data, err := json.Marshal(original)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filePath, data, 0o644))

	// When: updating config file
	err = updateConfigFile(filePath, SourceTypeVSCodeMCP)

	// Then: servers are replaced
	assert.NilError(t, err)
	updated := readJSONFile(t, filePath)
	servers := updated[ConfigKeyServers].(map[string]interface{})
	assert.Equal(t, len(servers), 1)
	assert.Assert(t, servers[CentianCommand] != nil)
}

func TestUpdateConfigFile_VSCodeSettings(t *testing.T) {
	// Given: a VS Code settings file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "settings.json")
	original := map[string]interface{}{
		ConfigKeyMCPServersPath: map[string]interface{}{
			"server": map[string]interface{}{"command": "node"},
		},
	}
	data, err := json.Marshal(original)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filePath, data, 0o644))

	// When: updating config file
	err = updateConfigFile(filePath, SourceTypeVSCodeSettings)

	// Then: mcp.servers are replaced
	assert.NilError(t, err)
	updated := readJSONFile(t, filePath)
	servers := updated[ConfigKeyMCPServersPath].(map[string]interface{})
	assert.Equal(t, len(servers), 1)
	assert.Assert(t, servers[CentianCommand] != nil)
}

func TestUpdateConfigFile_Unsupported(t *testing.T) {
	// Given: a config file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{}`), 0o644))

	// When: updating with unsupported type
	err := updateConfigFile(filePath, "unknown")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestApplyReplacementConfigs(t *testing.T) {
	// Given: a config file and replacement config
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, ClaudeDesktopConfigFile)
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"mcpServers":{"server":{"command":"node"}}}`), 0o644))

	configs := []ReplacementConfig{{
		SourcePath: filePath,
		SourceType: SourceTypeClaudeDesktop,
		OriginalServers: []string{
			"server",
		},
		ProxyConfig: generateClaudeDesktopReplacement(),
	}}

	// When: applying replacement configs
	applyReplacementConfigs(configs)

	// Then: config is updated
	updated := readJSONFile(t, filePath)
	servers := updated[ConfigKeyMCPServers].(map[string]interface{})
	assert.Assert(t, servers[CentianCommand] != nil)
}

func TestImportServers(t *testing.T) {
	// Given: servers including an invalid one
	servers := []Server{
		{Name: "one", Command: "node", SourcePath: "/tmp/one"},
		{Name: "two", URL: "https://example.com", SourcePath: "/tmp/two"},
		{Name: "bad", SourcePath: "/tmp/bad"},
	}

	// When: importing servers
	count := ImportServers(servers)

	// Then: only valid servers are imported
	assert.Equal(t, count, 2)
}

func TestShowImportSummary(t *testing.T) {
	// Given: zero and non-zero imports
	ShowImportSummary(0)
	ShowImportSummary(2)
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NilError(t, err)
	var decoded map[string]interface{}
	assert.NilError(t, json.Unmarshal(data, &decoded))
	return decoded
}
