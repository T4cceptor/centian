package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/T4cceptor/centian/internal/auth"
	"github.com/T4cceptor/centian/internal/config"
	urfavecli "github.com/urfave/cli/v3"
)

func TestAuthCommandStructure(t *testing.T) {
	if AuthCommand == nil {
		t.Fatal("AuthCommand is nil")
	}

	if AuthCommand.Name != "auth" {
		t.Errorf("Expected command name 'auth', got '%s'", AuthCommand.Name)
	}

	if AuthCommand.Usage == "" {
		t.Error("AuthCommand should have usage text")
	}

	if len(AuthCommand.Commands) == 0 {
		t.Fatal("AuthCommand should have subcommands")
	}

	var hasNewKey bool
	for _, subcmd := range AuthCommand.Commands {
		if subcmd.Name != "new-key" {
			continue
		}
		hasNewKey = true
		if subcmd.Usage == "" {
			t.Error("AuthNewKeyCommand should have usage text")
		}
		if subcmd.Description == "" {
			t.Error("AuthNewKeyCommand should have description")
		}
		if subcmd.Action == nil {
			t.Error("AuthNewKeyCommand should have action function")
		}
		break
	}

	if !hasNewKey {
		t.Error("AuthCommand should have 'new-key' subcommand")
	}
}

// writeConfigWithBackend writes a valid config carrying the given auth backend to
// a temp file and returns its path.
func writeConfigWithBackend(t *testing.T, backend *config.AuthBackendSettings) string {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.AuthBackend = backend
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func runNewKey(t *testing.T, args ...string) error {
	t.Helper()
	app := &urfavecli.Command{
		Name:     "centian",
		Commands: []*urfavecli.Command{AuthCommand},
	}
	return app.Run(context.Background(), append([]string{"centian", "auth", "new-key"}, args...))
}

func TestAuthNewKey_ConfigSelectsFileBackend(t *testing.T) {
	// Given: a config whose authBackend selects the file backend
	cleanup := setupTestHome(t)
	defer cleanup()
	configPath := writeConfigWithBackend(t, &config.AuthBackendSettings{Type: "file"})

	// When: minting a key with --config pointing at it
	err := runNewKey(t, "--name", "filed", "--config", configPath)

	// Then: the api key file is created and resolves the principal
	if err != nil {
		t.Fatalf("new-key error = %v", err)
	}
	keysPath, err := auth.DefaultAPIKeysPath()
	if err != nil {
		t.Fatalf("DefaultAPIKeysPath error = %v", err)
	}
	if _, statErr := os.Stat(keysPath); statErr != nil {
		t.Fatalf("expected api_keys.json at %s: %v", keysPath, statErr)
	}
}

func TestAuthNewKey_ConfigSelectsSQLiteStore(t *testing.T) {
	// Given: a config whose authBackend selects a sqlite store at a custom path
	cleanup := setupTestHome(t)
	defer cleanup()
	storePath := filepath.Join(t.TempDir(), "custom-principals.sqlite")
	configPath := writeConfigWithBackend(t, &config.AuthBackendSettings{Type: "sqlite", Store: storePath})

	// When: minting a key with --config pointing at it
	err := runNewKey(t, "--name", "bot", "--config", configPath)

	// Then: the configured sqlite store is created (not the default path)
	if err != nil {
		t.Fatalf("new-key error = %v", err)
	}
	if _, statErr := os.Stat(storePath); statErr != nil {
		t.Fatalf("expected sqlite store at %s: %v", storePath, statErr)
	}
}

func TestAuthNewKey_ConfigExpandsSQLiteStoreHomePath(t *testing.T) {
	// Given: a config whose authBackend selects a sqlite store under HOME
	cleanup := setupTestHome(t)
	defer cleanup()
	workDir := t.TempDir()
	t.Chdir(workDir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error = %v", err)
	}
	storePath := "~/.centian/custom-principals.sqlite"
	expectedStorePath := filepath.Join(homeDir, ".centian", "custom-principals.sqlite")
	configPath := writeConfigWithBackend(t, &config.AuthBackendSettings{Type: "sqlite", Store: storePath})

	// When: minting a key with --config pointing at it
	err = runNewKey(t, "--name", "bot", "--config", configPath)

	// Then: the configured sqlite store is created under HOME, not cwd
	if err != nil {
		t.Fatalf("new-key error = %v", err)
	}
	if _, statErr := os.Stat(expectedStorePath); statErr != nil {
		t.Fatalf("expected sqlite store at %s: %v", expectedStorePath, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "~")); !os.IsNotExist(statErr) {
		t.Fatalf("literal tilde path was created under cwd: %v", statErr)
	}
}

func TestAuthNewKey_MissingConfigIsFatal(t *testing.T) {
	// Given: a home with no relevant state
	cleanup := setupTestHome(t)
	defer cleanup()

	// When: pointing --config at a non-existent file
	err := runNewKey(t, "--name", "bot", "--config", filepath.Join(t.TempDir(), "missing.json"))

	// Then: the command fails rather than silently falling back to defaults
	if err == nil {
		t.Fatal("expected error for missing --config file")
	}
}

func TestAuthNewKey_DefaultConfigBackend(t *testing.T) {
	// Given: a default config (~/.centian/config.json) with the file backend
	cleanup := setupTestHome(t)
	defer cleanup()
	cfg := config.DefaultConfig()
	cfg.AuthBackend = &config.AuthBackendSettings{Type: "file"}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig error = %v", err)
	}

	// When: minting a key without --config
	err := runNewKey(t, "--name", "default-backed")

	// Then: the default config's backend (file) is used
	if err != nil {
		t.Fatalf("new-key error = %v", err)
	}
	keysPath, err := auth.DefaultAPIKeysPath()
	if err != nil {
		t.Fatalf("DefaultAPIKeysPath error = %v", err)
	}
	if _, statErr := os.Stat(keysPath); statErr != nil {
		t.Fatalf("expected api_keys.json at %s: %v", keysPath, statErr)
	}
}
