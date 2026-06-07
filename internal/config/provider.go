package config

import (
	"context"
	"fmt"
)

// ConfigProvider abstracts where and when configuration is loaded. The proxy
// consumes config through this seam instead of taking a monolithic GlobalConfig,
// so a project's ProjectConfig can be fetched on demand and an implementation can
// source config from somewhere other than a local file (e.g. a database or an HTTP
// API) without changing the server.
//
// The methods take a context so implementations backed by I/O can honor
// cancellation and deadlines; in-memory implementations ignore it.
//
//nolint:revive // "ConfigProvider" is the established name for this seam (see T2); config.Provider would be less descriptive at call sites.
type ConfigProvider interface {
	// Global returns the truly-global settings (proxy bind/port/timeout, auth
	// backend, version/name). It is small and needed before any project is served,
	// so implementations may load it eagerly.
	Global(ctx context.Context) (*GlobalConfig, error)

	// ListProjects returns the slugs of all known projects.
	ListProjects(ctx context.Context) ([]string, error)

	// GetProject fetches a single project's ProjectConfig on demand. File-backed
	// implementations serve this from a preloaded in-memory map; an I/O-backed
	// implementation may load it here, which can add latency to the MCP initialize
	// request.
	GetProject(ctx context.Context, slug string) (*ProjectConfig, error)
}

// FileConfigProvider is the file-backed ConfigProvider. It preloads the whole
// config once and serves each request from an in-memory map, so GetProject never
// performs I/O. This keeps the on-demand seam in place while preserving the
// "pre-load the file" behavior.
type FileConfigProvider struct {
	cfg *GlobalConfig
}

// compile-time assertion that FileConfigProvider satisfies ConfigProvider.
var _ ConfigProvider = (*FileConfigProvider)(nil)

// NewFileConfigProvider loads and validates a config from path, normalizes it to
// the project-based layout, and returns a provider that serves it from memory.
func NewFileConfigProvider(path string) (*FileConfigProvider, error) {
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	return NewConfigProviderFromConfig(cfg), nil
}

// NewConfigProviderFromConfig wraps an already-loaded GlobalConfig in a file
// provider. It normalizes the config to the project-based layout (idempotent) so
// callers can always reach projects via ListProjects/GetProject. Primarily used by
// the back-compat NewCentianServer constructor and tests.
func NewConfigProviderFromConfig(cfg *GlobalConfig) *FileConfigProvider {
	if cfg != nil {
		cfg.ResolveProjects()
	}
	return &FileConfigProvider{cfg: cfg}
}

// Global returns the preloaded global config.
func (p *FileConfigProvider) Global(_ context.Context) (*GlobalConfig, error) {
	if p == nil || p.cfg == nil {
		return nil, fmt.Errorf("config provider has no config loaded")
	}
	return p.cfg, nil
}

// ListProjects returns the slugs of all preloaded projects.
func (p *FileConfigProvider) ListProjects(_ context.Context) ([]string, error) {
	if p == nil || p.cfg == nil {
		return nil, fmt.Errorf("config provider has no config loaded")
	}
	slugs := make([]string, 0, len(p.cfg.Projects))
	for slug := range p.cfg.Projects {
		slugs = append(slugs, slug)
	}
	return slugs, nil
}

// GetProject returns the ProjectConfig for slug from the preloaded map.
func (p *FileConfigProvider) GetProject(_ context.Context, slug string) (*ProjectConfig, error) {
	if p == nil || p.cfg == nil {
		return nil, fmt.Errorf("config provider has no config loaded")
	}
	project, ok := p.cfg.Projects[slug]
	if !ok {
		return nil, fmt.Errorf("project %q not found", slug)
	}
	return project, nil
}
