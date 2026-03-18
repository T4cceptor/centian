// Package gateway provides the GatewayProvider abstraction for loading and saving
// gateway configurations from different backends.
package gateway

import "github.com/T4cceptor/centian/internal/config"

// GatewayProvider is the source of truth for gateway and MCP server configurations.
// Implementations can read from different backends (file, SQL, remote API).
type GatewayProvider interface {
	// LoadGatewayFile fetches the current gateway configuration.
	// Called at startup and on explicit reload. Safe to call concurrently.
	LoadGatewayFile() (*config.GatewayFile, error)

	// SaveGatewayFile persists the given gateway file to the backend.
	// Used by offline CLI commands (server add/remove/enable/disable).
	SaveGatewayFile(file *config.GatewayFile) error
}
