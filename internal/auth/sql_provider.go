package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// This file defines the SQL-backed PrincipalProvider and its underlying store.
//
// Principals are global (resolved at the HTTP layer before a project is chosen),
// so they live in their own SQLite database (~/.centian/principals.sqlite) with a
// dedicated schema and version table — independent of the per-project event store.
//
// Credentials are modelled generically: a principal_credentials row carries a
// `type` discriminator and an opaque `data` JSON blob, so future credential types
// (JWT, OAuth) are additive without schema churn. Authorization grants (gateways,
// projects) are principal-level and live in their own symmetric tables, keeping
// credentials free of authorization concerns.

const (
	// principalSchemaVersion is the current principals schema version. This is a
	// brand-new, dedicated database, so v1 simply creates all tables. Future
	// changes bump this and add migrations.
	principalSchemaVersion = 1
	// principalSchemaName names the row in principal_store_schema tracking the
	// installed schema version.
	principalSchemaName = "principal_storage"

	// credentialTypeAPIKey is the credential type discriminator for API keys.
	credentialTypeAPIKey = "api_key"
)

// PrincipalSchemaMigrationRequiredError reports that an existing principals store
// was written by a newer schema than this binary understands.
type PrincipalSchemaMigrationRequiredError struct {
	StoredVersion   int
	ExpectedVersion int
}

func (e *PrincipalSchemaMigrationRequiredError) Error() string {
	if e == nil {
		return "principal store schema migration required"
	}
	return fmt.Sprintf(
		"principal store schema version %d is newer than supported version %d; upgrade centian",
		e.StoredVersion,
		e.ExpectedVersion,
	)
}

// apiKeyCredentialData is the JSON shape stored in principal_credentials.data for
// api_key credentials. Only auth material lives here; grants are stored separately.
type apiKeyCredentialData struct {
	HashedKey string `json:"hashed_key"`
}

type principalRow struct {
	bun.BaseModel `bun:"table:principals"`

	PrincipalID        string `bun:"principal_id,pk"`
	DisplayName        string `bun:"display_name"`
	CreatedAtUnixMilli int64  `bun:"created_at_unix_milli"`
}

type principalCredentialRow struct {
	bun.BaseModel `bun:"table:principal_credentials"`

	CredentialID       string          `bun:"credential_id,pk"`
	PrincipalID        string          `bun:"principal_id"`
	Type               string          `bun:"type"`
	Data               json.RawMessage `bun:"data"`
	CreatedAtUnixMilli int64           `bun:"created_at_unix_milli"`
}

type principalGatewayRow struct {
	bun.BaseModel `bun:"table:principal_gateways"`

	PrincipalID string `bun:"principal_id,pk"`
	Gateway     string `bun:"gateway,pk"`
}

type principalProjectRow struct {
	bun.BaseModel `bun:"table:principal_projects"`

	PrincipalID string `bun:"principal_id,pk"`
	ProjectSlug string `bun:"project_slug,pk"`
}

type principalSchemaVersionRow struct {
	bun.BaseModel `bun:"table:principal_store_schema"`

	Name    string `bun:",pk"`
	Version int
}

// sqlPrincipalStore wraps the bun-backed principals database. It owns schema
// bootstrap/migration and the read/write queries used by the provider and CLI.
type sqlPrincipalStore struct {
	db *bun.DB
}

// openSQLPrincipalStore opens (creating if needed) the principals database at path
// and bootstraps its schema. path may be a file path or ":memory:" for tests.
func openSQLPrincipalStore(path string) (*sqlPrincipalStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite principal store path is required")
	}
	if !isSQLiteMemoryPath(path) {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("failed to create principal store directory: %w", err)
		}
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite principal store: %w", err)
	}
	// Keep the principal store on one SQLite connection so bootstrap/migration and
	// concurrent readers all observe one consistent sqliteshim-backed handle.
	sqldb.SetMaxOpenConns(1)

	store := &sqlPrincipalStore{db: bun.NewDB(sqldb, sqlitedialect.New())}
	if err := store.bootstrap(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return store, nil
}

func isSQLiteMemoryPath(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file::memory:")
}

// Close releases the underlying database handle.
func (s *sqlPrincipalStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlPrincipalStore) bootstrap(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS principal_store_schema (
		name TEXT PRIMARY KEY,
		version INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to bootstrap principal store schema: %w", err)
	}

	versionRow := &principalSchemaVersionRow{}
	err := s.db.NewSelect().Model(versionRow).Where("name = ?", principalSchemaName).Scan(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := s.createTables(ctx); err != nil {
			return err
		}
		if _, err := s.db.NewInsert().Model(&principalSchemaVersionRow{
			Name:    principalSchemaName,
			Version: principalSchemaVersion,
		}).Exec(ctx); err != nil {
			return fmt.Errorf("failed to initialize principal store schema version: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("failed to inspect principal store schema version: %w", err)
	case versionRow.Version > principalSchemaVersion:
		return &PrincipalSchemaMigrationRequiredError{
			StoredVersion:   versionRow.Version,
			ExpectedVersion: principalSchemaVersion,
		}
	default:
		// Same version (or a future older->newer migration once versions exist):
		// ensure tables are present. createTables is idempotent.
		return s.createTables(ctx)
	}
}

func (s *sqlPrincipalStore) createTables(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS principals (
			principal_id TEXT PRIMARY KEY,
			display_name TEXT,
			created_at_unix_milli INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS principal_credentials (
			credential_id TEXT PRIMARY KEY,
			principal_id TEXT NOT NULL,
			type TEXT NOT NULL,
			data TEXT,
			created_at_unix_milli INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_principal_credentials_principal_id ON principal_credentials(principal_id)`,
		`CREATE TABLE IF NOT EXISTS principal_gateways (
			principal_id TEXT NOT NULL,
			gateway TEXT NOT NULL,
			PRIMARY KEY (principal_id, gateway)
		)`,
		`CREATE TABLE IF NOT EXISTS principal_projects (
			principal_id TEXT NOT NULL,
			project_slug TEXT NOT NULL,
			PRIMARY KEY (principal_id, project_slug)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to bootstrap principal store schema: %w", err)
		}
	}
	return nil
}

// principalCount returns the number of stored principals.
func (s *sqlPrincipalStore) principalCount(ctx context.Context) (int, error) {
	return s.db.NewSelect().Model((*principalRow)(nil)).Count(ctx)
}

// getPrincipalByCredential resolves a credential id + secret to its Principal.
// Unknown credential ids, non-api_key credentials, secret mismatches, and orphaned
// credentials all map to ErrPrincipalNotFound (callers return unauthorized).
func (s *sqlPrincipalStore) getPrincipalByCredential(ctx context.Context, credID, secret string) (*Principal, error) {
	cred := &principalCredentialRow{}
	err := s.db.NewSelect().Model(cred).Where("credential_id = ?", credID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPrincipalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up credential: %w", err)
	}
	if cred.Type != credentialTypeAPIKey {
		return nil, ErrPrincipalNotFound
	}

	var data apiKeyCredentialData
	if err := json.Unmarshal(cred.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to decode credential data: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(data.HashedKey), []byte(secret)) != nil {
		return nil, ErrPrincipalNotFound
	}

	principal := &principalRow{}
	err = s.db.NewSelect().Model(principal).Where("principal_id = ?", cred.PrincipalID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		// Credential without a principal row is corrupt; treat as unresolved.
		return nil, ErrPrincipalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load principal: %w", err)
	}

	gateways, err := s.grantValues(ctx, (*principalGatewayRow)(nil), "gateway", cred.PrincipalID)
	if err != nil {
		return nil, err
	}
	projects, err := s.grantValues(ctx, (*principalProjectRow)(nil), "project_slug", cred.PrincipalID)
	if err != nil {
		return nil, err
	}

	displayName := principal.DisplayName
	if displayName == "" {
		displayName = credID
	}
	return &Principal{
		ID:           cred.PrincipalID,
		DisplayName:  displayName,
		CredentialID: credID,
		Gateways:     gateways,
		Projects:     projects,
	}, nil
}

// grantValues reads a single grant column for a principal from one of the
// principal_gateways / principal_projects tables.
func (s *sqlPrincipalStore) grantValues(ctx context.Context, model interface{}, column, principalID string) ([]string, error) {
	var values []string
	err := s.db.NewSelect().
		Model(model).
		Column(column).
		Where("principal_id = ?", principalID).
		Scan(ctx, &values)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s grants: %w", column, err)
	}
	return values, nil
}

// createAPIKeyPrincipal inserts a new principal, its api_key credential, and the
// gateway/project grants in a single transaction. entry supplies the bcrypt hash
// (entry.Hash), credential id (entry.ID), and persisted principal id
// (entry.PrincipalID); see NewAPIKeyEntry.
func (s *sqlPrincipalStore) createAPIKeyPrincipal(ctx context.Context, entry *APIKeyEntry, displayName string, gateways, projects []string) error {
	data, err := json.Marshal(apiKeyCredentialData{HashedKey: entry.Hash})
	if err != nil {
		return fmt.Errorf("failed to encode credential data: %w", err)
	}
	now := time.Now().UTC().UnixMilli()

	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(&principalRow{
			PrincipalID:        entry.PrincipalID,
			DisplayName:        displayName,
			CreatedAtUnixMilli: now,
		}).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert principal: %w", err)
		}
		if _, err := tx.NewInsert().Model(&principalCredentialRow{
			CredentialID:       entry.ID,
			PrincipalID:        entry.PrincipalID,
			Type:               credentialTypeAPIKey,
			Data:               data,
			CreatedAtUnixMilli: now,
		}).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert credential: %w", err)
		}
		for _, gateway := range gateways {
			if _, err := tx.NewInsert().Model(&principalGatewayRow{
				PrincipalID: entry.PrincipalID,
				Gateway:     gateway,
			}).Exec(ctx); err != nil {
				return fmt.Errorf("failed to insert gateway grant: %w", err)
			}
		}
		for _, project := range projects {
			if _, err := tx.NewInsert().Model(&principalProjectRow{
				PrincipalID: entry.PrincipalID,
				ProjectSlug: project,
			}).Exec(ctx); err != nil {
				return fmt.Errorf("failed to insert project grant: %w", err)
			}
		}
		return nil
	})
}

// SQLPrincipalProvider is the SQL-backed PrincipalProvider. It resolves tokens to
// principals via an indexed (primary-key) credential lookup plus one bcrypt verify,
// querying live so revoked credentials take effect without a restart.
type SQLPrincipalProvider struct {
	path  string
	store *sqlPrincipalStore
}

// NewSQLPrincipalProvider creates a provider backed by the sqlite file at path.
func NewSQLPrincipalProvider(path string) *SQLPrincipalProvider {
	return &SQLPrincipalProvider{path: path}
}

// Setup opens the database and bootstraps/migrates the schema. An empty store is
// valid (a fresh install has no principals yet), so Setup does not error on zero
// rows; callers may warn based on Count.
func (p *SQLPrincipalProvider) Setup(_ context.Context) error {
	store, err := openSQLPrincipalStore(p.path)
	if err != nil {
		return err
	}
	p.store = store
	return nil
}

// GetPrincipal resolves a token to its principal. Unknown credentials or secret
// mismatches return ErrPrincipalNotFound; pre-principal tokens return
// ErrLegacyTokenFormat.
func (p *SQLPrincipalProvider) GetPrincipal(ctx context.Context, token string) (*Principal, error) {
	credID, secret, err := parseToken(token)
	if err != nil {
		return nil, err
	}
	if p.store == nil {
		return nil, ErrPrincipalNotFound
	}
	return p.store.getPrincipalByCredential(ctx, credID, secret)
}

// Count returns the number of stored principals (best-effort, for startup logging).
func (p *SQLPrincipalProvider) Count() int {
	if p.store == nil {
		return 0
	}
	count, err := p.store.principalCount(context.Background())
	if err != nil {
		return 0
	}
	return count
}

// Path returns the sqlite file path the provider reads from.
func (p *SQLPrincipalProvider) Path() string {
	return p.path
}

// Close releases the database handle.
func (p *SQLPrincipalProvider) Close() error {
	if p.store == nil {
		return nil
	}
	return p.store.Close()
}
