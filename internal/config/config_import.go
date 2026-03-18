package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/T4cceptor/centian/internal/common"
)

// ImportedServer represents a server parsed from an external MCP config file.
type ImportedServer struct {
	Name        string
	Command     string
	Args        []string
	Env         map[string]string
	URL         string
	Headers     map[string]string
	Transport   string
	Description string
	Source      string
	SourcePath  string
}

// ParseImportedConfigFile parses an external MCP config file into importable servers.
func ParseImportedConfigFile(data []byte, filePath string) ([]ImportedServer, error) {
	return detectAndParseImportedConfig(data, filePath)
}

// ImportServers adds imported servers to the default gateway when they validate cleanly.
func ImportServers(cfg *GlobalConfig, servers []ImportedServer) (int, error) {
	if cfg == nil {
		return 0, fmt.Errorf("config is required")
	}

	imported := 0
	for i := range servers {
		server := &servers[i]
		if server.Command == "" && server.URL == "" {
			continue
		}

		gateway := ensureDefaultGateway(cfg)
		previous, hadPrevious := gateway.MCPServers[server.Name]

		enabled := true
		gateway.AddServer(server.Name, &MCPServerConfig{
			Name:        server.Name,
			Command:     server.Command,
			Args:        server.Args,
			Env:         server.Env,
			URL:         server.URL,
			Headers:     server.Headers,
			Enabled:     &enabled,
			Description: server.Description,
			Source:      server.SourcePath,
		})

		if err := ValidateConfig(cfg, true); err != nil {
			if hadPrevious {
				gateway.MCPServers[server.Name] = previous
			} else {
				gateway.RemoveServer(server.Name)
			}
			continue
		}

		imported++
	}

	return imported, nil
}

func ensureDefaultGateway(cfg *GlobalConfig) *GatewayConfig {
	if cfg.Gateways == nil {
		cfg.Gateways = make(map[string]*GatewayConfig)
	}
	if cfg.Gateways["default"] == nil {
		cfg.Gateways["default"] = &GatewayConfig{
			MCPServers: make(map[string]*MCPServerConfig),
		}
	}
	if cfg.Gateways["default"].MCPServers == nil {
		cfg.Gateways["default"].MCPServers = make(map[string]*MCPServerConfig)
	}
	return cfg.Gateways["default"]
}

func detectAndParseImportedConfig(data []byte, filePath string) ([]ImportedServer, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	switch {
	case parsed["mcpServers"] != nil:
		return parseImportedClaudeDesktopConfig(data, filePath)
	case parsed["servers"] != nil:
		return parseImportedVSCodeConfig(data, filePath)
	case parsed["mcp.servers"] != nil:
		return parseImportedSettingsConfig(data, filePath)
	default:
		return parseImportedGenericConfig(data, filePath)
	}
}

func parseImportedClaudeDesktopConfig(data []byte, filePath string) ([]ImportedServer, error) {
	var parsed struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	servers := make([]ImportedServer, 0, len(parsed.MCPServers))
	for name, server := range parsed.MCPServers {
		if server.Command == "" {
			continue
		}
		servers = append(servers, ImportedServer{
			Name:        name,
			Command:     server.Command,
			Args:        server.Args,
			Env:         server.Env,
			Transport:   string(common.StdioTransport),
			Description: fmt.Sprintf("Imported from Claude Desktop (%s)", name),
			Source:      "Claude Desktop",
			SourcePath:  ensureAbsolutePath(filePath),
		})
	}

	return servers, nil
}

func parseImportedVSCodeConfig(data []byte, filePath string) ([]ImportedServer, error) {
	var parsed struct {
		Servers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"servers"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	servers := make([]ImportedServer, 0, len(parsed.Servers))
	for name, server := range parsed.Servers {
		hasCommand := server.Command != ""
		hasURL := server.URL != ""
		if hasCommand == hasURL {
			continue
		}

		transport := string(common.StdioTransport)
		if hasURL {
			transport = string(common.HTTPTransport)
		}

		servers = append(servers, ImportedServer{
			Name:        name,
			Command:     server.Command,
			Args:        server.Args,
			Env:         server.Env,
			URL:         server.URL,
			Headers:     server.Headers,
			Transport:   transport,
			Description: fmt.Sprintf("Imported from VS Code MCP config (%s)", name),
			Source:      "VS Code MCP",
			SourcePath:  ensureAbsolutePath(filePath),
		})
	}

	return servers, nil
}

func parseImportedSettingsConfig(data []byte, filePath string) ([]ImportedServer, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	var servers []ImportedServer
	mcpConfig, exists := parsed["mcp.servers"]
	if !exists {
		return servers, nil
	}

	serverMap, ok := mcpConfig.(map[string]interface{})
	if !ok {
		return servers, nil
	}

	for name, serverData := range serverMap {
		serverInfo, ok := serverData.(map[string]interface{})
		if !ok {
			continue
		}
		server := extractImportedServerFromSettings(name, serverInfo, filePath)
		if server != nil {
			servers = append(servers, *server)
		}
	}

	return servers, nil
}

func parseImportedGenericConfig(data []byte, filePath string) ([]ImportedServer, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	var servers []ImportedServer
	for _, key := range []string{"servers", "mcpServers", "mcp", "tools", "services"} {
		section, exists := parsed[key]
		if !exists {
			continue
		}

		serverMap, ok := section.(map[string]interface{})
		if !ok {
			continue
		}

		for name, serverData := range serverMap {
			serverInfo, ok := serverData.(map[string]interface{})
			if !ok {
				continue
			}
			server := extractImportedServerFromGeneric(name, serverInfo, filePath, key)
			if server != nil {
				servers = append(servers, *server)
			}
		}
	}

	return servers, nil
}

func extractImportedServerFromSettings(name string, serverInfo map[string]interface{}, sourcePath string) *ImportedServer {
	server := &ImportedServer{
		Name:        name,
		Env:         make(map[string]string),
		Description: fmt.Sprintf("Imported from settings (%s)", name),
		Source:      "VS Code Settings",
		SourcePath:  ensureAbsolutePath(sourcePath),
		Transport:   string(common.StdioTransport),
	}

	if command, ok := serverInfo["command"].(string); ok {
		server.Command = command
	}
	if url, ok := serverInfo["url"].(string); ok {
		server.URL = url
		server.Transport = string(common.HTTPTransport)
	}
	if server.Command == "" && server.URL == "" {
		return nil
	}

	if args, ok := serverInfo["args"].([]interface{}); ok {
		for _, arg := range args {
			if argStr, ok := arg.(string); ok {
				server.Args = append(server.Args, argStr)
			}
		}
	}

	if env, ok := serverInfo["env"].(map[string]interface{}); ok {
		for key, value := range env {
			if valueStr, ok := value.(string); ok {
				server.Env[key] = valueStr
			}
		}
	}

	if headers, ok := serverInfo["headers"].(map[string]interface{}); ok {
		server.Headers = make(map[string]string)
		for key, value := range headers {
			if valueStr, ok := value.(string); ok {
				server.Headers[key] = valueStr
			}
		}
	}

	return server
}

func extractImportedServerFromGeneric(name string, serverInfo map[string]interface{}, sourcePath, section string) *ImportedServer {
	server := &ImportedServer{
		Name:        name,
		Env:         make(map[string]string),
		Description: fmt.Sprintf("Imported from %s (%s.%s)", filepath.Base(sourcePath), section, name),
		Source:      "Generic Config",
		SourcePath:  ensureAbsolutePath(sourcePath),
		Transport:   string(common.StdioTransport),
	}

	server.Command = firstStringField(serverInfo, "command", "cmd", "executable", "exec")
	server.URL = firstStringField(serverInfo, "url", "endpoint", "uri", "address")
	if server.URL != "" {
		server.Transport = string(common.HTTPTransport)
	}

	if server.Command == "" && server.URL == "" {
		return nil
	}

	server.Args = firstStringSliceField(serverInfo, "args", "arguments", "params", "parameters")
	server.Headers = stringMapField(serverInfo)

	return server
}

func firstStringField(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key].(string)
		if ok && value != "" {
			return value
		}
	}
	return ""
}

func firstStringSliceField(values map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		rawValues, ok := values[key].([]interface{})
		if !ok {
			continue
		}

		result := make([]string, 0, len(rawValues))
		for _, rawValue := range rawValues {
			value, ok := rawValue.(string)
			if ok {
				result = append(result, value)
			}
		}
		return result
	}

	return nil
}

func stringMapField(values map[string]interface{}) map[string]string {
	rawMap, ok := values["headers"].(map[string]interface{})
	if !ok {
		return nil
	}

	result := make(map[string]string)
	for mapKey, rawValue := range rawMap {
		value, ok := rawValue.(string)
		if ok {
			result[mapKey] = value
		}
	}
	if len(result) == 0 {
		return nil
	}

	return result
}

func ensureAbsolutePath(filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filePath
	}

	return absPath
}
