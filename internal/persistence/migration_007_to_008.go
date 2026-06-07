package persistence

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// migrateV7ToV8 adds the owner_principal_id column to task_runs so a run can be
// attributed to the principal that registered it. This backs cross-session
// resume (ownership checks) and the one-open-run-per-principal invariant.
//
// Existing rows predate ownership tracking and are left with an empty owner;
// they remain unowned and are excluded from per-principal lookups.
func (s *Store) migrateV7ToV8(ctx context.Context) error {
	stmts := []string{
		`ALTER TABLE task_runs ADD COLUMN owner_principal_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_owner_status ON task_runs(owner_principal_id, status, updated_at_unix_milli DESC)`,
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("add task_runs owner column: %w", err)
			}
		}
		return nil
	})
}
