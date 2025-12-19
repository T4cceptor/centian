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
// proxy behavior, processors, and additional metadata.
type GlobalConfig struct {
	Version    string                 `json:"version"`              // Config schema version
	Servers    map[string]*MCPServer  `json:"servers"`              // Named MCP servers
	Proxy      *ProxySettings         `json:"proxy,omitempty"`      // Proxy-level settings
	Processors []*ProcessorConfig     `json:"processors,omitempty"` // Processor chain
	Metadata   map[string]interface{} `json:"metadata,omitempty"`   // Additional metadata
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

// ProcessorConfig defines a single processor that executes during MCP request/response flow.
// Processors are composable units that can inspect, modify, or reject MCP messages.
//
// TODO: move below documentation into a better place
// Type-specific configuration (Config field):
//
// For "cli" type processors:
//   - "command" (string, required): Executable command to run (e.g., "python", "bash", "node")
//   - "args" (array of strings, optional): Command-line arguments (e.g., ["script.py", "--flag"])
//
// Example CLI processor:
//
//	{
//	  "name": "security-validator",
//	  "type": "cli",
//	  "enabled": true,
//	  "timeout": 20,
//	  "config": {
//	    "command": "python",
//	    "args": ["~/processors/security.py", "--strict"]
//	  }
//	}
type ProcessorConfig struct {
	Name    string                 `json:"name"`              // Unique processor name
	Type    string                 `json:"type"`              // Processor type: "cli" (future: "http", "builtin")
	Enabled bool                   `json:"enabled"`           // Whether processor is active
	Timeout int                    `json:"timeout,omitempty"` // Timeout in seconds (default: 15)
	Config  map[string]interface{} `json:"config"`            // Type-specific configuration
}

// ProcessorInput represents the JSON input passed to processors via stdin.
// This structure provides all context needed for the processor to make decisions.
type ProcessorInput struct {
	Type       string                 `json:"type"`       // "request" or "response"
	Timestamp  string                 `json:"timestamp"`  // ISO 8601 timestamp
	Connection ConnectionContext      `json:"connection"` // Connection metadata
	Payload    map[string]interface{} `json:"payload"`    // MCP message payload
	Metadata   ProcessorMetadata      `json:"metadata"`   // Additional context
}

// ConnectionContext provides connection-level metadata for processors.
type ConnectionContext struct {
	ServerName string `json:"server_name"` // Name of the MCP server
	Transport  string `json:"transport"`   // Transport type: stdio, http, websocket
	SessionID  string `json:"session_id"`  // Unique session identifier
}

// ProcessorMetadata contains additional context for processor execution.
type ProcessorMetadata struct {
	ProcessorChain  []string               `json:"processor_chain"`  // Names of processors already executed
	OriginalPayload map[string]interface{} `json:"original_payload"` // Original unmodified payload
}

// ProcessorOutput represents the JSON output expected from processors via stdout.
// Processors must return this structure to indicate their decision and any modifications.
type ProcessorOutput struct {
	Status   int                    `json:"status"`             // HTTP-style status: 200, 40x, 50x
	Payload  map[string]interface{} `json:"payload"`            // Modified or original payload
	Error    *string                `json:"error"`              // Error message if status >= 400, otherwise null
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Processor-specific metadata
}

// DefaultConfig returns a default configuration
func DefaultConfig() *GlobalConfig {
	return &GlobalConfig{
		Version:    "1.0.0",
		Servers:    make(map[string]*MCPServer),
		Proxy: &ProxySettings{
			Transport: "stdio",
			LogLevel:  "info",
			Timeout:   30,
		},
		Processors: []*ProcessorConfig{}, // Empty processor list is valid (no-op)
		Metadata:   make(map[string]interface{}),
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

	// Validate processors
	processorNames := make(map[string]bool)
	for i, processor := range config.Processors {
		// Required fields
		if processor.Name == "" {
			return fmt.Errorf("processor[%d]: name is required", i)
		}

		// Check for duplicate processor names
		if processorNames[processor.Name] {
			return fmt.Errorf("processor '%s': duplicate processor name", processor.Name)
		}
		processorNames[processor.Name] = true

		if processor.Type == "" {
			return fmt.Errorf("processor '%s': type is required", processor.Name)
		}

		// Validate type
		if processor.Type != "cli" {
			return fmt.Errorf("processor '%s': unsupported type '%s' (v1 only supports 'cli')", processor.Name, processor.Type)
		}

		// Set default timeout if not specified
		if processor.Timeout == 0 {
			processor.Timeout = 15 // Default 15 seconds
		}

		// Validate config field is present
		if processor.Config == nil {
			return fmt.Errorf("processor '%s': config is required", processor.Name)
		}

		// Validate type-specific config
		switch processor.Type {
		case "cli":
			// CLI processors require command field in config
			command, ok := processor.Config["command"]
			if !ok {
				return fmt.Errorf("processor '%s': config.command is required for cli type", processor.Name)
			}
			if _, ok := command.(string); !ok {
				return fmt.Errorf("processor '%s': config.command must be a string", processor.Name)
			}

			// Args is optional but must be array if present
			if args, exists := processor.Config["args"]; exists {
				if _, ok := args.([]interface{}); !ok {
					return fmt.Errorf("processor '%s': config.args must be an array", processor.Name)
				}
			}
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
