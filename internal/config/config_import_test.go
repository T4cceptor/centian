package config

import (
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestParseImportedConfigFile(t *testing.T) {
	t.Run("claude desktop", func(t *testing.T) {
		data := []byte(`{"mcpServers":{"good":{"command":"node","args":["a"],"env":{"A":"B"}},"bad":{"command":""}}}`)

		servers, err := ParseImportedConfigFile(data, "/tmp/claude_desktop_config.json")

		assert.NilError(t, err)
		assert.Equal(t, len(servers), 1)
		assert.Equal(t, servers[0].Name, "good")
		assert.Equal(t, servers[0].Transport, "stdio")
		assert.Equal(t, servers[0].Source, "Claude Desktop")
	})

	t.Run("vscode mcp json", func(t *testing.T) {
		data := []byte(`{"servers":{
  "stdio":{"type":"stdio","command":"node","args":["-v"],"env":{"A":"B"}},
  "http":{"type":"http","url":"https://example.com","headers":{"X":"Y"}},
  "invalid-both":{"command":"node","url":"https://example.com"},
  "invalid-empty":{}
}}`)

		servers, err := ParseImportedConfigFile(data, "/tmp/mcp.json")

		assert.NilError(t, err)
		assert.Equal(t, len(servers), 2)
		assert.Equal(t, servers[0].Source, "VS Code MCP")
		assert.Equal(t, servers[1].Source, "VS Code MCP")
	})

	t.Run("vscode settings", func(t *testing.T) {
		data := []byte(`{"mcp.servers":{"server-one":{"command":"node","args":["-v"],"env":{"A":"B"}},"server-two":{"url":"https://example.com","headers":{"X":"Y"}}}}`)

		servers, err := ParseImportedConfigFile(data, "/tmp/settings.json")

		assert.NilError(t, err)
		assert.Equal(t, len(servers), 2)

		var foundHTTP bool
		var foundStdio bool
		for _, server := range servers {
			switch server.Name {
			case "server-one":
				assert.Equal(t, server.Transport, "stdio")
				foundStdio = true
			case "server-two":
				assert.Equal(t, server.Transport, "http")
				assert.Equal(t, server.Headers["X"], "Y")
				foundHTTP = true
			}
		}
		assert.Assert(t, foundStdio)
		assert.Assert(t, foundHTTP)
	})

	t.Run("generic fallback", func(t *testing.T) {
		data := []byte(`{
  "services": {"one": {"command": "node", "args": ["-v"]}},
  "mcp": {"two": {"url": "https://example.com"}},
  "tools": {"bad": {"name": "skip"}}
}`)

		servers, err := ParseImportedConfigFile(data, "/tmp/config.json")

		assert.NilError(t, err)
		assert.Equal(t, len(servers), 2)
		assert.Equal(t, servers[0].Source, "Generic Config")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseImportedConfigFile([]byte(`{invalid}`), "/tmp/config.json")
		assert.Assert(t, err != nil)
	})

	t.Run("empty config", func(t *testing.T) {
		servers, err := ParseImportedConfigFile([]byte(`{"mcpServers":{}}`), "/tmp/config.json")
		assert.NilError(t, err)
		assert.Equal(t, len(servers), 0)
	})
}

func TestImportServers(t *testing.T) {
	t.Run("creates default gateway and preserves metadata", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateways = nil

		servers := []ImportedServer{
			{
				Name:        "stdio",
				Command:     "node",
				Args:        []string{"-v"},
				SourcePath:  "/tmp/stdio.json",
				Description: "Imported stdio server",
			},
			{
				Name:       "http",
				URL:        "https://example.com",
				Headers:    map[string]string{"Authorization": "Bearer token"},
				SourcePath: "/tmp/http.json",
			},
			{
				Name:       "bad",
				SourcePath: "/tmp/bad.json",
			},
		}

		count, err := ImportServers(cfg, servers)

		assert.NilError(t, err)
		assert.Equal(t, count, 2)
		assert.Assert(t, cfg.Gateways["default"] != nil)
		assert.Equal(t, cfg.Gateways["default"].MCPServers["stdio"].Source, "/tmp/stdio.json")
		assert.Equal(t, cfg.Gateways["default"].MCPServers["stdio"].Description, "Imported stdio server")
		assert.Equal(t, cfg.Gateways["default"].MCPServers["http"].Headers["Authorization"], "Bearer token")
	})

	t.Run("rolls back validation failures", func(t *testing.T) {
		cfg := DefaultConfig()

		count, err := ImportServers(cfg, []ImportedServer{{
			Name:       "bad name",
			Command:    "node",
			SourcePath: "/tmp/bad.json",
		}})

		assert.NilError(t, err)
		assert.Equal(t, count, 0)
		assert.Assert(t, cfg.Gateways["default"] != nil)
		_, exists := cfg.Gateways["default"].MCPServers["bad name"]
		assert.Assert(t, !exists)
	})

	t.Run("rejects nil config", func(t *testing.T) {
		_, err := ImportServers(nil, nil)
		assert.Assert(t, err != nil)
	})
}

func TestStringMapField(t *testing.T) {
	t.Run("returns nil for missing or invalid maps", func(t *testing.T) {
		assert.Assert(t, stringMapField(map[string]interface{}{}, "headers") == nil)
		assert.Assert(t, stringMapField(map[string]interface{}{"headers": "bad"}, "headers") == nil)
	})

	t.Run("filters non-string values and returns nil when empty", func(t *testing.T) {
		headers := stringMapField(map[string]interface{}{
			"headers": map[string]interface{}{
				"Authorization": "Bearer token",
				"Retry-After":   10,
			},
		}, "headers")

		assert.DeepEqual(t, headers, map[string]string{"Authorization": "Bearer token"})

		empty := stringMapField(map[string]interface{}{
			"headers": map[string]interface{}{
				"Retry-After": 10,
			},
		}, "headers")
		assert.Assert(t, empty == nil)
	})
}

func TestEnsureAbsolutePath(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "config.json")
	assert.Equal(t, ensureAbsolutePath(absolute), absolute)

	relative := filepath.Join("relative", "config.json")
	ensured := ensureAbsolutePath(relative)
	assert.Assert(t, filepath.IsAbs(ensured))
	assert.Assert(t, filepath.IsAbs(ensured))
	assert.Assert(t, filepath.Base(ensured) == "config.json")
}
