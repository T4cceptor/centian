package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// newTestSQLProvider opens a SQL principal provider over a fresh temp-file db.
func newTestSQLProvider(t *testing.T) *SQLPrincipalProvider {
	t.Helper()
	path := filepath.Join(t.TempDir(), "principals.sqlite")
	provider := NewSQLPrincipalProvider(path)
	if err := provider.Setup(context.Background()); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	return provider
}

// createKey mints an api key and persists it as a principal in the provider's store.
func createKey(t *testing.T, provider *SQLPrincipalProvider, name string, gateways, projects []string) GeneratedKey {
	t.Helper()
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	entry, err := NewAPIKeyEntry(gen)
	if err != nil {
		t.Fatalf("NewAPIKeyEntry() error = %v", err)
	}
	if err := provider.store.createAPIKeyPrincipal(context.Background(), &entry, name, gateways, projects); err != nil {
		t.Fatalf("createAPIKeyPrincipal() error = %v", err)
	}
	return gen
}

func TestSQLPrincipalProvider_GetPrincipal_ResolvesCreatedKey(t *testing.T) {
	// Given: a SQL provider with a created api key carrying grants
	provider := newTestSQLProvider(t)
	gen := createKey(t, provider, "ci bot", []string{"alpha"}, []string{"research"})

	// When: resolving the minted token
	principal, err := provider.GetPrincipal(context.Background(), gen.Token)

	// Then: the principal resolves with stable id, name, and grants
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}
	if principal.CredentialID != gen.CredID {
		t.Errorf("CredentialID = %q, want %q", principal.CredentialID, gen.CredID)
	}
	if principal.DisplayName != "ci bot" {
		t.Errorf("DisplayName = %q, want %q", principal.DisplayName, "ci bot")
	}
	if principal.ID == "" {
		t.Errorf("principal ID should be non-empty")
	}
	if len(principal.Gateways) != 1 || principal.Gateways[0] != "alpha" {
		t.Errorf("Gateways = %v, want [alpha]", principal.Gateways)
	}
	if len(principal.Projects) != 1 || principal.Projects[0] != "research" {
		t.Errorf("Projects = %v, want [research]", principal.Projects)
	}
}

func TestSQLPrincipalProvider_GetPrincipal_StableIDAcrossReopen(t *testing.T) {
	// Given: a created key in a sqlite file
	path := filepath.Join(t.TempDir(), "principals.sqlite")
	first := NewSQLPrincipalProvider(path)
	if err := first.Setup(context.Background()); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	gen := createKey(t, first, "bot", nil, nil)
	wantPrincipal, err := first.GetPrincipal(context.Background(), gen.Token)
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}
	_ = first.Close()

	// When: reopening the same database and resolving the same token
	second := NewSQLPrincipalProvider(path)
	if err := second.Setup(context.Background()); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	got, err := second.GetPrincipal(context.Background(), gen.Token)

	// Then: the principal id is unchanged
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}
	if got.ID != wantPrincipal.ID {
		t.Errorf("principal id changed across reopen: %q -> %q", wantPrincipal.ID, got.ID)
	}
}

func TestSQLPrincipalProvider_GetPrincipal_WrongSecret(t *testing.T) {
	// Given: a created key
	provider := newTestSQLProvider(t)
	gen := createKey(t, provider, "bot", nil, nil)

	// When: resolving a token with the right credential id but a tampered secret
	tampered := gen.Token + "x"
	_, err := provider.GetPrincipal(context.Background(), tampered)

	// Then: it is rejected as not found
	if !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("err = %v, want ErrPrincipalNotFound", err)
	}
}

func TestSQLPrincipalProvider_GetPrincipal_UnknownCredential(t *testing.T) {
	// Given: an empty provider
	provider := newTestSQLProvider(t)

	// When: resolving a well-formed token whose credential is not stored
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	_, err = provider.GetPrincipal(context.Background(), gen.Token)

	// Then: it is rejected as not found
	if !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("err = %v, want ErrPrincipalNotFound", err)
	}
}

func TestSQLPrincipalProvider_GetPrincipal_LegacyToken(t *testing.T) {
	// Given: a provider and a pre-principal (no embedded credential id) token
	provider := newTestSQLProvider(t)

	// When: resolving the legacy token
	_, err := provider.GetPrincipal(context.Background(), "sk-onlysecretnoseparator")

	// Then: the legacy-format sentinel is returned
	if !errors.Is(err, ErrLegacyTokenFormat) {
		t.Errorf("err = %v, want ErrLegacyTokenFormat", err)
	}
}

func TestSQLPrincipalProvider_Setup_EmptyStoreIsValid(t *testing.T) {
	// Given/When: a fresh provider over a non-existent db file
	provider := newTestSQLProvider(t)

	// Then: setup succeeds with zero principals (no error on empty)
	if got := provider.Count(); got != 0 {
		t.Errorf("Count() = %d, want 0", got)
	}
}

func TestSQLPrincipalProvider_MigrationGuard(t *testing.T) {
	// Given: a store whose recorded schema version is newer than supported
	path := filepath.Join(t.TempDir(), "principals.sqlite")
	store, err := openSQLPrincipalStore(path)
	if err != nil {
		t.Fatalf("openSQLPrincipalStore() error = %v", err)
	}
	if _, err := store.db.NewUpdate().
		Model(&principalSchemaVersionRow{}).
		Set("version = ?", principalSchemaVersion+1).
		Where("name = ?", principalSchemaName).
		Exec(context.Background()); err != nil {
		t.Fatalf("bump version error = %v", err)
	}
	_ = store.Close()

	// When: reopening the store
	_, err = openSQLPrincipalStore(path)

	// Then: a migration-required error is returned
	var migErr *PrincipalSchemaMigrationRequiredError
	if !errors.As(err, &migErr) {
		t.Fatalf("err = %v, want PrincipalSchemaMigrationRequiredError", err)
	}
}
