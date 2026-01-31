// Package config provides configuration management and MCP proxy functionality
// for the Centian CLI tool.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/CentianAI/centian-cli/internal/common"
)

// ProcessorType defines the type of processor, e.g. cli, webhook, internal, etc.
type ProcessorType string

const (
	// CLIProcessor represents the type of a CLI-based processor -> "cli".
	CLIProcessor ProcessorType = "cli"
)

// GlobalConfig represents the main configuration structure stored at ~/.centian/config.json.
// This is the root configuration object that contains all settings for MCP servers,
// proxy behavior, processors, and additional metadata.
type GlobalConfig struct {
	Name        string                    `json:"name"`                 // Name of the server - simplifies server identification
	Version     string                    `json:"version"`              // Config schema version
	AuthEnabled *bool                     `json:"auth,omitempty"`       // Enable or disable proxy auth
	AuthHeader  string                    `json:"authHeader,omitempty"` // Header name for proxy auth
	Proxy       *ProxySettings            `json:"proxy,omitempty"`      // Proxy-level settings
	Gateways    map[string]*GatewayConfig `json:"gateways,omitempty"`   // HTTP proxy gateways
	Processors  []*ProcessorConfig        `json:"processors,omitempty"` // Processor chain
	Metadata    map[string]interface{}    `json:"metadata,omitempty"`   // Additional metadata
}

// DefaultAuthHeader represents the default header for authentication at the Centian server.
const DefaultAuthHeader = "X-Centian-Auth"

// DefaultProxyHost represents the default bind address for the Centian server.
const DefaultProxyHost = "127.0.0.1"

// IsAuthEnabled returns true when auth is enabled or unset.
func (g *GlobalConfig) IsAuthEnabled() bool {
	if g == nil || g.AuthEnabled == nil {
		return true
	}
	return *g.AuthEnabled
}

// GetAuthHeader returns the configured auth header name or the default.
func (g *GlobalConfig) GetAuthHeader() string {
	if g == nil || g.AuthHeader == "" {
		return DefaultAuthHeader
	}
	return g.AuthHeader
}

// ServerSearchResult captures data and references
// when searching for a specific server in the config.
type ServerSearchResult struct {
	gatewayName string
	gateway     *GatewayConfig
	server      *MCPServerConfig
}

// SearchServerByName searches for a server given a name,
// can return multiple results for different gateways.
func (g *GlobalConfig) SearchServerByName(name string) []ServerSearchResult {
	foundServers := make([]ServerSearchResult, 0)
	for gatewayName, gatewayConfig := range g.Gateways {
		if gatewayConfig.HasServer(name) {
			foundServers = append(foundServers, ServerSearchResult{
				gatewayName: gatewayName,
				gateway:     gatewayConfig,
				server:      gatewayConfig.MCPServers[name],
			})
		}
	}
	return foundServers
}

// MCPServerConfig represents a single MCP server configuration.
// Each server defines how to start and connect to an MCP server process,
// including all necessary arguments, e.g. command, arguments,
// environment variables, and metadata.
type MCPServerConfig struct {
	Name        string                 `json:"name"`                  // Display name
	Command     string                 `json:"command,omitempty"`     // Executable command (for stdio/process transport)
	Args        []string               `json:"args,omitempty"`        // Command arguments
	Env         map[string]string      `json:"env,omitempty"`         // Environment variables
	URL         string                 `json:"url,omitempty"`         // HTTP/WebSocket URL (for http/sse transport)
	Headers     map[string]string      `json:"headers,omitempty"`     // HTTP headers (supports ${ENV_VAR} substitution)
	Enabled     *bool                  `json:"enabled,omitempty"`     // Whether server is active
	Description string                 `json:"description,omitempty"` // Human readable description
	Source      string                 `json:"source,omitempty"`      // Source file path for auto-discovered servers
	Config      map[string]interface{} `json:"config,omitempty"`      // Server-specific config
}

// IsEnabled returns true if the MCP server is either explicitly enabled or the flag is unset (nil).
func (s *MCPServerConfig) IsEnabled() bool {
	if s.Enabled == nil {
		return true // default
	}
	return *s.Enabled
}

// GetSubstitutedHeaders returns headers with environment variables substituted.
// Supports both ${VAR_NAME} and $VAR_NAME syntax.
// Example: "Bearer ${GITHUB_TOKEN}" -> "Bearer ghp_abc123...".
func (s *MCPServerConfig) GetSubstitutedHeaders() map[string]string {
	if s.Headers == nil {
		return make(map[string]string)
	}

	result := make(map[string]string)
	for key, value := range s.Headers {
		// Use os.Expand to substitute environment variables.
		// Supports both ${VAR} and $VAR syntax.
		result[key] = os.Expand(value, os.Getenv)
	}
	return result
}

// ProxySettings contains proxy-level configuration that affects how the
// centian proxy operates, including transport method, logging, and timeouts.
type ProxySettings struct {
	Host     string `json:"host,omitempty"`     // Bind address for the proxy
	Port     string `json:"port,omitempty"`     // HTTP proxy port (if enabled)
	LogLevel string `json:"logLevel,omitempty"` // debug, info, warn, error
	LogFile  string `json:"logFile,omitempty"`  // Log file path
	Timeout  int    `json:"timeout,omitempty"`  // Request timeout in seconds
}

// NewDefaultProxySettings creates a new ProxySettings with default values.
func NewDefaultProxySettings() ProxySettings {
	return ProxySettings{
		Host:     DefaultProxyHost,
		Port:     "8080",
		Timeout:  30,
		LogLevel: "info",
	}
}

// GatewayConfig represents a logical grouping of HTTP MCP servers.
type GatewayConfig struct {
	AllowDynamic         bool                        `json:"allowDynamic,omitempty"` // Allow dynamic proxy endpoints
	AllowGatewayEndpoint bool                        `json:"setupGateway,omitempty"` // Setup gateway endpoint with namespacing
	MCPServers           map[string]*MCPServerConfig `json:"mcpServers"`             // HTTP MCP servers in this gateway
	Processors           []*ProcessorConfig          `json:"processors,omitempty"`
}

// ListServers returns a slice of all available MCPServerConfigs for this GatewayConfig.
func (g *GatewayConfig) ListServers() []*MCPServerConfig {
	result := make([]*MCPServerConfig, 0)
	for _, server := range g.MCPServers {
		result = append(result, server)
	}
	return result
}

// AddServer adds a the provided server to the gateways MCP servers using name as key.
func (g *GatewayConfig) AddServer(name string, server *MCPServerConfig) {
	if g.MCPServers == nil {
		g.MCPServers = make(map[string]*MCPServerConfig)
	}
	g.MCPServers[name] = server
}

// RemoveServer removes server identified via name.
func (g *GatewayConfig) RemoveServer(name string) {
	delete(g.MCPServers, name)
}

// HasServer returns true if a server with the provided name exists in this gateway.
func (g *GatewayConfig) HasServer(name string) bool {
	for serverName := range g.MCPServers {
		if serverName == name {
			return true
		}
	}
	return false
}

//////// PROCESSOR CONFIG STRUCTS ///////.

// ProcessorConfig defines a single processor that executes during MCP request/response flow.
// Processors are composable units that can inspect, modify, or reject MCP messages.
//
// TODO: move below documentation into a better place
// Type-specific configuration (Config field):
//
// For CLIProcessor processors:
//   - "command" (string, required): Executable command to run (e.g., "python", "bash", "node").
//   - "args" (array of strings, optional): Command-line arguments (e.g., ["script.py", "--flag"]).
//
// Example CLI processor:
//
//	{.
//	  "name": "security-validator",
//	  "type": "cli",
//	  "enabled": true,
//	  "timeout": 20,
//	  "config": {
//	    "command": "python",
//	    "args": ["~/processors/security.py", "--strict"]
//	  }
//	}.
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
	// TODO: potentially add server data in here like URL/CMD, headers/args, etc.
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

// DefaultConfig returns a default configuration.
func DefaultConfig() *GlobalConfig {
	authEnabled := true
	proxySettings := NewDefaultProxySettings()
	return &GlobalConfig{
		Name:        "Centian Server",
		Version:     "1.0.0",
		AuthEnabled: &authEnabled,
		AuthHeader:  DefaultAuthHeader,
		Proxy:       &proxySettings,
		Gateways:    make(map[string]*GatewayConfig),
		Processors:  []*ProcessorConfig{}, // Empty processor list is valid (no-op)
		Metadata:    make(map[string]interface{}),
	}
}

// GetConfigDir returns the centian config directory path.
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".centian"), nil
}

// GetConfigPath returns the full path to config.json.
func GetConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(configDir, 0o750)
}

// LoadConfig loads the global configuration from ~/.centian/config.json.
// If the config file doesn't exist, it creates a new one with default settings.
// The configuration is validated after loading to ensure it's properly formatted.
func LoadConfig() (*GlobalConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	config, err := LoadConfigFromPath(configPath)
	return config, err
}

// LoadConfigFromPath loads configuration from a custom file path.
// The configuration is validated after loading.
func LoadConfigFromPath(path string) (*GlobalConfig, error) {
	// Check if config file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found at %s", path)
	}

	// Read config file.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON.
	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate config schema (allows empty gateways for config management).
	// Server startup should call ValidateConfigForServer for operational validation.
	if err := ValidateConfigSchema(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves the configuration to ~/.centian/config.json.
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

	// Marshall with indentation for readability.
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file.
	//nolint:gosec // We are writing a file without sensitive data.
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfigSchema performs basic schema validation on the configuration.
// This validates required fields and structure but allows empty gateways.
// Use ValidateConfigForServer for operational validation before starting a server.
func ValidateConfigSchema(config *GlobalConfig) error {
	if config.Version == "" {
		return fmt.Errorf("version field is required")
	}
	if config.Proxy == nil {
		return fmt.Errorf("proxy settings are required in config")
	}
	// Validate gateways that exist (but allow empty)
	if err := validateExistingGateways(config.Gateways); err != nil {
		return err
	}
	if err := validateProcessors(config.Processors); err != nil {
		return err
	}
	return nil
}

// ValidateConfig performs full validation including operational requirements.
// This requires at least one gateway to be configured.
func ValidateConfig(config *GlobalConfig) error {
	if err := ValidateConfigSchema(config); err != nil {
		return err
	}
	return ValidateConfigForServer(config)
}

// ValidateConfigForServer validates the config is ready for server operation.
// This checks operational requirements like having at least one gateway configured.
func ValidateConfigForServer(config *GlobalConfig) error {
	if len(config.Gateways) == 0 {
		// TODO: default config should already include gateways and not throw an error!
		return fmt.Errorf("no gateways configured. Add at least one gateway with HTTP MCP servers in your config")
	}
	return nil
}

// validateExistingGateways validates gateway configurations without requiring any.
// This allows empty gateway maps (for freshly initialized configs).
func validateExistingGateways(gateways map[string]*GatewayConfig) error {
	for gatewayName, gatewayConfig := range gateways {
		if err := validateGateway(gatewayName, *gatewayConfig); err != nil {
			return err
		}
		for name, server := range gatewayConfig.MCPServers {
			if err := validateServer(name, server); err != nil {
				return err
			}
		}
	}
	return nil
}

// isValidHTTPURL validates that a URL string is a properly formatted HTTP/HTTPS URL.
// Returns true if the URL has a valid http:// or https:// scheme and a host component.
func isValidHTTPURL(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	// Must have http or https scheme and a host.
	return (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host != ""
}

// validateGateway validates a gateway configuration.
func validateGateway(name string, config GatewayConfig) error {
	// Validate gateway name is URL compliant (used in endpoint paths).
	if !common.IsURLCompliant(name) {
		return fmt.Errorf("gateway '%s': name must be URL-safe (alphanumeric, dash, underscore only)", name)
	}

	// Validate at least one server exists.
	if len(config.MCPServers) == 0 {
		return fmt.Errorf("gateway '%s': must have at least one MCP server", name)
	}

	// Validate gateway-level processors if present.
	if len(config.Processors) > 0 {
		if err := validateProcessors(config.Processors); err != nil {
			return fmt.Errorf("gateway '%s': %w", name, err)
		}
	}

	return nil
}

// validateServer validates a single server configuration.
func validateServer(name string, server *MCPServerConfig) error {
	// Validate server name is URL compliant (used in endpoint paths).
	if !common.IsURLCompliant(name) {
		return fmt.Errorf("server '%s': name must be URL-safe (alphanumeric, dash, underscore only)", name)
	}

	// Validate transport consistency - must have either Command (stdio) OR URL (http), not both.
	hasCommand := server.Command != ""
	hasURL := server.URL != ""

	if !hasCommand && !hasURL {
		return fmt.Errorf("server '%s': must specify either 'command' (stdio transport) or 'url' (http transport)", name)
	}

	if hasCommand && hasURL {
		return fmt.Errorf("server '%s': cannot specify both 'command' and 'url' - choose either stdio or http transport", name)
	}

	// Validate URL format if URL is specified.
	if hasURL {
		if !isValidHTTPURL(server.URL) {
			return fmt.Errorf("server '%s': invalid URL format - must be a valid http:// or https:// URL", name)
		}

		// Headers only make sense for HTTP transport.
		// (For stdio transport, headers would be ignored).
	}

	// Validate Headers format - all values must be strings.
	for headerKey, headerValue := range server.Headers {
		if headerKey == "" {
			return fmt.Errorf("server '%s': header keys cannot be empty", name)
		}
		if headerValue == "" {
			return fmt.Errorf("server '%s': header '%s' has empty value", name, headerKey)
		}
	}

	return nil
}

// validateProcessors validates processor configurations.
func validateProcessors(processors []*ProcessorConfig) error {
	processorNames := make(map[string]bool)
	for i, processor := range processors {
		if err := validateProcessor(i, processor, processorNames); err != nil {
			return err
		}
	}
	return nil
}

// validateProcessor validates a single processor configuration.
func validateProcessor(index int, processor *ProcessorConfig, processorNames map[string]bool) error {
	// Required fields.
	if processor.Name == "" {
		return fmt.Errorf("processor[%d]: name is required", index)
	}

	// Check for duplicate processor names.
	if processorNames[processor.Name] {
		return fmt.Errorf("processor '%s': duplicate processor name", processor.Name)
	}
	processorNames[processor.Name] = true

	if processor.Type == "" {
		return fmt.Errorf("processor '%s': type is required", processor.Name)
	}

	// Validate type.
	if ProcessorType(processor.Type) != CLIProcessor {
		return fmt.Errorf("processor '%s': unsupported type '%s' (v1 only supports 'cli')", processor.Name, processor.Type)
	}

	// Set default timeout if not specified.
	if processor.Timeout == 0 {
		processor.Timeout = 15 // Default 15 seconds
	}

	// Validate config field is present.
	if processor.Config == nil {
		return fmt.Errorf("processor '%s': config is required", processor.Name)
	}

	// Validate type-specific config.
	return validateProcessorTypeConfig(processor)
}

// validateProcessorTypeConfig validates type-specific processor configuration.
func validateProcessorTypeConfig(processor *ProcessorConfig) error {
	//nolint:gocritic // switch used for future extensibility with additional processor types
	switch ProcessorType(processor.Type) {
	case CLIProcessor:
		// CLI processors require command field in config.
		command, ok := processor.Config["command"]
		if !ok {
			return fmt.Errorf("processor '%s': config.command is required for cli type", processor.Name)
		}
		if _, ok := command.(string); !ok {
			return fmt.Errorf("processor '%s': config.command must be a string", processor.Name)
		}

		// Args is optional but must be array if present.
		if args, exists := processor.Config["args"]; exists {
			if _, ok := args.([]interface{}); !ok {
				return fmt.Errorf("processor '%s': config.args must be an array", processor.Name)
			}
		}
	}
	return nil
}
