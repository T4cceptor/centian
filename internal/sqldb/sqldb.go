// Package sqldb centralizes opening Bun-backed SQL databases and adapting
// dialect-specific DDL, so the event store and the auth principal store can
// target either SQLite or Postgres without duplicating connection or schema
// logic.
//
// The canonical schema is written in SQLite DDL. PortableDDL adapts it to the
// target driver at bootstrap time; all read/write queries go through Bun, which
// already rewrites placeholders and inlines values per dialect, so they need no
// adaptation.
package sqldb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"

	// Register the pgx stdlib driver under the name "pgx".
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Driver identifies a supported SQL backend.
type Driver string

const (
	// SQLite stores data in a local file (or in-memory) database.
	SQLite Driver = "sqlite"
	// Postgres connects to a PostgreSQL server via a DSN.
	Postgres Driver = "postgres"
)

// ParseDriver normalizes a configured driver string, defaulting to SQLite when
// empty. Unknown drivers are reported as an error.
func ParseDriver(s string) (Driver, error) {
	switch Driver(strings.ToLower(strings.TrimSpace(s))) {
	case "", SQLite:
		return SQLite, nil
	case Postgres:
		return Postgres, nil
	default:
		return "", fmt.Errorf("unsupported sql driver %q (want %q or %q)", s, SQLite, Postgres)
	}
}

// Open opens a Bun database for the given driver and DSN.
//
// For SQLite, dsn is a file path (or ":memory:"); its parent directory is
// created if needed and the pool is pinned to a single connection so
// bootstrap/migration and concurrent readers share one consistent handle. For
// Postgres, dsn is a libpq/pgx connection string; the pool uses modest defaults
// and the connection is pinged to fail fast on a bad DSN.
func Open(driver Driver, dsn string) (*bun.DB, error) {
	switch driver {
	case SQLite:
		return openSQLite(dsn)
	case Postgres:
		return openPostgres(dsn)
	default:
		return nil, fmt.Errorf("unsupported sql driver %q", driver)
	}
}

func openSQLite(path string) (*bun.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite dsn (path) is required")
	}
	if !IsSQLiteMemoryPath(path) {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("failed to create sqlite directory: %w", err)
		}
	}
	sqldb, err := sql.Open(sqliteshim.ShimName, path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	// Keep SQLite on one connection so bootstrap/migration and concurrent readers
	// all observe one consistent sqliteshim-backed handle.
	sqldb.SetMaxOpenConns(1)
	return bun.NewDB(sqldb, sqlitedialect.New()), nil
}

func openPostgres(dsn string) (*bun.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}
	sqldb.SetMaxOpenConns(10)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxLifetime(30 * time.Minute)
	if err := sqldb.PingContext(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	return bun.NewDB(sqldb, pgdialect.New()), nil
}

// IsSQLiteMemoryPath reports whether path refers to an in-memory SQLite database.
func IsSQLiteMemoryPath(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file::memory:")
}

var (
	ddlBlob    = regexp.MustCompile(`\bBLOB\b`)
	ddlInteger = regexp.MustCompile(`\bINTEGER\b`)
	ddlReal    = regexp.MustCompile(`\bREAL\b`)
)

// PortableDDL adapts a CREATE TABLE/INDEX statement written in the canonical
// SQLite dialect to the target driver. For Postgres it rewrites the column type
// keywords that differ or would misbehave:
//
//   - BLOB    -> JSONB            (every blob column in this schema holds JSON)
//   - INTEGER -> BIGINT           (Postgres INTEGER is 4 bytes; unix-milli overflows)
//   - REAL    -> DOUBLE PRECISION (preserve float64 precision)
//
// For SQLite it returns stmt unchanged. Type keywords are uppercase while all
// column names are lowercase snake_case, so whole-word replacement is
// unambiguous.
func PortableDDL(stmt string, driver Driver) string {
	if driver != Postgres {
		return stmt
	}
	stmt = ddlBlob.ReplaceAllString(stmt, "JSONB")
	stmt = ddlInteger.ReplaceAllString(stmt, "BIGINT")
	stmt = ddlReal.ReplaceAllString(stmt, "DOUBLE PRECISION")
	return stmt
}

// Execer is the minimal executor used for issuing DDL. Both *bun.DB and bun.Tx
// satisfy it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type ddlExecer struct {
	exec   Execer
	driver Driver
}

func (d ddlExecer) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return d.exec.ExecContext(ctx, PortableDDL(query, d.driver), args...)
}

// DDLExecer wraps exec so that every statement passed to ExecContext is adapted
// for driver via PortableDDL before execution. This lets schema-creation helpers
// stay written in canonical SQLite DDL while remaining dialect-correct.
func DDLExecer(exec Execer, driver Driver) Execer {
	return ddlExecer{exec: exec, driver: driver}
}
