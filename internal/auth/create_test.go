package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAPIKey_SQLite_RoundTrips(t *testing.T) {
	// Given: a sqlite backend store path
	store := filepath.Join(t.TempDir(), "principals.sqlite")

	// When: creating a key through the backend-agnostic entry point
	created, err := CreateAPIKey(context.Background(), string(BackendSQLite), store, CreateAPIKeyParams{
		Name:     "ci bot",
		Projects: []string{"research"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	// Then: the token resolves through a fresh provider over the same store
	if created.BackendType != BackendSQLite {
		t.Errorf("BackendType = %q, want sqlite", created.BackendType)
	}
	provider := NewSQLPrincipalProvider(store)
	if err := provider.Setup(context.Background()); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	principal, err := provider.GetPrincipal(context.Background(), created.Token)
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}
	if principal.DisplayName != "ci bot" {
		t.Errorf("DisplayName = %q, want %q", principal.DisplayName, "ci bot")
	}
	if principal.ID != created.PrincipalID {
		t.Errorf("principal ID = %q, want %q", principal.ID, created.PrincipalID)
	}
	if len(principal.Projects) != 1 || principal.Projects[0] != "research" {
		t.Errorf("Projects = %v, want [research]", principal.Projects)
	}
}

func TestCreateAPIKey_SQLite_ExpandsHomeStore(t *testing.T) {
	// Given: a configured sqlite store using a shell-style home path
	homeDir := filepath.Join(t.TempDir(), "home")
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Chdir(workDir)
	store := "~/.centian/custom-principals.sqlite"
	expectedStore := filepath.Join(homeDir, ".centian", "custom-principals.sqlite")

	// When: creating a key through the configured backend
	created, err := CreateAPIKey(context.Background(), string(BackendSQLite), store, CreateAPIKeyParams{Name: "bot"})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	// Then: the sqlite file is created under HOME, not under the current directory
	if created.Store != expectedStore {
		t.Errorf("Store = %q, want %q", created.Store, expectedStore)
	}
	if _, statErr := os.Stat(expectedStore); statErr != nil {
		t.Fatalf("expected sqlite store at %s: %v", expectedStore, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "~")); !os.IsNotExist(statErr) {
		t.Fatalf("literal tilde path was created under cwd: %v", statErr)
	}
}

func TestCreateAPIKey_File_RoundTrips(t *testing.T) {
	// Given: a file backend store path
	store := filepath.Join(t.TempDir(), "api_keys.json")

	// When: creating a key with the file backend
	created, err := CreateAPIKey(context.Background(), string(BackendFile), store, CreateAPIKeyParams{Name: "filed"})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	// Then: a file provider over the same path resolves the token
	if created.BackendType != BackendFile {
		t.Errorf("BackendType = %q, want file", created.BackendType)
	}
	provider := NewFilePrincipalProvider(store)
	if err := provider.Setup(context.Background()); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	principal, err := provider.GetPrincipal(context.Background(), created.Token)
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}
	if principal.DisplayName != "filed" {
		t.Errorf("DisplayName = %q, want %q", principal.DisplayName, "filed")
	}
}

func TestNewPrincipalProvider_DefaultsToSQLite(t *testing.T) {
	// Given: an empty backend type and an explicit sqlite store
	store := filepath.Join(t.TempDir(), "principals.sqlite")

	// When: building a provider with empty type
	provider, err := NewPrincipalProvider("", store)
	if err != nil {
		t.Fatalf("NewPrincipalProvider() error = %v", err)
	}

	// Then: it is the sqlite provider
	if _, ok := provider.(*SQLPrincipalProvider); !ok {
		t.Errorf("provider type = %T, want *SQLPrincipalProvider", provider)
	}
}

func TestNewPrincipalProvider_UnsupportedType(t *testing.T) {
	// Given/When: an unknown backend type
	_, err := NewPrincipalProvider("postgres", "dsn")

	// Then: it is rejected
	if err == nil {
		t.Errorf("expected error for unsupported backend type")
	}
}
