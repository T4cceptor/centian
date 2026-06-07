package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestFileConfigProviderServesPreloadedProjects verifies that a provider built
// from a file exposes the resolved projects via Global/ListProjects/GetProject.
func TestFileConfigProviderServesPreloadedProjects(t *testing.T) {
	// Given: a legacy-flat config file with a single gateway on disk.
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cfg := DefaultConfig()
	cfg.Gateways = map[string]*GatewayConfig{
		"gateway1": {
			MCPServers: map[string]*MCPServerConfig{
				"server1": {Name: "server1", Command: "node"},
			},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// When: constructing a file-backed provider.
	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider failed: %v", err)
	}

	ctx := context.Background()

	// Then: Global returns the loaded config with proxy settings populated.
	global, err := provider.Global(ctx)
	if err != nil {
		t.Fatalf("Global failed: %v", err)
	}
	if global == nil || global.Proxy == nil {
		t.Fatalf("expected global config with proxy settings, got %+v", global)
	}

	// Then: the legacy-flat layout was auto-migrated to a single "default" project.
	slugs, err := provider.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(slugs) != 1 || slugs[0] != DefaultProjectSlug {
		t.Fatalf("expected [%q], got %v", DefaultProjectSlug, slugs)
	}

	// Then: GetProject returns the default project carrying the gateway.
	project, err := provider.GetProject(ctx, DefaultProjectSlug)
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if _, ok := project.Gateways["gateway1"]; !ok {
		t.Fatalf("expected gateway1 in default project, got %v", project.Gateways)
	}
}

// TestFileConfigProviderGetProjectUnknownSlug verifies an error on a missing slug.
func TestFileConfigProviderGetProjectUnknownSlug(t *testing.T) {
	// Given: a provider built from an in-memory default config.
	provider := NewConfigProviderFromConfig(DefaultConfig())

	// When: fetching a project that does not exist.
	_, err := provider.GetProject(context.Background(), "does-not-exist")

	// Then: an error is returned.
	if err == nil {
		t.Fatalf("expected error for unknown project slug, got nil")
	}
}

// TestNewConfigProviderFromConfigResolvesLegacyLayout verifies that wrapping a
// legacy-flat in-memory config normalizes it to the project-based layout.
func TestNewConfigProviderFromConfigResolvesLegacyLayout(t *testing.T) {
	// Given: a legacy-flat config with no Projects map.
	cfg := DefaultConfig()
	if len(cfg.Projects) != 0 {
		t.Fatalf("expected legacy layout with no projects, got %v", cfg.Projects)
	}

	// When: wrapping it in a file provider.
	provider := NewConfigProviderFromConfig(cfg)

	// Then: a synthesized "default" project is reachable via GetProject.
	project, err := provider.GetProject(context.Background(), DefaultProjectSlug)
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if project == nil || project.Slug != DefaultProjectSlug {
		t.Fatalf("expected default project, got %+v", project)
	}
}
