package sqldb

import (
	"strings"
	"testing"
)

func TestPortableDDL_PostgresRewritesTypes(t *testing.T) {
	// Given: a CREATE TABLE statement written in canonical SQLite DDL
	stmt := `CREATE TABLE IF NOT EXISTS task_runs (
		run_id TEXT PRIMARY KEY,
		created_at_unix_milli INTEGER NOT NULL,
		wall_clock_seconds REAL NOT NULL,
		payload_json BLOB NOT NULL
	)`

	// When: adapting it for Postgres
	got := PortableDDL(stmt, Postgres)

	// Then: BLOB/INTEGER/REAL are rewritten and column names are untouched
	if strings.Contains(got, "BLOB") {
		t.Errorf("BLOB not rewritten: %s", got)
	}
	if !strings.Contains(got, "JSONB") {
		t.Errorf("expected JSONB, got: %s", got)
	}
	if strings.Contains(got, "INTEGER") {
		t.Errorf("INTEGER not rewritten: %s", got)
	}
	if !strings.Contains(got, "BIGINT") {
		t.Errorf("expected BIGINT, got: %s", got)
	}
	if strings.Contains(got, " REAL") {
		t.Errorf("REAL not rewritten: %s", got)
	}
	if !strings.Contains(got, "DOUBLE PRECISION") {
		t.Errorf("expected DOUBLE PRECISION, got: %s", got)
	}
	// Column names (lowercase) must survive verbatim.
	for _, col := range []string{"run_id", "created_at_unix_milli", "wall_clock_seconds", "payload_json"} {
		if !strings.Contains(got, col) {
			t.Errorf("column %q missing after rewrite: %s", col, got)
		}
	}
}

func TestPortableDDL_SQLiteUnchanged(t *testing.T) {
	// Given: a SQLite DDL statement
	stmt := `CREATE TABLE t (a INTEGER, b BLOB, c REAL)`

	// When: adapting it for SQLite
	got := PortableDDL(stmt, SQLite)

	// Then: it is returned verbatim
	if got != stmt {
		t.Errorf("SQLite DDL was modified: got %q want %q", got, stmt)
	}
}

func TestParseDriver(t *testing.T) {
	cases := []struct {
		in      string
		want    Driver
		wantErr bool
	}{
		{in: "", want: SQLite},
		{in: "sqlite", want: SQLite},
		{in: "SQLite", want: SQLite},
		{in: " postgres ", want: Postgres},
		{in: "mysql", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseDriver(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDriver(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDriver(%q) error = %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDriver(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
