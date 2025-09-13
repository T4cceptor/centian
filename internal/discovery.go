// Package internal provides MCP server auto-discovery functionality.
// This system scans common configuration locations to automatically import
// existing MCP server configurations into centian.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DiscoveredServer represents an MCP server found during auto-discovery
type DiscoveredServer struct {
	Name        string            `json:"name"`        // Server name
	Command     string            `json:"command"`     // Executable command (for stdio transport)
	Args        []string          `json:"args"`        // Command arguments
	Env         map[string]string `json:"env"`         // Environment variables
	URL         string            `json:"url"`         // HTTP/WebSocket URL (for http/ws transport)
	Transport   string            `json:"transport"`   // Transport type: stdio, http, websocket
	Description     string            `json:"description"`     // Human readable description
	Source          string            `json:"source"`          // Where it was discovered from
	SourcePath      string            `json:"sourcePath"`      // Full path to source file
	ReplacementMode bool              `json:"replacementMode"` // Whether to generate replacement configs
}

// DiscoveryResult contains the results of auto-discovery scan
type DiscoveryResult struct {
	Servers []DiscoveredServer `json:"servers"`
	Errors  []string           `json:"errors"` // Non-fatal errors during discovery
}

// ConfigDiscoverer defines the interface for config file discoverers
type ConfigDiscoverer interface {
	// Name returns the human-readable name of this discoverer
	Name() string
	
	// Description returns a description of what this discoverer searches for
	Description() string
	
	// Discover scans for and parses MCP server configurations
	Discover() ([]DiscoveredServer, error)
	
	// IsAvailable checks if this discoverer can run on the current system
	IsAvailable() bool
}

// DiscoveryManager manages multiple config discoverers
type DiscoveryManager struct {
	discoverers []ConfigDiscoverer
}

// NewDiscoveryManager creates a new discovery manager with default discoverers
func NewDiscoveryManager() *DiscoveryManager {
	dm := &DiscoveryManager{
		discoverers: []ConfigDiscoverer{},
	}
	
	// Register built-in discoverers
	dm.RegisterDiscoverer(&ClaudeDesktopDiscoverer{})
	dm.RegisterDiscoverer(&VSCodeDiscoverer{})
	
	return dm
}

// RegisterDiscoverer adds a new config discoverer
func (dm *DiscoveryManager) RegisterDiscoverer(discoverer ConfigDiscoverer) {
	dm.discoverers = append(dm.discoverers, discoverer)
}

// DiscoverAll runs all available discoverers and aggregates results
func (dm *DiscoveryManager) DiscoverAll() *DiscoveryResult {
	result := &DiscoveryResult{
		Servers: []DiscoveredServer{},
		Errors:  []string{},
	}
	
	for _, discoverer := range dm.discoverers {
		if !discoverer.IsAvailable() {
			continue
		}
		
		servers, err := discoverer.Discover()
		if err != nil {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: %v", discoverer.Name(), err))
			continue
		}
		
		result.Servers = append(result.Servers, servers...)
	}
	
	// Deduplicate servers by name
	result.Servers = deduplicateServers(result.Servers)
	
	return result
}

// ListDiscoverers returns information about available discoverers
func (dm *DiscoveryManager) ListDiscoverers() []map[string]string {
	var discoverers []map[string]string
	for _, d := range dm.discoverers {
		discoverers = append(discoverers, map[string]string{
			"name":        d.Name(),
			"description": d.Description(),
			"available":   fmt.Sprintf("%t", d.IsAvailable()),
		})
	}
	return discoverers
}

// deduplicateServers removes duplicate servers, preferring later entries
func deduplicateServers(servers []DiscoveredServer) []DiscoveredServer {
	seen := make(map[string]int) // name -> index
	var result []DiscoveredServer
	
	for _, server := range servers {
		if idx, exists := seen[server.Name]; exists {
			// Replace existing server with newer one
			result[idx] = server
		} else {
			// Add new server
			seen[server.Name] = len(result)
			result = append(result, server)
		}
	}
	
	return result
}

// ClaudeDesktopDiscoverer discovers MCP servers from Claude Desktop configuration
type ClaudeDesktopDiscoverer struct{}

func (c *ClaudeDesktopDiscoverer) Name() string {
	return "Claude Desktop"
}

func (c *ClaudeDesktopDiscoverer) Description() string {
	return "Scans Claude Desktop configuration for MCP servers"
}

func (c *ClaudeDesktopDiscoverer) IsAvailable() bool {
	// Claude Desktop is primarily available on macOS and Windows
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func (c *ClaudeDesktopDiscoverer) Discover() ([]DiscoveredServer, error) {
	configPath := c.getConfigPath()
	if configPath == "" {
		return nil, fmt.Errorf("claude desktop config path not found")
	}
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []DiscoveredServer{}, nil // No config found, not an error
	}
	
	// Read and parse config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	
	var servers []DiscoveredServer
	for name, serverConfig := range config.MCPServers {
		// Determine transport type
		transport := "stdio" // Default for Claude Desktop is stdio
		if serverConfig.Command == "" {
			continue // Skip servers without commands in Claude Desktop
		}

		server := DiscoveredServer{
			Name:        name,
			Command:     serverConfig.Command,
			Args:        serverConfig.Args,
			Env:         serverConfig.Env,
			Transport:   transport,
			Description: fmt.Sprintf("Imported from Claude Desktop (%s)", name),
			Source:      "Claude Desktop",
			SourcePath:  configPath,
		}
		servers = append(servers, server)
	}
	
	return servers, nil
}

func (c *ClaudeDesktopDiscoverer) getConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "Claude", "claude_desktop_config.json")
		}
	}
	return ""
}

// VSCodeDiscoverer discovers MCP servers from VS Code configurations
type VSCodeDiscoverer struct{}

func (v *VSCodeDiscoverer) Name() string {
	return "VS Code"
}

func (v *VSCodeDiscoverer) Description() string {
	return "Scans VS Code workspace and user settings for MCP configurations"
}

func (v *VSCodeDiscoverer) IsAvailable() bool {
	return true // VS Code can be on any platform
}

func (v *VSCodeDiscoverer) Discover() ([]DiscoveredServer, error) {
	var allServers []DiscoveredServer
	
	// Search common VS Code config locations
	searchPaths := v.getSearchPaths()
	
	for _, searchPath := range searchPaths {
		servers, err := v.scanPath(searchPath)
		if err != nil {
			// Log error but continue with other paths
			continue
		}
		allServers = append(allServers, servers...)
	}
	
	return allServers, nil
}

func (v *VSCodeDiscoverer) getSearchPaths() []string {
	homeDir, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	
	paths := []string{
		filepath.Join(cwd, ".vscode", "mcp.json"),
		filepath.Join(cwd, ".vscode", "settings.json"),
	}
	
	// Add user settings path based on OS
	switch runtime.GOOS {
	case "darwin":
		paths = append(paths, filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json"))
	case "linux":
		paths = append(paths, filepath.Join(homeDir, ".config", "Code", "User", "settings.json"))
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			paths = append(paths, filepath.Join(appData, "Code", "User", "settings.json"))
		}
	}
	
	return paths
}

func (v *VSCodeDiscoverer) scanPath(configPath string) ([]DiscoveredServer, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []DiscoveredServer{}, nil
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	
	// Try to parse as different config formats
	if strings.HasSuffix(configPath, "mcp.json") {
		return v.parseMCPJson(data, configPath)
	} else if strings.HasSuffix(configPath, "settings.json") {
		return v.parseSettingsJson(data, configPath)
	}
	
	return []DiscoveredServer{}, nil
}

func (v *VSCodeDiscoverer) parseMCPJson(data []byte, sourcePath string) ([]DiscoveredServer, error) {
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string   `json:"url"`
		} `json:"servers"`
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	var servers []DiscoveredServer
	for name, serverConfig := range config.Servers {
		// Determine transport and set appropriate fields
		transport := "stdio" // default
		var url string
		
		// Check for URL field in config (for HTTP-based servers)
		if serverConfig.URL != "" {
			transport = "http"
			url = serverConfig.URL
		}
		
		server := DiscoveredServer{
			Name:        name,
			Command:     serverConfig.Command,
			Args:        serverConfig.Args,
			Env:         serverConfig.Env,
			URL:         url,
			Transport:   transport,
			Description: fmt.Sprintf("Imported from VS Code MCP config (%s)", name),
			Source:      "VS Code MCP",
			SourcePath:  sourcePath,
		}
		servers = append(servers, server)
	}
	
	return servers, nil
}

func (v *VSCodeDiscoverer) parseSettingsJson(data []byte, sourcePath string) ([]DiscoveredServer, error) {
	var config map[string]interface{}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	var servers []DiscoveredServer
	
	// Look for MCP-related settings in various extensions
	// This is a placeholder - specific extensions would need specific parsing
	// For now, we'll look for common MCP extension patterns
	
	if mcpConfig, exists := config["mcp.servers"]; exists {
		if serversMap, ok := mcpConfig.(map[string]interface{}); ok {
			for name, serverData := range serversMap {
				if serverInfo, ok := serverData.(map[string]interface{}); ok {
					server := v.extractServerFromMap(name, serverInfo, sourcePath)
					if server != nil {
						servers = append(servers, *server)
					}
				}
			}
		}
	}
	
	return servers, nil
}

func (v *VSCodeDiscoverer) extractServerFromMap(name string, serverInfo map[string]interface{}, sourcePath string) *DiscoveredServer {
	server := &DiscoveredServer{
		Name:        name,
		Env:         make(map[string]string),
		Description: fmt.Sprintf("Imported from VS Code settings (%s)", name),
		Source:      "VS Code Settings",
		SourcePath:  sourcePath,
		Transport:   "stdio", // default
	}
	
	// Check for command (stdio transport)
	if cmd, ok := serverInfo["command"].(string); ok {
		server.Command = cmd
	}
	
	// Check for URL (http transport)
	if url, ok := serverInfo["url"].(string); ok {
		server.URL = url
		server.Transport = "http"
	}
	
	// Require either command or URL
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
	
	return server
}