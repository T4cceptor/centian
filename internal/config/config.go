// Package config provides configuration management and MCP proxy functionality
// for the Centian tool.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
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
	// BuiltinProcessor represents an in-process processor shipped with Centian -> "builtin".
	BuiltinProcessor ProcessorType = "builtin"
	httpScheme       string        = "http"
	httpsScheme      string        = "https"
)

// DefaultProjectSlug is the project slug used when no explicit projects are configured.
const DefaultProjectSlug = "default"

// Gateway verification requirement values control how a gateway participates
// in project task verification.
const (
	VerificationRequirementOff      = "off"
	VerificationRequirementOptional = "optional"
	VerificationRequirementRequired = "required"
)

// GlobalConfig represents the main configuration structure stored at ~/.centian/config.json.
// This is the root configuration object that contains all settings for MCP servers,
// proxy behavior, processors, and additional metadata.
//
// GlobalConfig supports two layouts:
//  1. Flat (legacy): gateways, processors, auth, and capabilities live directly
//     on GlobalConfig. These are auto-wrapped into a "default" ProjectConfig at
//     load time via ResolveProjects().
//  2. Project-based: one or more named ProjectConfig entries under the "projects"
//     field. Each project gets its own route prefix, database, feature flags,
//     gateways, and processors.
type GlobalConfig struct {
	Name    string `json:"name"`    // Name of the server - simplifies server identification
	Version string `json:"version"` // Config schema version

	// Truly global settings - apply to the whole server process.
	Proxy       *ProxySettings       `json:"proxy,omitempty"`       // Proxy-level settings (host, port, logLevel, logOutput, timeout)
	AuthBackend *AuthBackendSettings `json:"authBackend,omitempty"` // Global principal/credential storage backend

	// Project-based layout: each project is an isolated tenant.
	Projects map[string]*ProjectConfig `json:"projects,omitempty"` // Named project configs

	// Legacy flat fields - auto-migrated into a "default" project by ResolveProjects().
	// When Projects is non-empty these MUST be empty (enforced by validation).
	AuthEnabled *bool                     `json:"auth,omitempty"`       // Enable or disable proxy auth
	AuthHeader  string                    `json:"authHeader,omitempty"` // Header name for proxy auth
	Gateways    map[string]*GatewayConfig `json:"gateways,omitempty"`   // HTTP proxy gateways
	Processors  []*ProcessorConfig        `json:"processors,omitempty"` // Processor chain
	Metadata    map[string]interface{}    `json:"metadata,omitempty"`   // Additional metadata
}

// ProjectConfig represents an isolated project (tenant) within the Centian server.
// Each project gets its own:
//   - Route prefix: /<project_slug>/mcp/<gateway> and /<project_slug>/ui
//   - SQLite database for event storage
//   - Feature flags (capabilities)
//   - Auth settings
//   - Gateways, processors, and metadata
type ProjectConfig struct {
	Slug        string `json:"slug,omitempty"`        // URL-safe project slug (derived from map key if empty)
	Description string `json:"description,omitempty"` // Human readable project description
	AuthEnabled *bool  `json:"auth,omitempty"`        // Enable or disable project-level auth
	AuthHeader  string `json:"authHeader,omitempty"`  // Header name for project-level auth

	Capabilities *CapabilitiesSettings     `json:"capabilities,omitempty"` // Project-scoped feature flags
	Web          *ProxyWebSettings         `json:"web,omitempty"`          // Public web settings (OAuth flows)
	Gateways     map[string]*GatewayConfig `json:"gateways,omitempty"`     // HTTP proxy gateways
	Processors   []*ProcessorConfig        `json:"processors,omitempty"`   // Processor chain
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`     // Additional metadata
}

// IsAuthEnabled returns true when auth is enabled for this project (defaults to true).
func (p *ProjectConfig) IsAuthEnabled() bool {
	if p == nil || p.AuthEnabled == nil {
		return true
	}
	return *p.AuthEnabled
}

// GetAuthHeader returns the configured auth header name or the default.
func (p *ProjectConfig) GetAuthHeader() string {
	if p == nil || p.AuthHeader == "" {
		return DefaultAuthHeader
	}
	return p.AuthHeader
}

// TaskVerificationCapability returns the configured taskverification capability block.
func (p *ProjectConfig) TaskVerificationCapability() *TaskVerificationCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.TaskVerification
}

// EventStorageCapability returns the configured event storage capability block.
func (p *ProjectConfig) EventStorageCapability() *EventStorageCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.EventStorage
}

// TestToolsCapability returns the configured test tools capability block.
func (p *ProjectConfig) TestToolsCapability() *TestToolsCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.TestTools
}

// UICapability returns the configured embedded UI capability block.
func (p *ProjectConfig) UICapability() *UICapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.UI
}

// TaskVerificationEnabled reports whether taskverification tools are enabled. Defaults to false.
func (p *ProjectConfig) TaskVerificationEnabled() bool {
	return p != nil && p.TaskVerificationCapability().IsEnabled()
}

// UIEnabled reports whether the embedded UI should be served. Defaults to false.
func (p *ProjectConfig) UIEnabled() bool {
	return p != nil && p.UICapability().IsEnabled()
}

// TestToolsEnabled reports whether proxy-owned test tools are enabled. Defaults to false.
func (p *ProjectConfig) TestToolsEnabled() bool {
	return p != nil && p.TestToolsCapability().IsEnabled()
}

// HasOAuthServers reports whether any gateway in this project has OAuth enabled.
func (p *ProjectConfig) HasOAuthServers() bool {
	if p == nil {
		return false
	}
	for _, gateway := range p.Gateways {
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

// ResolveProjects normalizes the GlobalConfig so that callers can always work
// with the Projects map. When the config uses the legacy flat layout (no
// explicit projects), a single "default" project is synthesized from the
// top-level gateways, processors, auth, and capability fields.
//
// After this call, the legacy flat fields on GlobalConfig are cleared.
func (g *GlobalConfig) ResolveProjects() {
	if g == nil {
		return
	}

	// Already using project-based layout.
	if len(g.Projects) > 0 {
		// Ensure slugs are populated from map keys.
		for slug, project := range g.Projects {
			if project.Slug == "" {
				project.Slug = slug
			}
		}
		return
	}

	// Synthesize a default project from legacy flat fields.
	project := &ProjectConfig{
		Slug:        DefaultProjectSlug,
		AuthEnabled: g.AuthEnabled,
		AuthHeader:  g.AuthHeader,
		Gateways:    g.Gateways,
		Processors:  g.Processors,
		Metadata:    g.Metadata,
	}

	// Move capabilities and web settings from ProxySettings into the project.
	if g.Proxy != nil {
		project.Capabilities = g.Proxy.Capabilities
		project.Web = g.Proxy.Web
	}

	g.Projects = map[string]*ProjectConfig{
		DefaultProjectSlug: project,
	}

	// Clear legacy fields to prevent confusion.
	g.AuthEnabled = nil
	g.AuthHeader = ""
	g.Gateways = nil
	g.Processors = nil
	g.Metadata = nil
	if g.Proxy != nil {
		g.Proxy.Capabilities = nil
		g.Proxy.Web = nil
	}
}

// IsLegacyLayout returns true when the config uses the flat (non-project) layout.
func (g *GlobalConfig) IsLegacyLayout() bool {
	return g != nil && len(g.Projects) == 0 && len(g.Gateways) > 0
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

// AuthBackendSettings configures where principals and their credentials are
// stored. This is a truly global setting because authentication resolves a token
// to a principal at the HTTP layer, before any project is selected. Type and Store
// may be empty; the auth package resolves empties to defaults (sqlite at the
// default principals database path).
type AuthBackendSettings struct {
	Type  string `json:"type,omitempty"`  // "sqlite" (default) or "file"
	Store string `json:"store,omitempty"` // Backend location (sqlite db path or key file path)
}

// GetAuthBackend returns the configured auth backend type and store. Both may be
// empty when no backend block is configured; callers resolve empties to defaults.
func (g *GlobalConfig) GetAuthBackend() (backendType, store string) {
	if g == nil || g.AuthBackend == nil {
		return "", ""
	}
	return g.AuthBackend.Type, g.AuthBackend.Store
}

// IsAuthEnabled returns true when auth is enabled or unset.
// After ResolveProjects(), this checks the legacy flat field; prefer
// checking ProjectConfig.IsAuthEnabled() for project-aware code.
func (g *GlobalConfig) IsAuthEnabled() bool {
	if g == nil || g.AuthEnabled == nil {
		return true
	}
	return *g.AuthEnabled
}

// GetAuthHeader returns the configured auth header name or the default.
// After ResolveProjects(), this checks the legacy flat field; prefer
// checking ProjectConfig.GetAuthHeader() for project-aware code.
func (g *GlobalConfig) GetAuthHeader() string {
	if g == nil || g.AuthHeader == "" {
		return DefaultAuthHeader
	}
	return g.AuthHeader
}

// GetDefaultProject returns the "default" project after ResolveProjects() has been called.
// Returns nil if no default project exists.
func (g *GlobalConfig) GetDefaultProject() *ProjectConfig {
	if g == nil || g.Projects == nil {
		return nil
	}
	return g.Projects[DefaultProjectSlug]
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
	Capabilities *CapabilitiesSettings `json:"capabilities,omitempty"` // Optional proxy-owned capabilities
	Web          *ProxyWebSettings     `json:"web,omitempty"`          // Public web settings for hosted OAuth flows
}

// ProxyWebSettings contains public-facing web settings required for browser-based flows.
type ProxyWebSettings struct {
	PublicBaseURL string `json:"publicBaseUrl,omitempty"`
}

// CapabilitiesSettings groups optional proxy-owned capabilities.
type CapabilitiesSettings struct {
	TaskVerification *TaskVerificationCapabilitySettings `json:"taskVerification,omitempty"`
	EventStorage     *EventStorageCapabilitySettings     `json:"eventStorage,omitempty"`
	TestTools        *TestToolsCapabilitySettings        `json:"testTools,omitempty"`
	UI               *UICapabilitySettings               `json:"ui,omitempty"`
}

// TaskVerificationCapabilitySettings controls taskverification capability behavior.
type TaskVerificationCapabilitySettings struct {
	Enabled            *bool  `json:"enabled,omitempty"`
	TemplatesPath      string `json:"templatesPath,omitempty"`
	IdleTimeoutSeconds int    `json:"idleTimeoutSeconds,omitempty"`
}

// EventStorageCapabilitySettings controls durable storage for task and action events.
type EventStorageCapabilitySettings struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Driver  string `json:"driver,omitempty"`
	Path    string `json:"path,omitempty"`
}

// TestToolsCapabilitySettings controls Centian-owned test/debug tools.
type TestToolsCapabilitySettings struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// UICapabilitySettings controls whether the embedded web UI is exposed.
type UICapabilitySettings struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// NewDefaultProxySettings creates a new ProxySettings with default values.
// Note: Capabilities and Web settings now live in ProjectConfig, not ProxySettings.
// Use NewDefaultCapabilities() for project-level defaults.
func NewDefaultProxySettings() ProxySettings {
	return ProxySettings{
		Host:      DefaultProxyHost,
		Port:      "9666",
		Timeout:   30,
		LogLevel:  DefaultProxyLogLevel,
		LogOutput: DefaultProxyLogOutput,
	}
}

// NewDefaultCapabilities creates default capability settings for a project.
func NewDefaultCapabilities() *CapabilitiesSettings {
	taskVerificationEnabled := false
	testToolsEnabled := false
	eventStorageEnabled := true
	uiEnabled := false
	return &CapabilitiesSettings{
		TaskVerification: &TaskVerificationCapabilitySettings{
			Enabled: &taskVerificationEnabled,
		},
		EventStorage: &EventStorageCapabilitySettings{
			Enabled: &eventStorageEnabled,
			Driver:  DefaultEventStorageDriver,
		},
		TestTools: &TestToolsCapabilitySettings{
			Enabled: &testToolsEnabled,
		},
		UI: &UICapabilitySettings{
			Enabled: &uiEnabled,
		},
	}
}

// NewDefaultProjectConfig creates a default ProjectConfig with standard capabilities.
func NewDefaultProjectConfig() *ProjectConfig {
	authEnabled := true
	return &ProjectConfig{
		Slug:         DefaultProjectSlug,
		AuthEnabled:  &authEnabled,
		AuthHeader:   DefaultAuthHeader,
		Capabilities: NewDefaultCapabilities(),
		Web:          &ProxyWebSettings{},
		Gateways:     map[string]*GatewayConfig{},
		Processors:   []*ProcessorConfig{},
		Metadata:     make(map[string]interface{}),
	}
}

// IsEnabled reports whether event storage is enabled. Defaults to true.
func (e *EventStorageCapabilitySettings) IsEnabled() bool {
	if e == nil || e.Enabled == nil {
		return true
	}
	return *e.Enabled
}

// IsEnabled reports whether taskverification is enabled. Defaults to false.
func (t *TaskVerificationCapabilitySettings) IsEnabled() bool {
	if t == nil || t.Enabled == nil {
		return false
	}
	return *t.Enabled
}

// IsEnabled reports whether Centian-owned test tools are enabled. Defaults to false.
func (t *TestToolsCapabilitySettings) IsEnabled() bool {
	if t == nil || t.Enabled == nil {
		return false
	}
	return *t.Enabled
}

// IsEnabled reports whether the embedded UI is enabled. Defaults to false.
func (u *UICapabilitySettings) IsEnabled() bool {
	if u == nil || u.Enabled == nil {
		return false
	}
	return *u.Enabled
}

// GetDriver returns the configured event storage driver or the default.
func (e *EventStorageCapabilitySettings) GetDriver() string {
	if e == nil || strings.TrimSpace(e.Driver) == "" {
		return DefaultEventStorageDriver
	}
	return strings.TrimSpace(e.Driver)
}

// GetTemplatesPath returns the configured task template directory override.
func (t *TaskVerificationCapabilitySettings) GetTemplatesPath() string {
	if t == nil {
		return ""
	}
	return strings.TrimSpace(t.TemplatesPath)
}

// GetIdleTimeoutSeconds returns the configured task idle timeout in seconds.
func (t *TaskVerificationCapabilitySettings) GetIdleTimeoutSeconds() int {
	if t == nil || t.IdleTimeoutSeconds <= 0 {
		return 0
	}
	return t.IdleTimeoutSeconds
}

// TaskVerificationCapability returns the configured taskverification capability block.
func (p *ProxySettings) TaskVerificationCapability() *TaskVerificationCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.TaskVerification
}

// EventStorageCapability returns the configured event storage capability block.
func (p *ProxySettings) EventStorageCapability() *EventStorageCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.EventStorage
}

// TestToolsCapability returns the configured test tools capability block.
func (p *ProxySettings) TestToolsCapability() *TestToolsCapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.TestTools
}

// UICapability returns the configured embedded UI capability block.
func (p *ProxySettings) UICapability() *UICapabilitySettings {
	if p == nil || p.Capabilities == nil {
		return nil
	}
	return p.Capabilities.UI
}

// TestToolsEnabled reports whether proxy-owned test tools are enabled. Defaults to false.
func (p *ProxySettings) TestToolsEnabled() bool {
	return p != nil && p.TestToolsCapability().IsEnabled()
}

// TaskVerificationEnabled reports whether taskverification tools are enabled. Defaults to false.
func (p *ProxySettings) TaskVerificationEnabled() bool {
	return p != nil && p.TaskVerificationCapability().IsEnabled()
}

// UIEnabled reports whether the embedded UI should be served. Defaults to false.
func (p *ProxySettings) UIEnabled() bool {
	return p != nil && p.UICapability().IsEnabled()
}

// GatewayConfig represents a logical grouping of HTTP MCP servers.
type GatewayConfig struct {
	AllowDynamic            bool                        `json:"allowDynamic,omitempty"`            // Allow dynamic proxy endpoints
	AllowGatewayEndpoint    bool                        `json:"setupGateway,omitempty"`            // Setup gateway endpoint with namespacing
	ForceReadOnlyHints      *bool                       `json:"forceReadOnlyHints,omitempty"`      // Override all tool annotations to readOnlyHint=true
	ForceSafeToolHints      *bool                       `json:"forceSafeToolHints,omitempty"`      // Override all tool annotations to conservative safe defaults for MCP clients
	VerificationRequirement string                      `json:"verificationRequirement,omitempty"` // Gateway task verification policy: off, optional, required
	MCPServers              map[string]*MCPServerConfig `json:"mcpServers"`                        // HTTP MCP servers in this gateway
	Processors              []*ProcessorConfig          `json:"processors,omitempty"`
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

// ForceReadOnlyHintsEnabled reports whether all tool annotations should be
// overridden to readOnlyHint=true for this gateway. Defaults to false.
func (g *GatewayConfig) ForceReadOnlyHintsEnabled() bool {
	return g != nil && g.ForceReadOnlyHints != nil && *g.ForceReadOnlyHints
}

// ForceSafeToolHintsEnabled reports whether all tool annotations should be
// overridden to conservative safe defaults for this gateway. Defaults to false.
func (g *GatewayConfig) ForceSafeToolHintsEnabled() bool {
	return g != nil && g.ForceSafeToolHints != nil && *g.ForceSafeToolHints
}

// NormalizedVerificationRequirement returns the configured requirement after trimming
// and lowercasing. Empty means the gateway should use the default policy.
func (g *GatewayConfig) NormalizedVerificationRequirement() string {
	if g == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(g.VerificationRequirement))
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
// For BuiltinProcessor processors (see type BuiltinProcessorSettings):
//   - "processor" (string, required): Built-in processor identifier (e.g., "prompt_injection_guard").
//   - "mode" (string, optional): Built-in processor mode.
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
	Type    string                 `json:"type"`              // Processor type: "cli", "webhook", or "builtin"
	Enabled bool                   `json:"enabled"`           // Whether processor is active
	Timeout int                    `json:"timeout,omitempty"` // Timeout in seconds (default: 15)
	Parts   []string               `json:"parts,omitempty"`   // Which context parts to provide: "payload", "meta", "routing", "auth", "annotations" (default: ["payload","meta"])
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

// BuiltinProcessorSettings contains parsed runtime settings for a built-in processor.
type BuiltinProcessorSettings struct {
	Processor    string
	Mode         string
	Scope        string
	Rules        []BuiltinRedactionRule
	Presets      []string
	GuardRules   []BuiltinToolGuardRule
	PathBoundary *BuiltinPathBoundarySettings
}

// BuiltinPromptInjectionGuard is the in-process prompt injection detection processor.
const BuiltinPromptInjectionGuard = "prompt_injection_guard"

const (
	// BuiltinPatternRedactionProcessor redacts user-configured regex patterns.
	BuiltinPatternRedactionProcessor = "pattern_redaction_processor"
	// BuiltinSecretTokenRedactor redacts common secret and token patterns.
	BuiltinSecretTokenRedactor = "secret_token_redactor"
	// BuiltinPIIRedactor redacts deterministic PII-like patterns.
	BuiltinPIIRedactor = "pii_redactor"
	// BuiltinToolCallGuard blocks or annotates configured tool-call policy matches.
	BuiltinToolCallGuard = "tool_call_guard"
)

const (
	BuiltinRedactionModeRedact   = "redact"
	BuiltinRedactionModeAnnotate = "annotate"

	BuiltinRedactionScopeRequest  = "request"
	BuiltinRedactionScopeResponse = "response"
	BuiltinRedactionScopeBoth     = "both"
)

const (
	BuiltinToolGuardModeBlock    = "block"
	BuiltinToolGuardModeAnnotate = "annotate"

	BuiltinToolGuardPresetDangerousCommands = "dangerous_commands"
	BuiltinToolGuardPresetPathBoundary      = "path_boundary"
)

// BuiltinRedactionRule contains one configurable pattern redaction rule.
type BuiltinRedactionRule struct {
	Name        string
	Pattern     string
	Replacement string
}

// BuiltinToolGuardArgumentRule contains one tool-call argument matcher.
type BuiltinToolGuardArgumentRule struct {
	Path    string
	Pattern string
}

// BuiltinToolGuardRule contains one configurable tool-call deny rule.
type BuiltinToolGuardRule struct {
	Name          string
	Category      string
	Severity      string
	Message       string
	ToolPatterns  []string
	ArgumentRules []BuiltinToolGuardArgumentRule
}

// BuiltinPathBoundarySettings contains path-aware tool guard settings.
type BuiltinPathBoundarySettings struct {
	AllowedRoots     []string
	RelativeBaseRoot string
	ToolPatterns     []string
	ArgumentPaths    []string
	DeniedPaths      []string
}

var allowedProcessorParts = map[string]bool{
	"payload":     true,
	"meta":        true,
	"routing":     true,
	"auth":        true,
	"annotations": true,
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

// DefaultConfig returns a default configuration using the legacy flat layout.
// Callers that need the project-based layout should call ResolveProjects() after
// populating gateways, or use DefaultProjectBasedConfig().
func DefaultConfig() *GlobalConfig {
	authEnabled := true
	proxySettings := NewDefaultProxySettings()
	proxySettings.Capabilities = NewDefaultCapabilities()
	proxySettings.Web = &ProxyWebSettings{}
	return &GlobalConfig{
		Name:        "Centian Server",
		Version:     "1.0.0",
		AuthEnabled: &authEnabled,
		AuthHeader:  DefaultAuthHeader,
		Proxy:       &proxySettings,
		Gateways:    map[string]*GatewayConfig{},
		Processors:  []*ProcessorConfig{},
		Metadata:    make(map[string]interface{}),
	}
}

// DefaultProjectBasedConfig returns a default configuration using the project-based layout.
func DefaultProjectBasedConfig() *GlobalConfig {
	proxySettings := NewDefaultProxySettings()
	return &GlobalConfig{
		Name:    "Centian Server",
		Version: "1.0.0",
		Proxy:   &proxySettings,
		Projects: map[string]*ProjectConfig{
			DefaultProjectSlug: NewDefaultProjectConfig(),
		},
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
//
// Supports both legacy flat layout and project-based layout.
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

	// Reject mixed layout: cannot have both top-level gateways AND projects.
	if len(config.Projects) > 0 && len(config.Gateways) > 0 {
		return fmt.Errorf("config cannot have both top-level 'gateways' and 'projects' - use one layout")
	}

	// Project-based layout validation.
	if len(config.Projects) > 0 {
		return validateProjects(config.Projects, strict)
	}

	// Legacy flat layout validation.
	return validateFlatLayout(config, strict)
}

// validateProjects validates all project configs in the project-based layout.
func validateProjects(projects map[string]*ProjectConfig, strict bool) error {
	for slug, project := range projects {
		if err := validateProjectSlug(slug); err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("project '%s': config cannot be nil", slug)
		}
		if err := validateProjectConfig(slug, project, strict); err != nil {
			return err
		}
	}
	return nil
}

// validateFlatLayout validates the legacy flat config layout.
func validateFlatLayout(config *GlobalConfig, strict bool) error {
	normalizeGateways(config.Gateways)

	if err := validateNameConventions(config.Gateways); err != nil {
		return err
	}
	if HasOAuthServers(config) {
		publicBaseURL := getPublicBaseURL(config)
		if publicBaseURL == "" {
			return fmt.Errorf("proxy.web.publicBaseUrl is required when downstream oauth is enabled")
		}
		if !isValidHTTPURL(publicBaseURL) {
			return fmt.Errorf("proxy.web.publicBaseUrl must be a valid http:// or https:// URL")
		}
	}

	if strict {
		taskVerificationEnabled := config != nil && config.Proxy != nil && config.Proxy.TaskVerificationEnabled()
		if err := validateGateways(config.Gateways, taskVerificationEnabled); err != nil {
			return err
		}
		if err := validateProcessors(config.Processors); err != nil {
			return err
		}
	}
	return nil
}

// getPublicBaseURL returns the public base URL from either the legacy proxy.web
// or project-level web settings.
func getPublicBaseURL(config *GlobalConfig) string {
	// Check legacy proxy.web first.
	if config.Proxy != nil && config.Proxy.Web != nil && config.Proxy.Web.PublicBaseURL != "" {
		return config.Proxy.Web.PublicBaseURL
	}
	return ""
}

// validateProjectSlug validates that a project slug is URL-safe.
func validateProjectSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("project slug cannot be empty")
	}
	if !common.IsURLCompliant(slug) {
		return fmt.Errorf("project '%s': slug must be URL-safe (alphanumeric, dash, underscore only)", slug)
	}
	return nil
}

// validateProjectConfig validates a single project configuration.
func validateProjectConfig(slug string, project *ProjectConfig, strict bool) error {
	normalizeGateways(project.Gateways)

	if err := validateNameConventions(project.Gateways); err != nil {
		return fmt.Errorf("project '%s': %w", slug, err)
	}

	if project.HasOAuthServers() {
		if project.Web == nil || project.Web.PublicBaseURL == "" {
			return fmt.Errorf("project '%s': web.publicBaseUrl is required when downstream oauth is enabled", slug)
		}
		if !isValidHTTPURL(project.Web.PublicBaseURL) {
			return fmt.Errorf("project '%s': web.publicBaseUrl must be a valid http:// or https:// URL", slug)
		}
	}

	if strict {
		if err := validateGateways(project.Gateways, project.TaskVerificationEnabled()); err != nil {
			return fmt.Errorf("project '%s': %w", slug, err)
		}
		if err := validateProcessors(project.Processors); err != nil {
			return fmt.Errorf("project '%s': %w", slug, err)
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

	// Legacy: capabilities and web may still live on ProxySettings for flat configs.
	normalizeCapabilities(proxy.Capabilities)
	if proxy.Web != nil {
		proxy.Web.PublicBaseURL = strings.TrimSpace(proxy.Web.PublicBaseURL)
	}
	return nil
}

// normalizeCapabilities trims whitespace from capability settings.
func normalizeCapabilities(capabilities *CapabilitiesSettings) {
	if capabilities == nil {
		return
	}
	if capabilities.TaskVerification != nil {
		capabilities.TaskVerification.TemplatesPath = strings.TrimSpace(capabilities.TaskVerification.TemplatesPath)
	}
	if capabilities.EventStorage != nil {
		capabilities.EventStorage.Driver = strings.TrimSpace(capabilities.EventStorage.Driver)
		capabilities.EventStorage.Path = strings.TrimSpace(capabilities.EventStorage.Path)
	}
}

func normalizeGateways(gateways map[string]*GatewayConfig) {
	for _, gateway := range gateways {
		if gateway == nil {
			continue
		}
		gateway.VerificationRequirement = gateway.NormalizedVerificationRequirement()
	}
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
func validateGateways(gateways map[string]*GatewayConfig, taskVerificationEnabled bool) error {
	if len(gateways) == 0 {
		return fmt.Errorf("no gateways configured - at least one gateway is required")
	}
	for gatewayName, gatewayConfig := range gateways {
		if gatewayConfig == nil {
			return fmt.Errorf("gateway '%s': config cannot be nil", gatewayName)
		}
		if err := validateGateway(gatewayName, *gatewayConfig, taskVerificationEnabled); err != nil {
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
func validateGateway(name string, config GatewayConfig, taskVerificationEnabled bool) error {
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

	requirement := config.NormalizedVerificationRequirement()
	switch requirement {
	case "", VerificationRequirementOff:
	case VerificationRequirementOptional, VerificationRequirementRequired:
		if !taskVerificationEnabled {
			return fmt.Errorf("gateway '%s': verificationRequirement %q requires project capabilities.taskVerification.enabled=true", name, requirement)
		}
	default:
		return fmt.Errorf("gateway '%s': verificationRequirement %q is unsupported (expected off, optional, or required)", name, requirement)
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
// Checks both legacy flat gateways and project-scoped gateways.
func HasOAuthServers(config *GlobalConfig) bool {
	if config == nil {
		return false
	}
	// Check legacy flat gateways.
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
	// Check project-scoped gateways.
	for _, project := range config.Projects {
		if project != nil && project.HasOAuthServers() {
			return true
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
	case CLIProcessor, WebhookProcessor, BuiltinProcessor:
	default:
		return fmt.Errorf("processor '%s': unsupported type '%s' (supported: 'cli', 'webhook', 'builtin')", processor.Name, processor.Type)
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
			return fmt.Errorf("processor '%s': unsupported part '%s' (allowed: payload, meta, routing, auth, annotations)", processor.Name, part)
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
	case BuiltinProcessor:
		_, err := ParseBuiltinProcessorSettings(processor)
		return err
	}
	return nil
}

// ParseBuiltinProcessorSettings validates and extracts built-in processor settings.
func ParseBuiltinProcessorSettings(processor *ProcessorConfig) (*BuiltinProcessorSettings, error) {
	value, ok := processor.Config["processor"]
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.processor is required for builtin type", processor.Name)
	}
	processorName, ok := value.(string)
	if !ok || strings.TrimSpace(processorName) == "" {
		return nil, fmt.Errorf("processor '%s': config.processor must be a non-empty string", processor.Name)
	}
	settings := &BuiltinProcessorSettings{
		Processor: strings.TrimSpace(processorName),
	}
	if modeValue, exists := processor.Config["mode"]; exists {
		mode, ok := modeValue.(string)
		if !ok {
			return nil, fmt.Errorf("processor '%s': config.mode must be a string", processor.Name)
		}
		settings.Mode = strings.TrimSpace(mode)
	}
	if scopeValue, exists := processor.Config["scope"]; exists {
		scope, ok := scopeValue.(string)
		if !ok {
			return nil, fmt.Errorf("processor '%s': config.scope must be a string", processor.Name)
		}
		settings.Scope = strings.TrimSpace(scope)
	}

	switch settings.Processor {
	case BuiltinPromptInjectionGuard:
		if !processor.Required {
			return nil, fmt.Errorf("processor '%s': prompt_injection_guard must set required=true", processor.Name)
		}
		if err := validateBuiltinRequiredParts(processor, "prompt_injection_guard", []string{"payload", "annotations"}); err != nil {
			return nil, err
		}
	case BuiltinPatternRedactionProcessor:
		if err := validateBuiltinRedactionSettings(processor, settings, BuiltinRedactionScopeBoth, true, true); err != nil {
			return nil, err
		}
	case BuiltinSecretTokenRedactor:
		if err := validateBuiltinRedactionSettings(processor, settings, BuiltinRedactionScopeBoth, false, false); err != nil {
			return nil, err
		}
	case BuiltinPIIRedactor:
		if err := validateBuiltinRedactionSettings(processor, settings, BuiltinRedactionScopeResponse, false, false); err != nil {
			return nil, err
		}
	case BuiltinToolCallGuard:
		if err := validateBuiltinToolGuardSettings(processor, settings); err != nil {
			return nil, err
		}
	}
	return settings, nil
}

func validateBuiltinRequiredParts(processor *ProcessorConfig, processorName string, requiredParts []string) error {
	parts := map[string]bool{}
	for _, part := range processor.GetParts() {
		parts[part] = true
	}
	for _, requiredPart := range requiredParts {
		if !parts[requiredPart] {
			return fmt.Errorf("processor '%s': %s requires part '%s'", processor.Name, processorName, requiredPart)
		}
	}
	return nil
}

func validateBuiltinRedactionSettings(processor *ProcessorConfig, settings *BuiltinProcessorSettings, defaultScope string, requiresRules bool, allowsRules bool) error {
	if settings.Mode == "" {
		settings.Mode = BuiltinRedactionModeRedact
	}
	switch settings.Mode {
	case BuiltinRedactionModeRedact, BuiltinRedactionModeAnnotate:
	default:
		return fmt.Errorf("processor '%s': config.mode must be 'redact' or 'annotate'", processor.Name)
	}

	if settings.Scope == "" {
		settings.Scope = defaultScope
	}
	switch settings.Scope {
	case BuiltinRedactionScopeRequest, BuiltinRedactionScopeResponse, BuiltinRedactionScopeBoth:
	default:
		return fmt.Errorf("processor '%s': config.scope must be 'request', 'response', or 'both'", processor.Name)
	}

	if err := validateBuiltinRequiredParts(processor, settings.Processor, []string{"payload", "annotations"}); err != nil {
		return err
	}

	if _, exists := processor.Config["rules"]; exists && !allowsRules {
		return fmt.Errorf("processor '%s': config.rules is only supported for pattern_redaction_processor", processor.Name)
	}

	rules, err := parseBuiltinRedactionRules(processor)
	if err != nil {
		return err
	}
	if requiresRules && len(rules) == 0 {
		return fmt.Errorf("processor '%s': config.rules must contain at least one rule", processor.Name)
	}
	settings.Rules = rules
	return nil
}

func parseBuiltinRedactionRules(processor *ProcessorConfig) ([]BuiltinRedactionRule, error) {
	rulesValue, exists := processor.Config["rules"]
	if !exists {
		return nil, nil
	}

	ruleMaps, err := processorConfigRuleMaps(rulesValue)
	if err != nil {
		return nil, fmt.Errorf("processor '%s': config.rules %w", processor.Name, err)
	}

	rules := make([]BuiltinRedactionRule, 0, len(ruleMaps))
	for index, ruleMap := range ruleMaps {
		rule, err := parseBuiltinRedactionRule(processor.Name, index, ruleMap)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func processorConfigRuleMaps(value interface{}) ([]map[string]interface{}, error) {
	switch typed := value.(type) {
	case []interface{}:
		rules := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			rule, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("entries must be objects")
			}
			rules = append(rules, rule)
		}
		return rules, nil
	case []map[string]interface{}:
		return typed, nil
	case []BuiltinRedactionRule:
		rules := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			rules = append(rules, map[string]interface{}{
				"name":        item.Name,
				"pattern":     item.Pattern,
				"replacement": item.Replacement,
			})
		}
		return rules, nil
	default:
		return nil, fmt.Errorf("must be an array")
	}
}

func parseBuiltinRedactionRule(processorName string, index int, value map[string]interface{}) (BuiltinRedactionRule, error) {
	name, err := requiredRuleString(processorName, index, value, "name")
	if err != nil {
		return BuiltinRedactionRule{}, err
	}
	pattern, err := requiredRuleString(processorName, index, value, "pattern")
	if err != nil {
		return BuiltinRedactionRule{}, err
	}
	replacement, err := requiredRuleString(processorName, index, value, "replacement")
	if err != nil {
		return BuiltinRedactionRule{}, err
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return BuiltinRedactionRule{}, fmt.Errorf("processor '%s': config.rules[%d].pattern is invalid: %w", processorName, index, err)
	}
	return BuiltinRedactionRule{Name: name, Pattern: pattern, Replacement: replacement}, nil
}

func validateBuiltinToolGuardSettings(processor *ProcessorConfig, settings *BuiltinProcessorSettings) error {
	if settings.Mode == "" {
		settings.Mode = BuiltinToolGuardModeBlock
	}
	switch settings.Mode {
	case BuiltinToolGuardModeBlock, BuiltinToolGuardModeAnnotate:
	default:
		return fmt.Errorf("processor '%s': config.mode must be 'block' or 'annotate'", processor.Name)
	}

	if settings.Scope != "" {
		return fmt.Errorf("processor '%s': config.scope is not supported for tool_call_guard", processor.Name)
	}
	if err := validateBuiltinRequiredParts(processor, settings.Processor, []string{"payload", "routing", "annotations"}); err != nil {
		return err
	}

	presets, err := parseBuiltinToolGuardPresets(processor)
	if err != nil {
		return err
	}
	rules, err := parseBuiltinToolGuardRules(processor)
	if err != nil {
		return err
	}
	pathBoundary, err := parseBuiltinPathBoundarySettings(processor, containsString(presets, BuiltinToolGuardPresetPathBoundary))
	if err != nil {
		return err
	}
	if len(presets) == 0 && len(rules) == 0 {
		return fmt.Errorf("processor '%s': config.presets or config.rules must contain at least one entry", processor.Name)
	}
	settings.Presets = presets
	settings.GuardRules = rules
	settings.PathBoundary = pathBoundary
	return nil
}

func parseBuiltinToolGuardPresets(processor *ProcessorConfig) ([]string, error) {
	raw, exists := processor.Config["presets"]
	if !exists {
		return nil, nil
	}
	presets, err := processorConfigNamedStringSlice(processor.Name, "presets", raw)
	if err != nil {
		return nil, err
	}
	for _, preset := range presets {
		switch preset {
		case BuiltinToolGuardPresetDangerousCommands, BuiltinToolGuardPresetPathBoundary:
		default:
			return nil, fmt.Errorf("processor '%s': config.presets contains unsupported preset %q", processor.Name, preset)
		}
	}
	return presets, nil
}

func parseBuiltinPathBoundarySettings(processor *ProcessorConfig, presetEnabled bool) (*BuiltinPathBoundarySettings, error) {
	raw, exists := processor.Config["path_boundary"]
	if !exists {
		if presetEnabled {
			return &BuiltinPathBoundarySettings{}, nil
		}
		return nil, nil
	}
	value, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.path_boundary must be an object", processor.Name)
	}
	allowedKeys := map[string]bool{
		"allowed_roots":      true,
		"relative_base_root": true,
		"tool_patterns":      true,
		"argument_paths":     true,
		"denied_paths":       true,
	}
	for key := range value {
		if !allowedKeys[key] {
			return nil, fmt.Errorf("processor '%s': config.path_boundary.%s is unsupported", processor.Name, key)
		}
	}

	settings := &BuiltinPathBoundarySettings{}
	var err error
	if rawAllowedRoots, ok := value["allowed_roots"]; ok {
		settings.AllowedRoots, err = parseLexicalRoots(processor.Name, "path_boundary.allowed_roots", rawAllowedRoots)
		if err != nil {
			return nil, err
		}
	}
	if rawRelativeBaseRoot, ok := value["relative_base_root"]; ok {
		relativeBaseRoot, ok := rawRelativeBaseRoot.(string)
		if !ok || strings.TrimSpace(relativeBaseRoot) == "" {
			return nil, fmt.Errorf("processor '%s': config.path_boundary.relative_base_root must be a non-empty string", processor.Name)
		}
		cleanedRoot, err := cleanLexicalRoot(processor.Name, "path_boundary.relative_base_root", relativeBaseRoot)
		if err != nil {
			return nil, err
		}
		settings.RelativeBaseRoot = cleanedRoot
	}
	if settings.RelativeBaseRoot == "" && len(settings.AllowedRoots) > 0 {
		settings.RelativeBaseRoot = settings.AllowedRoots[0]
	}
	if rawToolPatterns, ok := value["tool_patterns"]; ok {
		settings.ToolPatterns, err = processorConfigNamedStringSlice(processor.Name, "path_boundary.tool_patterns", rawToolPatterns)
		if err != nil {
			return nil, err
		}
		if err := validateGlobList(processor.Name, "path_boundary.tool_patterns", settings.ToolPatterns); err != nil {
			return nil, err
		}
	}
	if rawArgumentPaths, ok := value["argument_paths"]; ok {
		settings.ArgumentPaths, err = processorConfigNamedStringSlice(processor.Name, "path_boundary.argument_paths", rawArgumentPaths)
		if err != nil {
			return nil, err
		}
		if err := validateGlobList(processor.Name, "path_boundary.argument_paths", settings.ArgumentPaths); err != nil {
			return nil, err
		}
	}
	if rawDeniedPaths, ok := value["denied_paths"]; ok {
		settings.DeniedPaths, err = processorConfigNamedStringSlice(processor.Name, "path_boundary.denied_paths", rawDeniedPaths)
		if err != nil {
			return nil, err
		}
	}
	return settings, nil
}

func parseLexicalRoots(processorName string, fieldName string, value interface{}) ([]string, error) {
	rawRoots, err := processorConfigNamedStringSlice(processorName, fieldName, value)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(rawRoots))
	for _, root := range rawRoots {
		cleanedRoot, err := cleanLexicalRoot(processorName, fieldName, root)
		if err != nil {
			return nil, err
		}
		roots = append(roots, cleanedRoot)
	}
	return roots, nil
}

func cleanLexicalRoot(processorName string, fieldName string, root string) (string, error) {
	root = strings.TrimSpace(root)
	root = strings.ReplaceAll(root, "\\", "/")
	if root == "" {
		return "", fmt.Errorf("processor '%s': config.%s must contain only non-empty strings", processorName, fieldName)
	}
	if !strings.HasPrefix(root, "/") {
		return "", fmt.Errorf("processor '%s': config.%s must contain absolute lexical roots", processorName, fieldName)
	}
	return pathpkg.Clean(root), nil
}

func validateGlobList(processorName string, fieldName string, values []string) error {
	for _, value := range values {
		if _, err := pathpkg.Match(value, ""); err != nil {
			return fmt.Errorf("processor '%s': config.%s contains invalid glob %q: %w", processorName, fieldName, value, err)
		}
	}
	return nil
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func parseBuiltinToolGuardRules(processor *ProcessorConfig) ([]BuiltinToolGuardRule, error) {
	rulesValue, exists := processor.Config["rules"]
	if !exists {
		return nil, nil
	}
	ruleMaps, err := processorConfigRuleMaps(rulesValue)
	if err != nil {
		return nil, fmt.Errorf("processor '%s': config.rules %w", processor.Name, err)
	}

	rules := make([]BuiltinToolGuardRule, 0, len(ruleMaps))
	for index, ruleMap := range ruleMaps {
		rule, err := parseBuiltinToolGuardRule(processor.Name, index, ruleMap)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseBuiltinToolGuardRule(processorName string, index int, value map[string]interface{}) (BuiltinToolGuardRule, error) {
	name, err := requiredRuleString(processorName, index, value, "name")
	if err != nil {
		return BuiltinToolGuardRule{}, err
	}

	rule := BuiltinToolGuardRule{
		Name:     name,
		Category: "policy",
		Severity: "medium",
	}
	if raw, exists := value["category"]; exists {
		category, ok := raw.(string)
		if !ok || strings.TrimSpace(category) == "" {
			return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].category must be a non-empty string", processorName, index)
		}
		category = strings.TrimSpace(category)
		switch category {
		case "policy", "security", "privacy":
			rule.Category = category
		default:
			return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].category must be 'policy', 'security', or 'privacy'", processorName, index)
		}
	}
	if raw, exists := value["severity"]; exists {
		severity, ok := raw.(string)
		if !ok || strings.TrimSpace(severity) == "" {
			return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].severity must be a non-empty string", processorName, index)
		}
		severity = strings.TrimSpace(severity)
		switch severity {
		case "low", "medium", "high", "critical":
			rule.Severity = severity
		default:
			return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].severity must be 'low', 'medium', 'high', or 'critical'", processorName, index)
		}
	}
	if raw, exists := value["message"]; exists {
		message, ok := raw.(string)
		if !ok || strings.TrimSpace(message) == "" {
			return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].message must be a non-empty string", processorName, index)
		}
		rule.Message = strings.TrimSpace(message)
	}
	if raw, exists := value["tool_patterns"]; exists {
		toolPatterns, err := processorConfigNamedStringSlice(processorName, fmt.Sprintf("rules[%d].tool_patterns", index), raw)
		if err != nil {
			return BuiltinToolGuardRule{}, err
		}
		for _, pattern := range toolPatterns {
			if _, err := pathpkg.Match(pattern, ""); err != nil {
				return BuiltinToolGuardRule{}, fmt.Errorf("processor '%s': config.rules[%d].tool_patterns contains invalid glob %q: %w", processorName, index, pattern, err)
			}
		}
		rule.ToolPatterns = toolPatterns
	}
	if raw, exists := value["argument_rules"]; exists {
		argumentRules, err := parseBuiltinToolGuardArgumentRules(processorName, index, raw)
		if err != nil {
			return BuiltinToolGuardRule{}, err
		}
		rule.ArgumentRules = argumentRules
	}
	return rule, nil
}

func parseBuiltinToolGuardArgumentRules(processorName string, ruleIndex int, raw interface{}) ([]BuiltinToolGuardArgumentRule, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("processor '%s': config.rules[%d].argument_rules must be an array", processorName, ruleIndex)
	}
	rules := make([]BuiltinToolGuardArgumentRule, 0, len(items))
	for index, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d] must be an object", processorName, ruleIndex, index)
		}
		rule, err := parseBuiltinToolGuardArgumentRule(processorName, ruleIndex, index, itemMap)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseBuiltinToolGuardArgumentRule(processorName string, ruleIndex int, index int, value map[string]interface{}) (BuiltinToolGuardArgumentRule, error) {
	rule := BuiltinToolGuardArgumentRule{}
	if raw, exists := value["path"]; exists {
		pathValue, ok := raw.(string)
		if !ok || strings.TrimSpace(pathValue) == "" {
			return BuiltinToolGuardArgumentRule{}, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d].path must be a non-empty string", processorName, ruleIndex, index)
		}
		rule.Path = strings.TrimSpace(pathValue)
		if _, err := pathpkg.Match(rule.Path, ""); err != nil {
			return BuiltinToolGuardArgumentRule{}, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d].path contains invalid glob: %w", processorName, ruleIndex, index, err)
		}
	}
	if raw, exists := value["pattern"]; exists {
		pattern, ok := raw.(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			return BuiltinToolGuardArgumentRule{}, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d].pattern must be a non-empty string", processorName, ruleIndex, index)
		}
		rule.Pattern = strings.TrimSpace(pattern)
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return BuiltinToolGuardArgumentRule{}, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d].pattern is invalid: %w", processorName, ruleIndex, index, err)
		}
	}
	if rule.Path == "" && rule.Pattern == "" {
		return BuiltinToolGuardArgumentRule{}, fmt.Errorf("processor '%s': config.rules[%d].argument_rules[%d] requires path or pattern", processorName, ruleIndex, index)
	}
	return rule, nil
}

func requiredRuleString(processorName string, index int, value map[string]interface{}, key string) (string, error) {
	raw, exists := value[key]
	if !exists {
		return "", fmt.Errorf("processor '%s': config.rules[%d].%s is required", processorName, index, key)
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("processor '%s': config.rules[%d].%s must be a non-empty string", processorName, index, key)
	}
	return strings.TrimSpace(text), nil
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

func processorConfigNamedStringSlice(processorName string, fieldName string, value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}

	var rawValues []interface{}
	switch typed := value.(type) {
	case []interface{}:
		rawValues = typed
	case []string:
		rawValues = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			rawValues = append(rawValues, item)
		}
	default:
		return nil, fmt.Errorf("processor '%s': config.%s must be an array", processorName, fieldName)
	}

	values := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		text, ok := raw.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("processor '%s': config.%s must contain only non-empty strings", processorName, fieldName)
		}
		values = append(values, strings.TrimSpace(text))
	}
	return values, nil
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
