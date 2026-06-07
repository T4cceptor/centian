package auth

import (
	"context"
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
	_, err := NewPrincipalProvider("mysql", "dsn")

	// Then: it is rejected
	if err == nil {
		t.Errorf("expected error for unsupported backend type")
	}
}

func TestNewPrincipalProvider_Postgres(t *testing.T) {
	// Given/When: the postgres backend type with a DSN
	provider, err := NewPrincipalProvider("postgres", "postgres://localhost/centian")
	if err != nil {
		t.Fatalf("NewPrincipalProvider() error = %v", err)
	}

	// Then: it is the SQL provider (Setup would connect; not exercised here)
	if _, ok := provider.(*SQLPrincipalProvider); !ok {
		t.Errorf("provider type = %T, want *SQLPrincipalProvider", provider)
	}
}

func TestNewPrincipalProvider_PostgresRequiresDSN(t *testing.T) {
	// Given/When: the postgres backend type without a store/DSN
	_, err := NewPrincipalProvider("postgres", "")

	// Then: it is rejected (postgres has no default location)
	if err == nil {
		t.Errorf("expected error for postgres backend without a dsn")
	}
}
