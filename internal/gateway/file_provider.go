package gateway

import (
	"github.com/T4cceptor/centian/internal/config"
)

// FileGatewayProvider reads and writes gateway configuration from a JSON file on disk.
type FileGatewayProvider struct {
	path string // absolute path to gateways.json
	cfg  *config.ServerConfig
}

// NewFileGatewayProvider creates a FileGatewayProvider.
// The path is derived from cfg.GatewayProvider; if empty, defaults to ~/.centian/gateways.json.
func NewFileGatewayProvider(cfg *config.ServerConfig) (*FileGatewayProvider, error) {
	path, err := config.GetGatewayFilePath(cfg)
	if err != nil {
		return nil, err
	}
	return &FileGatewayProvider{path: path, cfg: cfg}, nil
}

// LoadGatewayFile reads and returns the gateway file from disk.
func (p *FileGatewayProvider) LoadGatewayFile() (*config.GatewayFile, error) {
	return config.LoadGatewayFileFromPath(p.path)
}

// SaveGatewayFile writes the gateway file to disk.
func (p *FileGatewayProvider) SaveGatewayFile(file *config.GatewayFile) error {
	return config.SaveGatewayFileToPath(p.path, file)
}
