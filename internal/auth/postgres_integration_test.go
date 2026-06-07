package auth

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// principalTables lists the auth tables, child-first so drops respect any FK.
var principalTables = []string{
	"principal_projects",
	"principal_gateways",
	"principal_credentials",
	"principals",
	"principal_store_schema",
}

// postgresAuthDSN returns the configured Postgres DSN, skipping when unset, and
// drops the principal tables so each run bootstraps a clean schema (also proving
// the principal DDL is valid Postgres).
func postgresAuthDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CENTIAN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CENTIAN_TEST_POSTGRES_DSN to run Postgres integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, table := range principalTables {
		if _, err := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table+" CASCADE"); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
	return dsn
}

func TestPostgres_SQLPrincipalProvider_CreateAndResolve(t *testing.T) {
	// Given: a clean Postgres principal store and a freshly minted api key
	dsn := postgresAuthDSN(t)
	ctx := context.Background()

	created, err := CreateAPIKey(ctx, string(BackendPostgres), dsn, CreateAPIKeyParams{
		Name:     "pg ci bot",
		Gateways: []string{"alpha"},
		Projects: []string{"research"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if created.BackendType != BackendPostgres {
		t.Errorf("BackendType = %q, want %q", created.BackendType, BackendPostgres)
	}

	// When: a postgres-backed provider resolves the minted token
	provider, err := NewPrincipalProvider(string(BackendPostgres), dsn)
	if err != nil {
		t.Fatalf("NewPrincipalProvider() error = %v", err)
	}
	if _, ok := provider.(*SQLPrincipalProvider); !ok {
		t.Fatalf("provider type = %T, want *SQLPrincipalProvider", provider)
	}
	if err := provider.Setup(ctx); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	principal, err := provider.GetPrincipal(ctx, created.Token)
	if err != nil {
		t.Fatalf("GetPrincipal() error = %v", err)
	}

	// Then: the principal resolves with its id, name, and grants intact
	if principal.ID != created.PrincipalID {
		t.Errorf("principal ID = %q, want %q", principal.ID, created.PrincipalID)
	}
	if principal.CredentialID != created.CredentialID {
		t.Errorf("CredentialID = %q, want %q", principal.CredentialID, created.CredentialID)
	}
	if principal.DisplayName != "pg ci bot" {
		t.Errorf("DisplayName = %q, want %q", principal.DisplayName, "pg ci bot")
	}
	if len(principal.Gateways) != 1 || principal.Gateways[0] != "alpha" {
		t.Errorf("Gateways = %v, want [alpha]", principal.Gateways)
	}
	if len(principal.Projects) != 1 || principal.Projects[0] != "research" {
		t.Errorf("Projects = %v, want [research]", principal.Projects)
	}
}
