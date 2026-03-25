// Package config provides configuration management and MCP proxy functionality
// for the Centian tool.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/T4cceptor/centian/internal/common"
)

// ProcessorType defines the type of processor, e.g. cli, webhook, internal, etc.
type ProcessorType string

const (
	// CLIProcessor represents the type of a CLI-based processor -> "cli".
	CLIProcessor ProcessorType = "cli"
	// WebhookProcessor represents the type of a webhook-based processor -> "webhook".
	WebhookProcessor ProcessorType = "webhook"
	httpScheme       string        = "http"
	httpsScheme      string        = "https"
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

// Default proxy logging settings.
const (
	DefaultProxyLogLevel      = "info"
	DefaultProxyLogOutput     = "file"
	DefaultEventStorageDriver = "sqlite"
)

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
	Name        string                 `json:"name"`                  // MCP Server name - used to reference this specific server
	Command     string                 `json:"command,omitempty"`     // MCP Server Executable command (for stdio/process transport)
	Args        []string               `json:"args,omitempty"`        // MCP Server Command arguments
	Env         map[string]string      `json:"env,omitempty"`         // Environment variables
	URL         string                 `json:"url,omitempty"`         // MCP Server URL (for http/sse transport)
	Headers     map[string]string      `json:"headers,omitempty"`     // HTTP headers (supports ${ENV_VAR} substitution)
	OAuth       *OAuthConfig           `json:"oauth,omitempty"`       // Downstream OAuth settings for HTTP MCP servers
	Enabled     *bool                  `json:"enabled,omitempty"`     // Whether server is active
	Description string                 `json:"description,omitempty"` // Human readable description
	Source      string                 `json:"source,omitempty"`      // Source file path for auto-discovered servers
	Config      map[string]interface{} `json:"config,omitempty"`      // Server-specific config
}

// GetTransport returns McpTransportType based on config data.
func (s *MCPServerConfig) GetTransport() common.McpTransportType {
	r, _ := common.GetTransport(s.URL, s.Command)
	return r
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

// OAuthEnabled returns true when downstream OAuth is configured and enabled.
func (s *MCPServerConfig) OAuthEnabled() bool {
	return s != nil && s.OAuth != nil && s.OAuth.Enabled
}

// OAuthConfig defines downstream OAuth settings for HTTP MCP servers.
type OAuthConfig struct {
	Enabled               bool     `json:"enabled"`
	ClientID              string   `json:"clientId,omitempty"`
	ClientSecret          string   `json:"clientSecret,omitempty"`
	ClientAuthMethod      string   `json:"clientAuthMethod,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	Resource              string   `json:"resource,omitempty"`
	Issuer                string   `json:"issuer,omitempty"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint,omitempty"`
	TokenEndpoint         string   `json:"tokenEndpoint,omitempty"`
}

// ProxySettings contains proxy-level configuration that affects how the
// centian proxy operates, including transport method, logging, and timeouts.
type ProxySettings struct {
	Host         string                `json:"host,omitempty"`         // Bind address for the proxy
	Port         string                `json:"port,omitempty"`         // HTTP proxy port (if enabled)
	LogLevel     string                `json:"logLevel,omitempty"`     // debug, info, warn, error
	LogOutput    string                `json:"logOutput,omitempty"`    // file, console, both
	LogFile      string                `json:"logFile,omitempty"`      // Log file path for internal logger
	Timeout      int                   `json:"timeout,omitempty"`      // Request timeout in seconds
	FeatureFlags *FeatureFlagsSettings `json:"featureFlags,omitempty"` // Proxy-owned feature toggles
	Web          *ProxyWebSettings     `json:"web,omitempty"`          // Public web settings for hosted OAuth flows
	EventStorage *EventStorageSettings `json:"eventStorage,omitempty"` // Event persistence settings
}

// ProxyWebSettings contains public-facing web settings required for browser-based flows.
type ProxyWebSettings struct {
	PublicBaseURL string `json:"publicBaseUrl,omitempty"`
}

// EventStorageSettings controls durable storage for task and action events.
type EventStorageSettings struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Driver  string `json:"driver,omitempty"`
	Path    string `json:"path,omitempty"`
}

// FeatureFlagsSettings groups proxy-owned feature toggles.
type FeatureFlagsSettings struct {
	EnableTestTools  bool `json:"enableTestTools,omitempty"`
	TaskVerification bool `json:"taskVerification,omitempty"`
}

// NewDefaultProxySettings creates a new ProxySettings with default values.
func NewDefaultProxySettings() ProxySettings {
	return ProxySettings{
		Host:      DefaultProxyHost,
		Port:      "8080",
		Timeout:   30,
		LogLevel:  DefaultProxyLogLevel,
		LogOutput: DefaultProxyLogOutput,
		Web:       &ProxyWebSettings{},
		EventStorage: &EventStorageSettings{
			Driver: DefaultEventStorageDriver,
		},
	}
}

// IsEnabled reports whether event storage is enabled. Defaults to true.
func (e *EventStorageSettings) IsEnabled() bool {
	if e == nil || e.Enabled == nil {
		return true
	}
	return *e.Enabled
}

// TestToolsEnabled reports whether proxy-owned test tools are enabled. Defaults to false.
func (p *ProxySettings) TestToolsEnabled() bool {
	return p != nil && p.FeatureFlags != nil && p.FeatureFlags.EnableTestTools
}

// TaskVerificationEnabled reports whether taskverification tools are enabled. Defaults to false.
func (p *ProxySettings) TaskVerificationEnabled() bool {
	return p != nil && p.FeatureFlags != nil && p.FeatureFlags.TaskVerification
}

// GetDriver returns the configured event storage driver or the default.
func (e *EventStorageSettings) GetDriver() string {
	if e == nil || strings.TrimSpace(e.Driver) == "" {
		return DefaultEventStorageDriver
	}
	return strings.TrimSpace(e.Driver)
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
	Parts   []string               `json:"parts,omitempty"`   // Which context parts to provide: "payload", "meta", "routing", "auth" (default: ["payload","meta"])
	Config  map[string]interface{} `json:"config"`            // Type-specific configuration

	// Determines if processor is required to run, "false" by default,
	// meaning the processor can both fail initiation and
	// processing without causing the whole processor chain to fail.
	// If set to true, a failure will cause subsequent processors NOT to run.
	Required bool `json:"required"`
}

// CLIProcessorSettings contains parsed runtime settings for a CLI processor.
type CLIProcessorSettings struct {
	Command string
	Args    []string
}

// WebhookProcessorSettings contains parsed runtime settings for a webhook processor.
type WebhookProcessorSettings struct {
	URL     string
	Headers map[string]string
}

var allowedProcessorParts = map[string]bool{
	"payload": true,
	"meta":    true,
	"routing": true,
	"auth":    true,
}

var allowedWebhookConfigKeys = map[string]bool{
	"url":     true,
	"headers": true,
}

// GetParts returns the configured parts, defaulting to ["payload"] if not specified.
func (p *ProcessorConfig) GetParts() []string {
	if len(p.Parts) == 0 {
		// TODO: create "parts enum"
		return []string{"payload", "meta"}
	}
	return p.Parts
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
		Gateways:    map[string]*GatewayConfig{},
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
		return nil, fmt.Errorf("configuration file not found at %s - try running 'centian init'", path)
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
	if err := ValidateConfig(&cfg, false); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves the configuration to ~/.centian/config.json.
// Creates the ~/.centian directory if it doesn't exist and writes the
// configuration as formatted JSON with proper indentation.
func SaveConfig(config *GlobalConfig) error {
	if err := ValidateConfig(config, false); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}
	return saveConfig(config)
}

func saveConfig(config *GlobalConfig) error {
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

	// Write to file. Config may contain downstream client secrets.
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfig performs basic schema validation on the configuration.
// This validates required fields and structure but allows empty gateways.
// Use ValidateConfigForServer for operational validation before starting a server.
func ValidateConfig(config *GlobalConfig, strict bool) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}
	if config.Version == "" {
		return fmt.Errorf("version field is required")
	}
	if config.Proxy == nil {
		return fmt.Errorf("proxy settings are required in config")
	}
	if err := validateProxySettings(config.Proxy); err != nil {
		return err
	}
	if err := validateNameConventions(config.Gateways); err != nil {
		return err
	}
	if HasOAuthServers(config) {
		if config.Proxy.Web == nil || config.Proxy.Web.PublicBaseURL == "" {
			return fmt.Errorf("proxy.web.publicBaseUrl is required when downstream oauth is enabled")
		}
		if !isValidHTTPURL(config.Proxy.Web.PublicBaseURL) {
			return fmt.Errorf("proxy.web.publicBaseUrl must be a valid http:// or https:// URL")
		}
	}

	if strict {
		// Validate config for operational purposes - meaning: can we start the server with this?
		if err := validateGateways(config.Gateways); err != nil {
			return err
		}
		if err := validateProcessors(config.Processors); err != nil {
			return err
		}
	}
	return nil
}

func validateProxySettings(proxy *ProxySettings) error {
	proxy.LogLevel = strings.ToLower(strings.TrimSpace(proxy.LogLevel))
	if proxy.LogLevel == "" {
		proxy.LogLevel = DefaultProxyLogLevel
	}
	switch proxy.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("proxy.logLevel: unsupported value %q (expected debug, info, warn, or error)", proxy.LogLevel)
	}

	proxy.LogOutput = strings.ToLower(strings.TrimSpace(proxy.LogOutput))
	if proxy.LogOutput == "" {
		proxy.LogOutput = DefaultProxyLogOutput
	}
	switch proxy.LogOutput {
	case "file", "console", "both":
	default:
		return fmt.Errorf("proxy.logOutput: unsupported value %q (expected file, console, or both)", proxy.LogOutput)
	}

	proxy.LogFile = strings.TrimSpace(proxy.LogFile)
	if proxy.Web != nil {
		proxy.Web.PublicBaseURL = strings.TrimSpace(proxy.Web.PublicBaseURL)
	}
	return nil
}

// validateNameConventions validates gateway and server names.
// This is run for both strict and non-strict config validation.
func validateNameConventions(gateways map[string]*GatewayConfig) error {
	for gatewayName, gatewayConfig := range gateways {
		if !common.IsURLCompliant(gatewayName) {
			return fmt.Errorf("gateway '%s': name must be URL-safe (alphanumeric, dash, underscore only)", gatewayName)
		}
		if gatewayConfig == nil {
			return fmt.Errorf("gateway '%s': config cannot be nil", gatewayName)
		}
		for serverName := range gatewayConfig.MCPServers {
			if !common.IsURLCompliant(serverName) {
				return fmt.Errorf("server '%s': name must be URL-safe (alphanumeric, dash, underscore only)", serverName)
			}
		}
	}
	return nil
}

// validateGateways validates gateway configurations without requiring any.
// This allows empty gateway maps (for freshly initialized configs).
func validateGateways(gateways map[string]*GatewayConfig) error {
	if len(gateways) == 0 {
		return fmt.Errorf("no gateways configured - at least one gateway is required")
	}
	for gatewayName, gatewayConfig := range gateways {
		if gatewayConfig == nil {
			return fmt.Errorf("gateway '%s': config cannot be nil", gatewayName)
		}
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
	return (parsedURL.Scheme == httpScheme || parsedURL.Scheme == httpsScheme) && parsedURL.Host != ""
}

// validateGateway validates a gateway configuration.
func validateGateway(name string, config GatewayConfig) error {
	// Validate gateway name is URL compliant (used in endpoint paths).
	if !common.IsURLCompliant(name) {
		return fmt.Errorf("gateway '%s': name must be URL-safe (alphanumeric, dash, underscore only)", name)
	}

	// Validate at least one server exists.
	activeMCPServers := []*MCPServerConfig{}
	for _, serverConfig := range config.MCPServers {
		if serverConfig.Enabled == nil || *serverConfig.Enabled {
			activeMCPServers = append(activeMCPServers, serverConfig)
		}
	}
	if len(activeMCPServers) == 0 {
		return fmt.Errorf("gateway '%s': must have at least one active MCP server", name)
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
	if server == nil {
		return fmt.Errorf("server '%s': config cannot be nil", name)
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

	if err := validateOAuthConfig(name, server); err != nil {
		return err
	}

	return nil
}

func validateOAuthConfig(name string, server *MCPServerConfig) error {
	if server == nil || !server.OAuthEnabled() {
		return nil
	}
	if err := validateOAuthTransport(name, server); err != nil {
		return err
	}

	oauthConfig := server.OAuth
	normalizeOAuthConfig(oauthConfig)
	if err := validateOAuthRequiredFields(name, oauthConfig); err != nil {
		return err
	}
	if err := validateOAuthClientAuthMethod(name, oauthConfig.ClientAuthMethod); err != nil {
		return err
	}
	return validateOAuthEndpoints(name, oauthConfig)
}

func validateOAuthTransport(name string, server *MCPServerConfig) error {
	if server.URL == "" || server.Command != "" {
		return fmt.Errorf("server '%s': downstream OAuth is only supported for HTTP MCP servers", name)
	}
	return nil
}

func normalizeOAuthConfig(oauthConfig *OAuthConfig) {
	oauthConfig.ClientID = strings.TrimSpace(oauthConfig.ClientID)
	oauthConfig.ClientSecret = strings.TrimSpace(oauthConfig.ClientSecret)
	oauthConfig.ClientAuthMethod = strings.ToLower(strings.TrimSpace(oauthConfig.ClientAuthMethod))
	oauthConfig.Resource = strings.TrimSpace(oauthConfig.Resource)
	oauthConfig.Issuer = strings.TrimSpace(oauthConfig.Issuer)
	oauthConfig.AuthorizationEndpoint = strings.TrimSpace(oauthConfig.AuthorizationEndpoint)
	oauthConfig.TokenEndpoint = strings.TrimSpace(oauthConfig.TokenEndpoint)
}

func validateOAuthRequiredFields(name string, oauthConfig *OAuthConfig) error {
	switch {
	case oauthConfig.ClientID == "":
		return fmt.Errorf("server '%s': oauth.clientId is required when oauth is enabled", name)
	case oauthConfig.ClientSecret == "":
		return fmt.Errorf("server '%s': oauth.clientSecret is required when oauth is enabled", name)
	case oauthConfig.Resource == "":
		return fmt.Errorf("server '%s': oauth.resource is required when oauth is enabled", name)
	default:
		return nil
	}
}

func validateOAuthClientAuthMethod(name, method string) error {
	switch method {
	case "", "client_secret_basic", "client_secret_post":
		return nil
	default:
		return fmt.Errorf("server '%s': oauth.clientAuthMethod must be client_secret_basic or client_secret_post", name)
	}
}

func validateOAuthEndpoints(name string, oauthConfig *OAuthConfig) error {
	if err := validateOAuthIssuer(name, oauthConfig); err != nil {
		return err
	}
	if err := validateOptionalOAuthURL(name, "oauth.authorizationEndpoint", oauthConfig.AuthorizationEndpoint); err != nil {
		return err
	}
	return validateOptionalOAuthURL(name, "oauth.tokenEndpoint", oauthConfig.TokenEndpoint)
}

func validateOAuthIssuer(name string, oauthConfig *OAuthConfig) error {
	if oauthConfig.Issuer == "" {
		if !isValidHTTPURL(oauthConfig.AuthorizationEndpoint) || !isValidHTTPURL(oauthConfig.TokenEndpoint) {
			return fmt.Errorf("server '%s': oauth.issuer or both oauth.authorizationEndpoint and oauth.tokenEndpoint are required", name)
		}
		return nil
	}
	if !isValidHTTPURL(oauthConfig.Issuer) {
		return fmt.Errorf("server '%s': oauth.issuer must be a valid http:// or https:// URL", name)
	}
	return nil
}

func validateOptionalOAuthURL(name, fieldName, value string) error {
	if value == "" || isValidHTTPURL(value) {
		return nil
	}
	return fmt.Errorf("server '%s': %s must be a valid http:// or https:// URL", name, fieldName)
}

// HasOAuthServers reports whether any configured downstream server enables OAuth.
func HasOAuthServers(config *GlobalConfig) bool {
	if config == nil {
		return false
	}
	for _, gateway := range config.Gateways {
		if gateway == nil {
			continue
		}
		for _, server := range gateway.MCPServers {
			if server != nil && server.OAuthEnabled() {
				return true
			}
		}
	}
	return false
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
	switch ProcessorType(processor.Type) {
	case CLIProcessor, WebhookProcessor:
	default:
		return fmt.Errorf("processor '%s': unsupported type '%s' (supported: 'cli', 'webhook')", processor.Name, processor.Type)
	}

	// Set default timeout if not specified.
	if processor.Timeout == 0 {
		processor.Timeout = 15 // Default 15 seconds
	}

	if err := validateProcessorParts(processor); err != nil {
		return err
	}

	// Validate config field is present.
	if processor.Config == nil {
		return fmt.Errorf("processor '%s': config is required", processor.Name)
	}

	// Validate type-specific config.
	return validateProcessorTypeConfig(processor)
}

func validateProcessorParts(processor *ProcessorConfig) error {
	for _, part := range processor.GetParts() {
		if !allowedProcessorParts[part] {
			return fmt.Errorf("processor '%s': unsupported part '%s' (allowed: payload, meta, routing, auth)", processor.Name, part)
		}
	}
	return nil
}

// HasProcessor returns true if a processor with the given name exists in the global config.
func (g *GlobalConfig) HasProcessor(name string) bool {
	for _, p := range g.Processors {
		if p.Name == name {
			return true
		}
	}
	return false
}

// AddProcessor appends a processor to the global processor chain.
func (g *GlobalConfig) AddProcessor(p *ProcessorConfig) {
	g.Processors = append(g.Processors, p)
}

// ReplaceProcessor replaces an existing processor by name, preserving its position in the chain.
// Returns false if no processor with the given name was found.
func (g *GlobalConfig) ReplaceProcessor(name string, p *ProcessorConfig) bool {
	for i, existing := range g.Processors {
		if existing.Name == name {
			g.Processors[i] = p
			return true
		}
	}
	return false
}

// InferProcessorNameFromPath extracts a processor name from a file path.
// Strips the directory and extension, lowercases the result.
func InferProcessorNameFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return strings.ToLower(name)
}

// InferProcessorNameFromWebhookURL extracts a processor name from a webhook URL.
// Prefers the last path segment and falls back to hostname when no path segment exists.
func InferProcessorNameFromWebhookURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "webhook-processor"
	}

	path := strings.Trim(parsed.Path, "/")
	if path != "" {
		if base := filepath.Base(path); base != "." && base != "/" && base != "" {
			return strings.ToLower(base)
		}
	}

	if host := parsed.Hostname(); host != "" {
		return strings.ToLower(host)
	}

	return "webhook-processor"
}

// InferCommandFromPath determines the runtime command and args for a CLI processor
// based on the file extension of the script path.
func InferCommandFromPath(path string) (command string, args []string, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py":
		return "python3", []string{path}, nil
	case ".js":
		return "node", []string{path}, nil
	case ".ts":
		return "npx", []string{"ts-node", path}, nil
	case ".sh":
		return "bash", []string{path}, nil
	default:
		return "", nil, fmt.Errorf("unsupported file extension '%s' - supported: .py, .js, .ts, .sh", ext)
	}
}

// validateProcessorTypeConfig validates type-specific processor configuration.
func validateProcessorTypeConfig(processor *ProcessorConfig) error {
	//nolint:gocritic // switch used for future extensibility with additional processor types
	switch ProcessorType(processor.Type) {
	case CLIProcessor:
		_, err := ParseCLIProcessorSettings(processor)
		return err
	case WebhookProcessor:
		_, err := ParseWebhookProcessorSettings(processor)
		return err
	}
	return nil
}

// ParseCLIProcessorSettings validates and extracts CLI processor settings.
func ParseCLIProcessorSettings(processor *ProcessorConfig) (*CLIProcessorSettings, error) {
	command, ok := processor.Config["command"]
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.command is required for cli type", processor.Name)
	}
	commandValue, ok := command.(string)
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.command must be a string", processor.Name)
	}

	args, err := processorConfigStringSlice(processor.Name, processor.Config["args"])
	if err != nil {
		return nil, err
	}

	return &CLIProcessorSettings{
		Command: commandValue,
		Args:    args,
	}, nil
}

// ParseWebhookProcessorSettings validates and extracts webhook processor settings.
func ParseWebhookProcessorSettings(processor *ProcessorConfig) (*WebhookProcessorSettings, error) {
	urlValue, ok := processor.Config["url"]
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.url is required for webhook type", processor.Name)
	}
	urlString, ok := urlValue.(string)
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.url must be a string", processor.Name)
	}
	parsedURL, err := url.Parse(urlString)
	if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("processor '%s': config.url must be a valid http/https URL", processor.Name)
	}
	if parsedURL.Scheme != httpScheme && parsedURL.Scheme != httpsScheme {
		return nil, fmt.Errorf("processor '%s': config.url must use http or https", processor.Name)
	}

	headers := make(map[string]string)
	if headersValue, exists := processor.Config["headers"]; exists {
		headers, err = ProcessorConfigStringMap(headersValue)
		if err != nil {
			return nil, fmt.Errorf("processor '%s': config.headers must be an object with string values", processor.Name)
		}
	}

	for key := range processor.Config {
		if !allowedWebhookConfigKeys[key] {
			return nil, fmt.Errorf("processor '%s': config.%s is unsupported for webhook type", processor.Name, key)
		}
	}

	return &WebhookProcessorSettings{
		URL:     urlString,
		Headers: headers,
	}, nil
}

func processorConfigStringSlice(processorName string, value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}

	argsArray, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.args must be an array", processorName)
	}

	args := make([]string, 0, len(argsArray))
	for _, arg := range argsArray {
		argStr, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("processor '%s': config.args must contain only strings", processorName)
		}
		args = append(args, argStr)
	}
	return args, nil
}

// ProcessorConfigStringMap converts a config value into a string map.
// Accepts both map[string]string and map[string]interface{} with string values.
func ProcessorConfigStringMap(value interface{}) (map[string]string, error) {
	switch typed := value.(type) {
	case map[string]string:
		result := make(map[string]string, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return result, nil
	case map[string]interface{}:
		result := make(map[string]string, len(typed))
		for key, item := range typed {
			strValue, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("value for %q must be a string", key)
			}
			result[key] = strValue
		}
		return result, nil
	default:
		return nil, fmt.Errorf("value must be an object")
	}
}
