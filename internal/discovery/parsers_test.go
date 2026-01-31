package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestDetectSourceType(t *testing.T) {
	// Given: file paths for known sources
	cases := []struct {
		path     string
		expected string
	}{
		{"/tmp/claude_desktop_config.json", "claude-desktop"},
		{"/repo/.vscode/mcp.json", "vscode-mcp"},
		{"/repo/.vscode/settings.json", "vscode-settings"},
		{"/home/.claude/config.json", "claude-code"},
		{"/home/.continue/config.json", "continue-dev"},
		{"/home/other.json", "generic"},
	}

	for _, testCase := range cases {
		// When: detecting the source type
		result := detectSourceType(testCase.path)

		// Then: it matches expectations
		assert.Equal(t, result, testCase.expected)
	}
}

func TestGetParser(t *testing.T) {
	// Given: known parser names
	known := []string{
		"detectAndParse",
		"claudeDesktopParser",
		"vscodeParser",
		"settingsParser",
		"genericParser",
	}

	for _, name := range known {
		// When: getting parser
		parser := getParser(name)

		// Then: parser exists
		assert.Assert(t, parser != nil)
	}

	// Given: an unknown parser name
	parser := getParser("missing")

	// Then: parser is nil
	assert.Assert(t, parser == nil)
}

func TestParseClaudeDesktopConfig(t *testing.T) {
	// Given: Claude Desktop config with a valid and invalid server
	data := []byte(`{"mcpServers":{"good":{"command":"node","args":["a"],"env":{"A":"B"}},"bad":{"command":""}}}`)

	// When: parsing the config
	servers, err := parseClaudeDesktopConfig(data, "/tmp/claude_desktop_config.json")

	// Then: only valid servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Name, "good")
	assert.Equal(t, servers[0].Transport, "stdio")
	assert.Equal(t, servers[0].Source, "Claude Desktop")
}

func TestParseVSCodeConfig(t *testing.T) {
	// Given: VS Code config with stdio, http, and invalid servers
	data := []byte(`{"servers":{
  "stdio":{"type":"stdio","command":"node","args":["-v"],"env":{"A":"B"}},
  "http":{"type":"http","url":"https://example.com","headers":{"X":"Y"}},
  "invalid-type":{"type":"other"},
  "invalid-stdio":{"type":"stdio"},
  "invalid-http":{"type":"http"}
}}`)

	// When: parsing the config
	servers, err := parseVSCodeConfig(data, "/tmp/mcp.json")

	// Then: only valid servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 2)
	assert.Equal(t, servers[0].Source, "VS Code MCP")
	assert.Equal(t, servers[1].Source, "VS Code MCP")
}

func TestParseSettingsConfig(t *testing.T) {
	// Given: VS Code settings config
	data := []byte(`{"mcp.servers":{"server-one":{"command":"node","args":["-v"],"env":{"A":"B"}},"server-two":{"url":"https://example.com"}}}`)

	// When: parsing the config
	servers, err := parseSettingsConfig(data, "/tmp/settings.json")

	// Then: servers are extracted
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 2)
	assert.Equal(t, servers[0].Source, "VS Code Settings")

	var foundHTTP bool
	var foundStdio bool
	for _, server := range servers {
		switch server.Name {
		case "server-one":
			assert.Equal(t, server.Transport, "stdio")
			foundStdio = true
		case "server-two":
			assert.Equal(t, server.Transport, "http")
			foundHTTP = true
		}
	}
	assert.Assert(t, foundStdio)
	assert.Assert(t, foundHTTP)
}

func TestParseGenericConfig(t *testing.T) {
	// Given: a generic config with various keys
	data := []byte(`{
  "servers": {"one": {"command": "node", "args": ["-v"]}},
  "mcp": {"two": {"url": "https://example.com"}},
  "tools": {"bad": {"name": "skip"}}
}`)

	// When: parsing the config
	servers, err := parseGenericConfig(data, "/tmp/config.json")

	// Then: servers are extracted
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 2)
	assert.Equal(t, servers[0].Source, "Generic Config")
}

func TestDetectAndParseConfig(t *testing.T) {
	// Given: inputs for multiple formats
	claude := []byte(`{"mcpServers":{"a":{"command":"node"}}}`)
	vscode := []byte(`{"servers":{"a":{"type":"stdio","command":"node"}}}`)
	settings := []byte(`{"mcp.servers":{"a":{"command":"node"}}}`)
	generic := []byte(`{"mcpServers":{"a":{"command":"node"}}}`)

	// When: detecting and parsing
	claudeServers, err := detectAndParseConfig(claude, "/tmp/claude.json")
	assert.NilError(t, err)
	vscodeServers, err := detectAndParseConfig(vscode, "/tmp/mcp.json")
	assert.NilError(t, err)
	settingsServers, err := detectAndParseConfig(settings, "/tmp/settings.json")
	assert.NilError(t, err)
	genericServers, err := detectAndParseConfig(generic, "/tmp/config.json")
	assert.NilError(t, err)

	// Then: each format yields servers
	assert.Equal(t, len(claudeServers), 1)
	assert.Equal(t, len(vscodeServers), 1)
	assert.Equal(t, len(settingsServers), 1)
	assert.Equal(t, len(genericServers), 1)
}

func TestParseConfigFile(t *testing.T) {
	// Given: an MCP servers config
	data := []byte(`{"mcpServers":{"a":{"command":"node"}}}`)

	// When: parsing using ParseConfigFile
	servers, err := ParseConfigFile(data, "/tmp/claude.json")

	// Then: servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
}

func TestExtractServerFromSettings(t *testing.T) {
	// Given: settings server map with command and env
	info := map[string]interface{}{
		"command": "node",
		"args":    []interface{}{"-v", 42},
		"env":     map[string]interface{}{"A": "B", "C": 1},
	}

	// When: extracting server
	server := extractServerFromSettings("demo", info, "/tmp/settings.json")

	// Then: server fields are extracted
	assert.Assert(t, server != nil)
	assert.Equal(t, server.Name, "demo")
	assert.Equal(t, server.Command, "node")
	assert.Equal(t, len(server.Args), 1)
	assert.Equal(t, server.Env["A"], "B")
}

func TestExtractServerFromGeneric(t *testing.T) {
	// Given: generic server maps
	withCommand := map[string]interface{}{
		"cmd":  "node",
		"args": []interface{}{"-v"},
	}
	withURL := map[string]interface{}{
		"url": "https://example.com",
	}
	invalid := map[string]interface{}{
		"name": "skip",
	}

	// When: extracting servers
	serverCommand := extractServerFromGeneric("cmd", withCommand, "/tmp/config.json", "servers")
	serverURL := extractServerFromGeneric("url", withURL, "/tmp/config.json", "mcp")
	serverInvalid := extractServerFromGeneric("bad", invalid, "/tmp/config.json", "tools")

	// Then: only valid servers are returned
	assert.Assert(t, serverCommand != nil)
	assert.Assert(t, serverURL != nil)
	assert.Assert(t, serverInvalid == nil)
	assert.Equal(t, serverURL.Transport, "http")
}

func TestRegexDiscovererMatchesPattern(t *testing.T) {
	// Given: a temp file and pattern
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{
		FileRegex:    `config\.json$`,
		ContentRegex: []string{"\"servers\""},
	}

	// When: matching pattern
	matches, err := r.matchesPattern(filePath, pattern)

	// Then: matches
	assert.NilError(t, err)
	assert.Assert(t, matches)

	// Given: an invalid file regex
	pattern.FileRegex = "["

	// When: matching pattern
	_, err = r.matchesPattern(filePath, pattern)

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestRegexDiscovererMatchesContentRegex(t *testing.T) {
	// Given: a file with content
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{}
	patterns := []string{"[", "servers"}

	// When: matching content regex
	matches, err := r.matchesContentRegex(filePath, patterns)

	// Then: valid pattern matches
	assert.NilError(t, err)
	assert.Assert(t, matches)
}

func TestRegexDiscovererParseFile(t *testing.T) {
	// Given: a VS Code MCP config file and auto-detect pattern
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{
		Parser:     "vscodeParser",
		SourceType: "auto-detect",
	}

	// When: parsing file
	servers, err := r.parseFile(filePath, pattern)

	// Then: source is auto-detected
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Source, "generic")
}

func TestRegexDiscovererProcessFile(t *testing.T) {
	// Given: a file matching multiple patterns
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{Patterns: []Pattern{
		{
			FileRegex:    `mcp\.json$`,
			ContentRegex: []string{"servers"},
			Parser:       "genericParser",
			Priority:     1,
		},
		{
			FileRegex:    `mcp\.json$`,
			ContentRegex: []string{"servers"},
			Parser:       "vscodeParser",
			Priority:     10,
		},
	}}

	// When: processing file
	servers, err := r.processFile(filePath)

	// Then: highest priority parser wins
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Source, "VS Code MCP")
}

func TestRegexDiscovererScanPath(t *testing.T) {
	// Given: a directory with a config file
	tmpDir := createSearchRoot(t)
	filePath := filepath.Join(tmpDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{Patterns: []Pattern{
		{
			FileRegex:    `mcp\.json$`,
			ContentRegex: []string{"servers"},
			Parser:       "vscodeParser",
			Priority:     1,
		},
	}, MaxDepth: 2}

	// When: scanning the directory
	servers, err := r.scanPath(tmpDir, 0)

	// Then: servers are found
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
}

func TestRegexDiscovererDiscover(t *testing.T) {
	// Given: a discoverer with a temp search path
	tmpDir := createSearchRoot(t)
	filePath := filepath.Join(tmpDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{
		Patterns: []Pattern{{
			FileRegex:    `mcp\.json$`,
			ContentRegex: []string{"servers"},
			Parser:       "vscodeParser",
			Priority:     1,
		}},
		SearchPaths: []string{tmpDir},
		MaxDepth:    2,
		Enabled:     true,
	}

	// When: discovering
	servers, err := r.Discover()

	// Then: servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)

	// Given: discoverer disabled
	r.Enabled = false

	// When: discovering
	servers, err = r.Discover()

	// Then: no servers
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestExpandPathAndEnsureAbsolutePath(t *testing.T) {
	// Given: a temp home directory and env var
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("TEST_PATH", filepath.Join(tempHome, "env"))

	// When: expanding paths
	expandedHome := expandPath("~/config.json")
	expandedEnv := expandPath("$TEST_PATH/config.json")

	// Then: paths are expanded
	assert.Equal(t, expandedHome, filepath.Join(tempHome, "config.json"))
	assert.Equal(t, expandedEnv, filepath.Join(tempHome, "env", "config.json"))

	// When: ensuring absolute path
	abs := ensureAbsolutePath("relative/config.json")

	// Then: path is absolute
	assert.Assert(t, filepath.IsAbs(abs))
}

func TestShouldSkipDirectory(t *testing.T) {
	// Given: directories that should be skipped
	baseDir := createSearchRoot(t)
	assert.Assert(t, shouldSkipDirectory(filepath.Join(baseDir, "node_modules")))
	assert.Assert(t, shouldSkipDirectory(filepath.Join(baseDir, ".git")))

	// Then: normal directories are not skipped
	assert.Assert(t, !shouldSkipDirectory(filepath.Join(baseDir, "project")))
}

func createSearchRoot(t *testing.T) string {
	t.Helper()
	candidates := []string{}
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		candidates = append(candidates, homeDir)
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		candidates = append(candidates, cwd)
	}

	for _, baseDir := range candidates {
		if shouldSkipDirectory(baseDir) {
			continue
		}
		dir, err := os.MkdirTemp(baseDir, "discovery-test-")
		if err != nil {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err == nil {
			dir = absDir
		}
		if shouldSkipDirectory(dir) || shouldSkipDirectory(filepath.Join(dir, "project")) {
			_ = os.RemoveAll(dir)
			continue
		}
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		return dir
	}

	t.Skip("unable to create discovery test directory outside excluded paths")
	return ""
}

func TestVSCodeDiscovererParsing(t *testing.T) {
	// Given: VS Code discoverer and configs
	d := &VSCodeDiscoverer{}
	mcpData := []byte(`{"servers":{"demo":{"command":"node","args":["-v"],"env":{"A":"B"},"url":"https://example.com"}}}`)
	settingsData := []byte(`{"mcp.servers":{"demo":{"command":"node"}}}`)

	// When: parsing mcp.json
	mcpServers, err := d.parseMCPJson(mcpData, "/tmp/mcp.json")
	assert.NilError(t, err)

	// Then: servers are parsed
	assert.Equal(t, len(mcpServers), 1)
	assert.Equal(t, mcpServers[0].Source, "VS Code MCP")

	// When: parsing settings.json
	settingsServers, err := d.parseSettingsJSON(settingsData, "/tmp/settings.json")
	assert.NilError(t, err)

	// Then: servers are parsed
	assert.Equal(t, len(settingsServers), 1)
	assert.Equal(t, settingsServers[0].Source, "VS Code Settings")
}

func TestClaudeDesktopDiscoverer(t *testing.T) {
	// Given: a Claude desktop discoverer
	d := &ClaudeDesktopDiscoverer{}

	switch runtime.GOOS {
	case darwin:
		// Given: a temp home with a config file
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)
		configPath := filepath.Join(tempHome, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		assert.NilError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
		assert.NilError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{"demo":{"command":"node"}}}`), 0o644))

		// When: discovering
		servers, err := d.Discover()

		// Then: servers are returned
		assert.NilError(t, err)
		assert.Equal(t, len(servers), 1)
	case windows:
		// Given: a temp appdata with a config file
		tempAppData := t.TempDir()
		t.Setenv("APPDATA", tempAppData)
		configPath := filepath.Join(tempAppData, "Claude", "claude_desktop_config.json")
		assert.NilError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
		assert.NilError(t, os.WriteFile(configPath, []byte(`{"mcpServers":{"demo":{"command":"node"}}}`), 0o644))

		// When: discovering
		servers, err := d.Discover()

		// Then: servers are returned
		assert.NilError(t, err)
		assert.Equal(t, len(servers), 1)
	default:
		// When: discovering on unsupported platform
		_, err := d.Discover()

		// Then: error is returned
		assert.Assert(t, err != nil)
	}
}

func TestVSCodeDiscovererScanPath(t *testing.T) {
	// Given: a temp VS Code config path
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "settings.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"mcp.servers":{"demo":{"command":"node"}}}`), 0o644))

	d := &VSCodeDiscoverer{}

	// When: scanning the path
	servers, err := d.scanPath(filePath)

	// Then: servers are returned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
}

func TestVSCodeDiscovererExtractServerFromMap(t *testing.T) {
	// Given: server info with args and env
	info := map[string]interface{}{
		"command": "node",
		"args":    []interface{}{"-v"},
		"env":     map[string]interface{}{"A": "B"},
	}
	v := &VSCodeDiscoverer{}

	// When: extracting server
	server := v.extractServerFromMap("demo", info, "/tmp/settings.json")

	// Then: server is built
	assert.Assert(t, server != nil)
	assert.Equal(t, server.Command, "node")
	assert.Equal(t, server.Env["A"], "B")
}

func TestParseSettingsConfig_InvalidJSON(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")

	// When: parsing settings
	_, err := parseSettingsConfig(data, "/tmp/settings.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestParseGenericConfig_Invalid(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")

	// When: parsing generic config
	_, err := parseGenericConfig(data, "/tmp/config.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestRegexDiscovererParseFile_UnknownParser(t *testing.T) {
	// Given: a temp file and unknown parser
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{Parser: "missing"}

	// When: parsing the file
	_, err := r.parseFile(filePath, pattern)

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestRegexDiscovererProcessFile_NoMatch(t *testing.T) {
	// Given: a file that doesn't match patterns
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{Patterns: []Pattern{{
		FileRegex:    `other\.json$`,
		ContentRegex: []string{"servers"},
		Parser:       "genericParser",
		Priority:     1,
	}}}

	// When: processing file
	servers, err := r.processFile(filePath)

	// Then: no servers
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestRegexDiscovererMatchesContentRegex_NoMatch(t *testing.T) {
	// Given: a file with content
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{}
	patterns := []string{"missing"}

	// When: matching content regex
	matches, err := r.matchesContentRegex(filePath, patterns)

	// Then: no match
	assert.NilError(t, err)
	assert.Assert(t, !matches)
}

func TestParseVSCodeConfig_InvalidJSON(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")

	// When: parsing
	_, err := parseVSCodeConfig(data, "/tmp/mcp.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestParseClaudeDesktopConfig_InvalidJSON(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")

	// When: parsing
	_, err := parseClaudeDesktopConfig(data, "/tmp/claude.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestDetectAndParseConfig_InvalidJSON(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")

	// When: detecting and parsing
	_, err := detectAndParseConfig(data, "/tmp/config.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestParseFile_AutoDetectSourceType(t *testing.T) {
	// Given: a file path used for auto-detect
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "settings.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"mcp.servers":{"demo":{"command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{Parser: "settingsParser", SourceType: "auto-detect"}

	// When: parsing
	servers, err := r.parseFile(filePath, pattern)

	// Then: source is auto-detected
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Source, "vscode-settings")
}

func TestParseSettingsConfig_Empty(t *testing.T) {
	// Given: settings JSON without mcp.servers
	data := []byte(`{"other":{}}`)

	// When: parsing
	servers, err := parseSettingsConfig(data, "/tmp/settings.json")

	// Then: no servers
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestParseGenericConfig_Empty(t *testing.T) {
	// Given: JSON without known keys
	data := []byte(`{"other":{}}`)

	// When: parsing
	servers, err := parseGenericConfig(data, "/tmp/config.json")

	// Then: no servers
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestRegexDiscovererScanPath_MaxDepth(t *testing.T) {
	// Given: a deep directory with a config file
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "level1", "level2")
	assert.NilError(t, os.MkdirAll(nestedDir, 0o755))
	filePath := filepath.Join(nestedDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{Patterns: []Pattern{{
		FileRegex:    `mcp\.json$`,
		ContentRegex: []string{"servers"},
		Parser:       "vscodeParser",
		Priority:     1,
	}}, MaxDepth: 0}

	// When: scanning at max depth 0
	servers, err := r.scanPath(tmpDir, 0)

	// Then: nested files are not scanned
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestVSCodeDiscovererGetSearchPaths(t *testing.T) {
	// Given: VS Code discoverer
	d := &VSCodeDiscoverer{}

	// When: getting search paths
	paths := d.getSearchPaths()

	// Then: paths include workspace .vscode settings
	assert.Assert(t, containsPath(paths, ".vscode"))
	assert.Assert(t, containsPath(paths, "settings.json"))
}

func TestParseMCPJson_Invalid(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")
	v := &VSCodeDiscoverer{}

	// When: parsing
	_, err := v.parseMCPJson(data, "/tmp/mcp.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestVSCodeParseSettingsJSON_Invalid(t *testing.T) {
	// Given: invalid JSON
	data := []byte("{invalid")
	v := &VSCodeDiscoverer{}

	// When: parsing
	_, err := v.parseSettingsJSON(data, "/tmp/settings.json")

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestParseSettingsJSON_Empty(t *testing.T) {
	// Given: empty JSON
	data := []byte(`{"other":{}}`)
	v := &VSCodeDiscoverer{}

	// When: parsing
	servers, err := v.parseSettingsJSON(data, "/tmp/settings.json")

	// Then: no servers
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 0)
}

func TestRegexDiscovererMatchesPattern_NoContentRegex(t *testing.T) {
	// Given: a file and pattern without content regex
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{FileRegex: `config\.json$`}

	// When: matching pattern
	matches, err := r.matchesPattern(filePath, pattern)

	// Then: file regex match is sufficient
	assert.NilError(t, err)
	assert.Assert(t, matches)
}

func TestParseFile_SetsSourceForAutoDetect(t *testing.T) {
	// Given: a file with auto-detect source type
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "claude_desktop_config.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"mcpServers":{"demo":{"command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{Parser: "claudeDesktopParser", SourceType: "auto-detect"}

	// When: parsing
	servers, err := r.parseFile(filePath, pattern)

	// Then: source is set
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Source, "claude-desktop")
}

func TestParseGenericConfig_ArgsExtraction(t *testing.T) {
	// Given: generic config with args key
	config := map[string]interface{}{
		"servers": map[string]interface{}{
			"demo": map[string]interface{}{
				"command":   "node",
				"arguments": []interface{}{"-v"},
			},
		},
	}
	data, err := json.Marshal(config)
	assert.NilError(t, err)

	// When: parsing
	servers, err := parseGenericConfig(data, "/tmp/config.json")

	// Then: args are extracted
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, len(servers[0].Args), 1)
}

func TestParseSettingsConfig_ArgsExtraction(t *testing.T) {
	// Given: settings config with args array
	config := map[string]interface{}{
		"mcp.servers": map[string]interface{}{
			"demo": map[string]interface{}{
				"command": "node",
				"args":    []interface{}{"-v"},
			},
		},
	}
	data, err := json.Marshal(config)
	assert.NilError(t, err)

	// When: parsing
	servers, err := parseSettingsConfig(data, "/tmp/settings.json")

	// Then: args are extracted
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, len(servers[0].Args), 1)
}

func TestRegexDiscovererMatchesContentRegex_ReadError(t *testing.T) {
	// Given: a missing file
	r := &RegexDiscoverer{}

	// When: matching content regex
	_, err := r.matchesContentRegex("/missing/file.json", []string{"servers"})

	// Then: error is returned
	assert.Assert(t, err != nil)
}

func TestParseFile_SourceTypeNotAutoDetect(t *testing.T) {
	// Given: a file and pattern with explicit source type
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mcp.json")
	assert.NilError(t, os.WriteFile(filePath, []byte(`{"servers":{"demo":{"type":"stdio","command":"node"}}}`), 0o644))

	r := &RegexDiscoverer{}
	pattern := Pattern{Parser: "vscodeParser", SourceType: "manual"}

	// When: parsing
	servers, err := r.parseFile(filePath, pattern)

	// Then: source stays as parser default
	assert.NilError(t, err)
	assert.Equal(t, len(servers), 1)
	assert.Equal(t, servers[0].Source, "VS Code MCP")
}

func containsPath(paths []string, part string) bool {
	for _, p := range paths {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}
