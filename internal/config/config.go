// Package internal provides configuration management and MCP proxy functionality
// for the Centian CLI tool.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalConfig represents the main configuration structure stored at ~/.centian/config.jsonc.
// This is the root configuration object that contains all settings for MCP servers,
// proxy behavior, lifecycle hooks, and additional metadata.
type GlobalConfig struct {
	Version  string                 `json:"version"`            // Config schema version
	Servers  map[string]*MCPServer  `json:"servers"`            // Named MCP servers
	Proxy    *ProxySettings         `json:"proxy,omitempty"`    // Proxy-level settings
	Hooks    *HookSettings          `json:"hooks,omitempty"`    // Lifecycle hooks
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// MCPServer represents a single MCP server configuration.
// Each server defines how to start and connect to an MCP server process,
// including command, arguments, environment variables, and metadata.
type MCPServer struct {
	Name        string                 `json:"name"`                  // Display name
	Command     string                 `json:"command,omitempty"`     // Executable command (for stdio/process transport)
	Args        []string               `json:"args,omitempty"`        // Command arguments
	Env         map[string]string      `json:"env,omitempty"`         // Environment variables
	URL         string                 `json:"url,omitempty"`         // HTTP/WebSocket URL (for http/ws transport)
	Transport   string                 `json:"transport,omitempty"`   // Transport type: stdio, http, websocket
	Enabled     bool                   `json:"enabled"`               // Whether server is active
	Description string                 `json:"description,omitempty"` // Human readable description
	Source      string                 `json:"source,omitempty"`      // Source file path for auto-discovered servers
	Config      map[string]interface{} `json:"config,omitempty"`      // Server-specific config
}

// ProxySettings contains proxy-level configuration that affects how the
// centian proxy operates, including transport method, logging, and timeouts.
type ProxySettings struct {
	Port      int    `json:"port,omitempty"`     // HTTP proxy port (if enabled)
	Transport string `json:"transport"`          // stdio, http, websocket
	LogLevel  string `json:"logLevel,omitempty"` // debug, info, warn, error
	LogFile   string `json:"logFile,omitempty"`  // Log file path
	Timeout   int    `json:"timeout,omitempty"`  // Request timeout in seconds
}

// HookSettings contains lifecycle hook configurations that define custom
// commands or HTTP endpoints to execute at various points in the MCP request/response cycle.
type HookSettings struct {
	PreRequest   *HookConfig `json:"preRequest,omitempty"`   // Before forwarding request
	PostRequest  *HookConfig `json:"postRequest,omitempty"`  // After receiving response
	OnConnect    *HookConfig `json:"onConnect,omitempty"`    // When server connects
	OnDisconnect *HookConfig `json:"onDisconnect,omitempty"` // When server disconnects
}

// HookConfig defines a single hook that can execute either a shell command
// or make an HTTP POST request when triggered by lifecycle events.
type HookConfig struct {
	Command string            `json:"command"`           // Command to execute
	Args    []string          `json:"args,omitempty"`    // Command arguments
	Env     map[string]string `json:"env,omitempty"`     // Environment variables
	Timeout int               `json:"timeout,omitempty"` // Hook timeout in seconds
	URL     string            `json:"url,omitempty"`     // HTTP endpoint to POST to
}

// DefaultConfig returns a default configuration
func DefaultConfig() *GlobalConfig {
	return &GlobalConfig{
		Version: "1.0.0",
		Servers: make(map[string]*MCPServer),
		Proxy: &ProxySettings{
			Transport: "stdio",
			LogLevel:  "info",
			Timeout:   30,
		},
		Hooks:    &HookSettings{},
		Metadata: make(map[string]interface{}),
	}
}

// GetConfigDir returns the centian config directory path
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".centian"), nil
}

// GetConfigPath returns the full path to config.jsonc
func GetConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.jsonc"), nil
}

// EnsureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	return os.MkdirAll(configDir, 0o755)
}

// LoadConfig loads the global configuration from ~/.centian/config.jsonc.
// If the config file doesn't exist, it creates a new one with default settings.
// The configuration is validated after loading to ensure it's properly formatted.
func LoadConfig() (*GlobalConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found at %s", configPath)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON (note: JSONC support would need additional parsing)
	var config GlobalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate config
	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the configuration to ~/.centian/config.jsonc.
// Creates the ~/.centian directory if it doesn't exist and writes the
// configuration as formatted JSON with proper indentation.
func SaveConfig(config *GlobalConfig) error {
	if err := EnsureConfigDir(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Marshall with indentation for readability
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfig performs basic validation on the configuration to ensure
// required fields are present and values are within acceptable ranges.
// Returns an error if any validation rules fail.
func ValidateConfig(config *GlobalConfig) error {
	if config.Version == "" {
		return fmt.Errorf("version field is required")
	}

	if config.Proxy != nil {
		if config.Proxy.Transport == "" {
			return fmt.Errorf("proxy.transport is required")
		}

		validTransports := []string{"stdio", "http", "websocket"}
		valid := false
		for _, t := range validTransports {
			if config.Proxy.Transport == t {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("proxy.transport must be one of: %v", validTransports)
		}
	}

	// Validate servers
	for name, server := range config.Servers {
		if server.Name == "" {
			server.Name = name // Use key as name if not specified
		}

		// Set default transport if not specified
		if server.Transport == "" {
			if server.URL != "" {
				server.Transport = "http" // Default to HTTP if URL is provided
			} else {
				server.Transport = "stdio" // Default to stdio for command-based servers
			}
		}

		// Validate based on transport type
		switch server.Transport {
		case "stdio", "process":
			if server.Command == "" {
				return fmt.Errorf("server '%s': command is required for %s transport", name, server.Transport)
			}
		case "http", "https", "websocket", "ws":
			if server.URL == "" {
				return fmt.Errorf("server '%s': URL is required for %s transport", name, server.Transport)
			}
		default:
			return fmt.Errorf("server '%s': unsupported transport '%s' (supported: stdio, http, websocket)", name, server.Transport)
		}
	}

	return nil
}

// GetServer returns a server configuration by name
func (c *GlobalConfig) GetServer(name string) (*MCPServer, error) {
	server, exists := c.Servers[name]
	if !exists {
		return nil, fmt.Errorf("server '%s' not found", name)
	}
	if !server.Enabled {
		return nil, fmt.Errorf("server '%s' is disabled", name)
	}
	return server, nil
}

// AddServer adds a new server configuration
func (c *GlobalConfig) AddServer(name string, server *MCPServer) {
	if c.Servers == nil {
		c.Servers = make(map[string]*MCPServer)
	}
	c.Servers[name] = server
}

// RemoveServer removes a server configuration
func (c *GlobalConfig) RemoveServer(name string) {
	delete(c.Servers, name)
}

// ListEnabledServers returns names of enabled servers only
func (c *GlobalConfig) ListEnabledServers() []string {
	var names []string
	for name, server := range c.Servers {
		if server.Enabled {
			names = append(names, name)
		}
	}
	return names
}
