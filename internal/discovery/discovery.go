// Package internal provides MCP server auto-discovery functionality.
// This system scans common configuration locations to automatically import
// existing MCP server configurations into centian.
package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CentianAI/centian-cli/internal/common"
)

// Server represents an MCP server found during auto-discovery
type Server struct {
	Name            string            `json:"name"`            // Server name
	Command         string            `json:"command"`         // Executable command (for stdio transport)
	Args            []string          `json:"args"`            // Command arguments
	Env             map[string]string `json:"env"`             // Environment variables
	URL             string            `json:"url"`             // HTTP/WebSocket URL (for http/ws transport)
	Headers         map[string]string `json:"headers"`         // HTTP headers (for http transport)
	Transport       string            `json:"transport"`       // Transport type: stdio, http, websocket
	Description     string            `json:"description"`     // Human readable description
	Source          string            `json:"source"`          // Where it was discovered from
	SourcePath      string            `json:"sourcePath"`      // Full path to source file
	ReplacementMode bool              `json:"replacementMode"` // Whether to generate replacement configs
	DuplicatesFound int               `json:"duplicatesFound"` // Number of identical configs found and merged
}

// Result contains the results of auto-discovery scan
type Result struct {
	Servers []Server `json:"servers"`
	Errors  []string `json:"errors"` // Non-fatal errors during discovery
}

// ConfigFileGroup represents servers grouped by their source config file
type ConfigFileGroup struct {
	SourcePath      string   `json:"sourcePath"`      // Absolute path to config file
	Servers         []Server `json:"servers"`         // Servers found in this file
	StdioCount      int      `json:"stdioCount"`      // Number of stdio servers
	HTTPCount       int      `json:"httpCount"`       // Number of http servers
	TotalCount      int      `json:"totalCount"`      // Total number of servers
	DuplicatesFound int      `json:"duplicatesFound"` // Total number of duplicate configs merged
}

// GroupedDiscoveryResult contains servers grouped by source file
type GroupedDiscoveryResult struct {
	Groups []ConfigFileGroup `json:"groups"`
	Errors []string          `json:"errors"` // Non-fatal errors during discovery
}

// GroupDiscoveryResults groups servers by their source config file
func GroupDiscoveryResults(result *Result) *GroupedDiscoveryResult {
	// Count duplicates per file BEFORE any deduplication
	duplicateCounts := countDuplicatesPerFile(result.Servers)

	// Group servers by source path
	groupMap := make(map[string][]Server)
	for _, server := range result.Servers {
		groupMap[server.SourcePath] = append(groupMap[server.SourcePath], server)
	}

	// Create groups with transport counts and per-file duplicate counts
	var groups []ConfigFileGroup
	for sourcePath, servers := range groupMap {
		stdioCount := 0
		httpCount := 0

		for _, server := range servers {
			switch server.Transport {
			case "stdio":
				stdioCount++
			case "http":
				httpCount++
			}
		}

		group := ConfigFileGroup{
			SourcePath:      sourcePath,
			Servers:         servers,
			StdioCount:      stdioCount,
			HTTPCount:       httpCount,
			TotalCount:      len(servers),
			DuplicatesFound: duplicateCounts[sourcePath], // Use per-file duplicate count
		}
		groups = append(groups, group)
	}

	return &GroupedDiscoveryResult{
		Groups: groups,
		Errors: result.Errors,
	}
}

// countDuplicatesPerFile counts how many servers in each file have duplicates in other files
func countDuplicatesPerFile(allServers []Server) map[string]int {
	// Create a map of config signature -> list of source paths
	configToSources := make(map[string][]string)

	for _, server := range allServers {
		// Create config signature (same as in deduplicateServers)
		configSig := fmt.Sprintf("%s|%s|%v|%v|%s|%v",
			server.Transport, server.Command, server.Args, server.Env, server.URL, server.Headers)

		configToSources[configSig] = append(configToSources[configSig], server.SourcePath)
	}

	// Count duplicates per file
	duplicatesPerFile := make(map[string]int)

	for _, server := range allServers {
		configSig := fmt.Sprintf("%s|%s|%v|%v|%s|%v",
			server.Transport, server.Command, server.Args, server.Env, server.URL, server.Headers)

		sources := configToSources[configSig]

		// If this config appears in multiple files, it's a duplicate
		if len(sources) > 1 {
			duplicatesPerFile[server.SourcePath]++
		}
	}

	return duplicatesPerFile
}

// ConfigDiscoverer defines the interface for config file discoverers
type ConfigDiscoverer interface {
	// Name returns the human-readable name of this discoverer
	Name() string

	// Description returns a description of what this discoverer searches for
	Description() string

	// Discover scans for and parses MCP server configurations
	Discover() ([]Server, error)

	// IsAvailable checks if this discoverer can run on the current system
	IsAvailable() bool
}

// RegexDiscoverer represents a regex-based pattern for discovering MCP configs
type RegexDiscoverer struct {
	DiscovererName        string
	DiscovererDescription string
	Patterns              []Pattern
	SearchPaths           []string
	MaxDepth              int
	Enabled               bool
}

// Pattern defines how to find and parse a specific type of config file
type Pattern struct {
	FileRegex    string   `json:"fileRegex"`    // Regex pattern for file path/name
	ContentRegex []string `json:"contentRegex"` // Content must match these patterns
	Parser       string   `json:"parser"`       // Parser function name
	SourceType   string   `json:"sourceType"`   // For replacement logic
	Priority     int      `json:"priority"`     // Higher = search first
}

// ConfigParserFunc is a function that parses config data and extracts MCP servers
type ConfigParserFunc func(data []byte, filePath string) ([]Server, error)

// Manager manages multiple config discoverers
type Manager struct {
	discoverers []ConfigDiscoverer
}

// NewDiscoveryManager creates a new discovery manager with default discoverers
func NewDiscoveryManager() *Manager {
	dm := &Manager{
		discoverers: []ConfigDiscoverer{},
	}

	// Register built-in discoverers
	dm.RegisterDiscoverer(&ClaudeDesktopDiscoverer{})
	dm.RegisterDiscoverer(&VSCodeDiscoverer{})
	dm.RegisterDiscoverer(GetDefaultRegexDiscoverer())

	return dm
}

// RegisterDiscoverer adds a new config discoverer
func (dm *Manager) RegisterDiscoverer(discoverer ConfigDiscoverer) {
	dm.discoverers = append(dm.discoverers, discoverer)
}

// DiscoverAll runs all available discoverers and aggregates results
func (dm *Manager) DiscoverAll() *Result {
	common.LogInfo("Starting MCP server discovery")

	result := &Result{
		Servers: []Server{},
		Errors:  []string{},
	}

	common.LogDebug("Checking %d registered discoverers", len(dm.discoverers))

	for _, discoverer := range dm.discoverers {
		if !discoverer.IsAvailable() {
			common.LogDebug("Skipping unavailable discoverer: %s", discoverer.Name())
			continue
		}

		common.LogDebug("Running discoverer: %s", discoverer.Name())
		servers, err := discoverer.Discover()
		if err != nil {
			errMsg := fmt.Sprintf("%s: %v", discoverer.Name(), err)
			common.LogError("Discovery failed: %s", errMsg)
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		common.LogInfo("Discoverer '%s' found %d server(s)", discoverer.Name(), len(servers))
		result.Servers = append(result.Servers, servers...)
	}

	// Deduplicate servers by name
	originalCount := len(result.Servers)
	result.Servers = deduplicateServers(result.Servers)
	if originalCount != len(result.Servers) {
		common.LogInfo("Deduplicated servers: %d -> %d", originalCount, len(result.Servers))
	}

	common.LogInfo("Discovery completed: found %d unique server(s), %d error(s)", len(result.Servers), len(result.Errors))

	return result
}

// ListDiscoverers returns information about available discoverers
func (dm *Manager) ListDiscoverers() []map[string]string {
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
func deduplicateServers(servers []Server) []Server {
	seen := make(map[string][]Server) // name -> list of servers with that name
	var result []Server

	// Group servers by name
	for _, server := range servers {
		seen[server.Name] = append(seen[server.Name], server)
	}

	// Process each group
	for originalName, serverGroup := range seen {
		if len(serverGroup) == 1 {
			// Only one server with this name, add it directly
			result = append(result, serverGroup[0])
			continue
		}

		// Multiple servers with same name - check for duplicates
		uniqueConfigs := make(map[string][]Server) // config hash -> list of servers with identical config

		for _, server := range serverGroup {
			// Create a config signature for comparison (excluding name and source path)
			configSig := fmt.Sprintf("%s|%s|%v|%v|%s|%v",
				server.Transport, server.Command, server.Args, server.Env, server.URL, server.Headers)

			uniqueConfigs[configSig] = append(uniqueConfigs[configSig], server)
		}

		// Add unique configs with counter suffixes if needed
		counter := 1
		for _, duplicateGroup := range uniqueConfigs {
			// Choose the best representative from identical configs
			bestServer := duplicateGroup[0]
			for _, server := range duplicateGroup[1:] {
				// Prefer the one with more specific source info
				if len(server.SourcePath) > len(bestServer.SourcePath) {
					bestServer = server
				}
			}

			// Set the duplicates count (total found - 1 = number of duplicates)
			bestServer.DuplicatesFound = len(duplicateGroup) - 1

			if len(uniqueConfigs) == 1 {
				// Only one unique config, keep original name
				result = append(result, bestServer)
			} else {
				// Multiple unique configs, add counter suffix
				bestServer.Name = fmt.Sprintf("%s-%d", originalName, counter)
				result = append(result, bestServer)
				counter++
			}
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

func (c *ClaudeDesktopDiscoverer) Discover() ([]Server, error) {
	configPath := c.getConfigPath()
	if configPath == "" {
		return nil, fmt.Errorf("claude desktop config path not found")
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []Server{}, nil // No config found, not an error
	}

	// Read and parse config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Type    string            `json:"type"`
			Headers map[string]string `json:"headers"`
		} `json:"servers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	var servers []Server

	// Parse mcpServers section (stdio-based servers)
	for name, serverConfig := range config.MCPServers {
		// Determine transport type
		transport := "stdio" // Default for Claude Desktop is stdio
		if serverConfig.Command == "" {
			continue // Skip servers without commands in Claude Desktop
		}

		server := Server{
			Name:       name,
			Command:    serverConfig.Command,
			Args:       serverConfig.Args,
			Env:        serverConfig.Env,
			Transport:  transport,
			Source:     "Claude Desktop",
			SourcePath: ensureAbsolutePath(configPath),
		}
		servers = append(servers, server)
	}

	// Parse servers section (HTTP-based servers)
	for name, serverConfig := range config.Servers {
		// Determine transport and set appropriate fields
		transport := "stdio" // default
		var url string

		// Check for URL field in config (for HTTP-based servers)
		if serverConfig.URL != "" {
			transport = "http"
			url = serverConfig.URL
		} else if serverConfig.Command == "" {
			continue // Skip servers without command or URL
		}

		server := Server{
			Name:       name,
			Command:    serverConfig.Command,
			Args:       serverConfig.Args,
			Env:        serverConfig.Env,
			URL:        url,
			Transport:  transport,
			Source:     "Claude Desktop",
			SourcePath: configPath,
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

func (v *VSCodeDiscoverer) Discover() ([]Server, error) {
	var allServers []Server

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

func (v *VSCodeDiscoverer) scanPath(configPath string) ([]Server, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []Server{}, nil
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

	return []Server{}, nil
}

func (v *VSCodeDiscoverer) parseMCPJson(data []byte, sourcePath string) ([]Server, error) {
	var config struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
		} `json:"servers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []Server
	for name, serverConfig := range config.Servers {
		// Determine transport and set appropriate fields
		transport := "stdio" // default
		var url string

		// Check for URL field in config (for HTTP-based servers)
		if serverConfig.URL != "" {
			transport = "http"
			url = serverConfig.URL
		}

		server := Server{
			Name:       name,
			Command:    serverConfig.Command,
			Args:       serverConfig.Args,
			Env:        serverConfig.Env,
			URL:        url,
			Transport:  transport,
			Source:     "VS Code MCP",
			SourcePath: ensureAbsolutePath(sourcePath),
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func (v *VSCodeDiscoverer) parseSettingsJson(data []byte, sourcePath string) ([]Server, error) {
	var config map[string]interface{}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []Server

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

func (v *VSCodeDiscoverer) extractServerFromMap(name string, serverInfo map[string]interface{}, sourcePath string) *Server {
	server := &Server{
		Name:       name,
		Env:        make(map[string]string),
		Source:     "VS Code Settings",
		SourcePath: ensureAbsolutePath(sourcePath),
		Transport:  "stdio", // default
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

// RegexDiscoverer implementation
func (r *RegexDiscoverer) Name() string {
	return r.DiscovererName
}

func (r *RegexDiscoverer) Description() string {
	return r.DiscovererDescription
}

func (r *RegexDiscoverer) IsAvailable() bool {
	return r.Enabled
}

func (r *RegexDiscoverer) Discover() ([]Server, error) {
	if !r.Enabled {
		return []Server{}, nil
	}

	var allServers []Server

	// Search each configured path
	for _, searchPath := range r.SearchPaths {
		expandedPath := expandPath(searchPath)
		servers, err := r.scanPath(expandedPath, 0)
		if err != nil {
			// Log error but continue with other paths
			continue
		}
		allServers = append(allServers, servers...)
	}

	return allServers, nil
}

// scanPath recursively scans a directory for config files matching patterns
func (r *RegexDiscoverer) scanPath(path string, depth int) ([]Server, error) {
	if depth > r.MaxDepth {
		return []Server{}, nil
	}

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		return []Server{}, nil // Path doesn't exist, not an error
	}

	var servers []Server

	if info.IsDir() {
		// Skip common directories that are unlikely to contain MCP configs
		if shouldSkipDirectory(path) {
			return []Server{}, nil
		}

		// Scan directory contents
		entries, err := os.ReadDir(path)
		if err != nil {
			return []Server{}, err
		}

		for _, entry := range entries {
			entryPath := filepath.Join(path, entry.Name())
			entryServers, err := r.scanPath(entryPath, depth+1)
			if err != nil {
				continue // Skip errors in subdirectories
			}
			servers = append(servers, entryServers...)
		}
	} else {
		// Check if file matches any pattern
		fileServers, err := r.processFile(path)
		if err != nil {
			return []Server{}, err
		}
		servers = append(servers, fileServers...)
	}

	return servers, nil
}

// processFile checks if a file matches patterns and extracts servers
func (r *RegexDiscoverer) processFile(filePath string) ([]Server, error) {
	// Sort patterns by priority (higher first)
	patterns := make([]Pattern, len(r.Patterns))
	copy(patterns, r.Patterns)

	// Simple bubble sort by priority
	for i := 0; i < len(patterns); i++ {
		for j := 0; j < len(patterns)-1-i; j++ {
			if patterns[j].Priority < patterns[j+1].Priority {
				patterns[j], patterns[j+1] = patterns[j+1], patterns[j]
			}
		}
	}

	// Try each pattern in priority order
	for _, pattern := range patterns {
		matches, err := r.matchesPattern(filePath, pattern)
		if err != nil {
			continue
		}

		if matches {
			return r.parseFile(filePath, pattern)
		}
	}

	return []Server{}, nil
}

// expandPath expands ~ and environment variables in paths
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// ensureAbsolutePath converts a file path to absolute path
func ensureAbsolutePath(filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filePath // Return original if conversion fails
	}

	return absPath
}

// shouldSkipDirectory determines if a directory should be skipped during discovery
// to improve performance by avoiding large directories unlikely to contain MCP configs
func shouldSkipDirectory(fullPath string) bool {
	// Get platform-specific excluded directories
	excludedDirs := getExcludedDirectories()

	// Check directory name against excluded patterns
	dirName := filepath.Base(fullPath)

	// Standard exclusions that apply regardless of platform
	standardExclusions := map[string]bool{
		// Version control
		".git": true,
		".svn": true,
		".hg":  true,

		// Package managers and dependencies
		"node_modules": true,
		"vendor":       true,
		"venv":         true,
		".venv":        true,
		"env":          true,
		"__pycache__":  true,
		".npm":         true,
		".yarn":        true,
		"target":       true, // Rust/Java
		"build":        true,
		"dist":         true,
		"out":          true,

		// IDE and editor files
		".idea":    true,
		".eclipse": true,
		".vs":      true,

		// Docker and containers
		".docker": true,
		"docker":  true,
		".podman": true,
	}

	// Check standard exclusions by directory name
	if standardExclusions[dirName] {
		return true
	}

	// Check platform-specific exclusions by path patterns
	homeDir, _ := os.UserHomeDir()
	for _, excludedPattern := range excludedDirs {
		// Convert pattern to full path if not absolute
		var checkPath string
		if strings.HasPrefix(excludedPattern, "/") {
			checkPath = excludedPattern
		} else {
			checkPath = filepath.Join(homeDir, excludedPattern)
		}

		// Check if current path contains the excluded pattern
		if strings.Contains(fullPath, excludedPattern) || strings.Contains(fullPath, checkPath) {
			return true
		}
	}

	return false
}
