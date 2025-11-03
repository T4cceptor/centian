package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// matchesPattern checks if a file matches a discovery pattern
func (r *RegexDiscoverer) matchesPattern(filePath string, pattern DiscoveryPattern) (bool, error) {
	// Check file path regex
	matched, err := regexp.MatchString(pattern.FileRegex, filePath)
	if err != nil {
		return false, fmt.Errorf("invalid file regex: %w", err)
	}

	if !matched {
		return false, nil
	}

	// If no content regex specified, file regex match is sufficient
	if len(pattern.ContentRegex) == 0 {
		return true, nil
	}

	// Check content regex patterns
	return r.matchesContentRegex(filePath, pattern.ContentRegex)
}

// matchesContentRegex checks if file content matches any of the content patterns
func (r *RegexDiscoverer) matchesContentRegex(filePath string, contentPatterns []string) (bool, error) {
	// Read file content (limit to first 10KB for performance)
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read up to 10KB
	buffer := make([]byte, 10240)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return false, err
	}

	content := string(buffer[:n])

	// Check each content pattern
	for _, pattern := range contentPatterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue // Skip invalid regex
		}

		if regex.MatchString(content) {
			return true, nil
		}
	}

	return false, nil
}

// parseFile extracts MCP servers from a matched file
func (r *RegexDiscoverer) parseFile(filePath string, pattern DiscoveryPattern) ([]DiscoveredServer, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Get the appropriate parser
	parser := getParser(pattern.Parser)
	if parser == nil {
		return nil, fmt.Errorf("unknown parser: %s", pattern.Parser)
	}

	servers, err := parser(data, filePath)
	if err != nil {
		return nil, err
	}

	// Set source type for replacement logic
	for i := range servers {
		if pattern.SourceType != "auto-detect" {
			// Use pattern-specified source type
		} else {
			// Auto-detect based on file path
			servers[i].Source = detectSourceType(filePath)
		}
	}

	return servers, nil
}

// getParser returns the appropriate parser function for the given name
func getParser(parserName string) ConfigParserFunc {
	parsers := map[string]ConfigParserFunc{
		"detectAndParse":      detectAndParseConfig,
		"claudeDesktopParser": parseClaudeDesktopConfig,
		"vscodeParser":        parseVSCodeConfig,
		"settingsParser":      parseSettingsConfig,
		"genericParser":       parseGenericConfig,
	}

	return parsers[parserName]
}

// detectSourceType automatically determines source type from file path
func detectSourceType(filePath string) string {
	path := strings.ToLower(filePath)

	if strings.Contains(path, "claude_desktop_config.json") {
		return "claude-desktop"
	} else if strings.Contains(path, ".vscode/mcp.json") {
		return "vscode-mcp"
	} else if strings.Contains(path, "settings.json") {
		return "vscode-settings"
	} else if strings.Contains(path, ".claude") {
		return "claude-code"
	} else if strings.Contains(path, ".continue") {
		return "continue-dev"
	}

	return "generic"
}

// Parser implementations

// detectAndParseConfig automatically detects config format and parses appropriately
func detectAndParseConfig(data []byte, filePath string) ([]DiscoveredServer, error) {
	// Try to parse as JSON first
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Check for different config formats
	if _, exists := config["mcpServers"]; exists {
		// Claude Desktop format
		return parseClaudeDesktopConfig(data, filePath)
	}

	if _, exists := config["servers"]; exists {
		// VS Code MCP format or generic servers section
		return parseVSCodeConfig(data, filePath)
	}

	if _, exists := config["mcp.servers"]; exists {
		// VS Code settings format
		return parseSettingsConfig(data, filePath)
	}

	// Try generic parsing for unknown formats
	return parseGenericConfig(data, filePath)
}

// parseClaudeDesktopConfig parses Claude Desktop configuration format
func parseClaudeDesktopConfig(data []byte, filePath string) ([]DiscoveredServer, error) {
	var config struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []DiscoveredServer
	for name, serverConfig := range config.MCPServers {
		if serverConfig.Command == "" {
			continue // Skip servers without commands
		}

		server := DiscoveredServer{
			Name:        name,
			Command:     serverConfig.Command,
			Args:        serverConfig.Args,
			Env:         serverConfig.Env,
			Transport:   "stdio",
			Description: fmt.Sprintf("Imported from Claude Desktop (%s)", name),
			Source:      "Claude Desktop",
			SourcePath:  ensureAbsolutePath(filePath),
		}
		servers = append(servers, server)
	}

	return servers, nil
}

// parseVSCodeConfig parses VS Code MCP configuration format
func parseVSCodeConfig(data []byte, filePath string) ([]DiscoveredServer, error) {
	var config struct {
		Servers map[string]struct {
			Type    string            `json:"type"`    // Required: "stdio" or "http"
			Command string            `json:"command"` // Required for stdio type
			Args    []string          `json:"args"`    // Optional for stdio type
			Env     map[string]string `json:"env"`     // Optional for stdio type
			URL     string            `json:"url"`     // Required for http type
			Headers map[string]string `json:"headers"` // Optional for http type
		} `json:"servers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []DiscoveredServer
	for name, serverConfig := range config.Servers {
		// Validate based on type field
		switch serverConfig.Type {
		case "stdio":
			// For stdio type, command is required
			if serverConfig.Command == "" {
				continue // Skip invalid stdio servers
			}
		case "http":
			// For http type, url is required
			if serverConfig.URL == "" {
				continue // Skip invalid http servers
			}
		default:
			// Skip servers with unknown or missing type
			continue
		}

		// Set transport and url based on type
		transport := serverConfig.Type
		var url string
		if serverConfig.Type == "http" {
			url = serverConfig.URL
		}

		server := DiscoveredServer{
			Name:        name,
			Command:     serverConfig.Command,
			Args:        serverConfig.Args,
			Env:         serverConfig.Env,
			URL:         url,
			Transport:   transport,
			Headers:     serverConfig.Headers,
			Description: fmt.Sprintf("Imported from VS Code MCP config (%s)", name),
			Source:      "VS Code MCP",
			SourcePath:  ensureAbsolutePath(filePath),
		}
		servers = append(servers, server)
	}

	return servers, nil
}

// parseSettingsConfig parses VS Code settings.json MCP configuration
func parseSettingsConfig(data []byte, filePath string) ([]DiscoveredServer, error) {
	var config map[string]interface{}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []DiscoveredServer

	// Look for MCP-related settings
	if mcpConfig, exists := config["mcp.servers"]; exists {
		if serversMap, ok := mcpConfig.(map[string]interface{}); ok {
			for name, serverData := range serversMap {
				if serverInfo, ok := serverData.(map[string]interface{}); ok {
					server := extractServerFromSettings(name, serverInfo, filePath)
					if server != nil {
						servers = append(servers, *server)
					}
				}
			}
		}
	}

	return servers, nil
}

// parseGenericConfig attempts to parse unknown configuration formats
func parseGenericConfig(data []byte, filePath string) ([]DiscoveredServer, error) {
	var config map[string]interface{}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []DiscoveredServer

	// Look for common patterns that might indicate MCP servers
	commonKeys := []string{"servers", "mcpServers", "mcp", "tools", "services"}

	for _, key := range commonKeys {
		if section, exists := config[key]; exists {
			if serversMap, ok := section.(map[string]interface{}); ok {
				for name, serverData := range serversMap {
					if serverInfo, ok := serverData.(map[string]interface{}); ok {
						server := extractServerFromGeneric(name, serverInfo, filePath, key)
						if server != nil {
							servers = append(servers, *server)
						}
					}
				}
			}
		}
	}

	return servers, nil
}

// Helper function to extract server from settings format
func extractServerFromSettings(name string, serverInfo map[string]interface{}, sourcePath string) *DiscoveredServer {
	server := &DiscoveredServer{
		Name:        name,
		Env:         make(map[string]string),
		Description: fmt.Sprintf("Imported from settings (%s)", name),
		Source:      "VS Code Settings",
		SourcePath:  ensureAbsolutePath(sourcePath),
		Transport:   "stdio", // default
	}

	// Extract common fields
	if cmd, ok := serverInfo["command"].(string); ok {
		server.Command = cmd
	}

	if url, ok := serverInfo["url"].(string); ok {
		server.URL = url
		server.Transport = "http"
	}

	if server.Command == "" && server.URL == "" {
		return nil // Skip servers without command or URL
	}

	// Extract args
	if argsInterface, ok := serverInfo["args"]; ok {
		if argsList, ok := argsInterface.([]interface{}); ok {
			for _, arg := range argsList {
				if argStr, ok := arg.(string); ok {
					server.Args = append(server.Args, argStr)
				}
			}
		}
	}

	// Extract environment variables
	if envInterface, ok := serverInfo["env"]; ok {
		if env, ok := envInterface.(map[string]interface{}); ok {
			for key, value := range env {
				if valueStr, ok := value.(string); ok {
					server.Env[key] = valueStr
				}
			}
		}
	}

	return server
}

// Helper function to extract server from generic format
func extractServerFromGeneric(name string, serverInfo map[string]interface{}, sourcePath string, section string) *DiscoveredServer {
	server := &DiscoveredServer{
		Name:        name,
		Env:         make(map[string]string),
		Description: fmt.Sprintf("Imported from %s (%s.%s)", filepath.Base(sourcePath), section, name),
		Source:      "Generic Config",
		SourcePath:  ensureAbsolutePath(sourcePath),
		Transport:   "stdio", // default
	}

	// Try to extract common field names
	possibleCommandKeys := []string{"command", "cmd", "executable", "exec"}
	possibleURLKeys := []string{"url", "endpoint", "uri", "address"}
	possibleArgsKeys := []string{"args", "arguments", "params", "parameters"}

	// Extract command
	for _, key := range possibleCommandKeys {
		if cmd, ok := serverInfo[key].(string); ok && cmd != "" {
			server.Command = cmd
			break
		}
	}

	// Extract URL
	for _, key := range possibleURLKeys {
		if url, ok := serverInfo[key].(string); ok && url != "" {
			server.URL = url
			server.Transport = "http"
			break
		}
	}

	if server.Command == "" && server.URL == "" {
		return nil // Skip servers without command or URL
	}

	// Extract args
	for _, key := range possibleArgsKeys {
		if argsInterface, ok := serverInfo[key]; ok {
			if argsList, ok := argsInterface.([]interface{}); ok {
				for _, arg := range argsList {
					if argStr, ok := arg.(string); ok {
						server.Args = append(server.Args, argStr)
					}
				}
				break
			}
		}
	}

	return server
}
